package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// getAvailableSpace returns available disk space in bytes
func getAvailableSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}

	// Available blocks * block size
	return stat.Bavail * uint64(stat.Bsize), nil
}

// formatBytes converts bytes to human readable string
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// generateChaffFile creates a file filled with random data
// returns (remainingSpace, bytesWritten, error)
func generateChaffFile(filename string, sizeMB int64, availableSpace uint64) (uint64, uint64, error) {
	sizeBytes := uint64(sizeMB) * 1024 * 1024

	// Don't exceed available space
	if sizeBytes > availableSpace {
		sizeBytes = availableSpace
	}

	if sizeBytes == 0 {
		return availableSpace, 0, nil
	}

	file, err := os.Create(filename)
	if err != nil {
		return availableSpace, 0, err
	}
	defer file.Close()

	// Write in chunks to manage memory usage
	const chunkSize = 1024 * 1024 // 1MB chunks
	var bytesWritten uint64

	for bytesWritten < sizeBytes {
		remaining := sizeBytes - bytesWritten
		currentChunk := chunkSize
		if remaining < chunkSize {
			currentChunk = int(remaining)
		}

		// Generate random data
		buffer := make([]byte, currentChunk)
		_, err := rand.Read(buffer)
		if err != nil {
			return availableSpace - bytesWritten, bytesWritten, err
		}

		// Write to file
		written, err := file.Write(buffer)
		if err != nil {
			return availableSpace - bytesWritten, bytesWritten, err
		}

		bytesWritten += uint64(written)

		// Print progress every 100MB
		if bytesWritten%(100*1024*1024) == 0 {
			fmt.Printf("Written: %d MB\n", bytesWritten/(1024*1024))
		}
	}

	// Ensure data is flushed to disk
	if err := file.Sync(); err != nil {
		return availableSpace - bytesWritten, bytesWritten, err
	}

	fmt.Printf("Created: %s (%s)\n", filename, formatBytes(sizeBytes))
	return availableSpace - sizeBytes, bytesWritten, nil
}

// shredFile overwrites the file with DoD-like 3 passes and removes it:
// pass 1: 0xFF, pass 2: 0x00, pass 3: random bytes.
func shredFile(path string) error {
	const chunkSize = 1024 * 1024 // 1MB

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		// If file can't be opened for writing, attempt to remove it
		_ = os.Remove(path)
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return err
	}
	size := info.Size()
	if size == 0 {
		_ = f.Close()
		return os.Remove(path)
	}

	passes := []int{0, 1, 2} // 0 -> 0xFF, 1 -> 0x00, 2 -> random

	buf := make([]byte, chunkSize)

	for _, pass := range passes {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}

		remaining := size
		for remaining > 0 {
			toWrite := int64(chunkSize)
			if remaining < toWrite {
				toWrite = remaining
			}

			switch pass {
			case 0:
				// 0xFF
				for i := int64(0); i < toWrite; i++ {
					buf[i] = 0xFF
				}
			case 1:
				// 0x00
				for i := int64(0); i < toWrite; i++ {
					buf[i] = 0x00
				}
			case 2:
				// random
				if _, err := rand.Read(buf[:toWrite]); err != nil {
					return err
				}
			}

			n, err := f.Write(buf[:toWrite])
			if err != nil {
				return err
			}
			if int64(n) != toWrite {
				return fmt.Errorf("short write while shredding %s", path)
			}
			remaining -= toWrite
		}

		// Ensure pass is flushed
		if err := f.Sync(); err != nil {
			return err
		}
	}

	// Close before removal
	if err := f.Close(); err != nil {
		return err
	}

	// Finally remove the file
	if err := os.Remove(path); err != nil {
		return err
	}

	fmt.Printf("Shredded and removed: %s\n", path)
	return nil
}

func shredFiles(files []string) {
	for _, f := range files {
		if err := shredFile(f); err != nil {
			fmt.Printf("Error shredding %s: %v\n", f, err)
		}
	}
}

func main() {
	// Configuration
	outputDir := "./chaff"   // Current directory - change as needed
	fileSizeMB := int64(100) // Size of each chaff file in MB
	filePrefix := "chaff_"

	fmt.Println("=== Chaff Generator ===")
	fmt.Println("WARNING: This will fill your disk with random data!")
	absPath, _ := filepath.Abs(outputDir)
	fmt.Printf("Target directory: %s\n", absPath)
	fmt.Printf("File size: %d MB per file\n", fileSizeMB)
	fmt.Println()

	// Safety confirmation
	fmt.Print("Are you sure you want to continue? (yes/NO): ")
	var response string
	fmt.Scanln(&response)
	if response != "yes" {
		fmt.Println("Operation cancelled.")
		return
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", outputDir, err)
		return
	}

	fileCount := 0
	availableSpace, err := getAvailableSpace(outputDir)
	if err != nil {
		fmt.Printf("Error getting disk space: %v\n", err)
		return
	}

	createdFiles := []string{}

	fmt.Printf("\nStarting with %s available\n", formatBytes(availableSpace))
	fmt.Println("Generating chaff files...")

	for availableSpace > 0 {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s%06d.dat", filePrefix, fileCount))

		// Check if we're about to run out of space for even a small file
		if availableSpace < 10*1024*1024 { // Less than 10MB left
			fmt.Println("Less than 10MB remaining, creating final small file...")
			// Create one final small file with remaining space (attempt 1MB but generator will cap)
			finalFilename := filepath.Join(outputDir, fmt.Sprintf("%sFINAL.dat", filePrefix))
			remainingSpace, bytesCreated, err := generateChaffFile(finalFilename, 1, availableSpace)
			if err != nil {
				fmt.Printf("Error creating final file: %v\n", err)
			} else if bytesCreated > 0 {
				createdFiles = append(createdFiles, finalFilename)
			}
			availableSpace = remainingSpace
			break
		}

		// Generate regular chaff file
		remainingSpace, bytesCreated, err := generateChaffFile(filename, fileSizeMB, availableSpace)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", filename, err)
			// Try to continue with next file
			fileCount++
			continue
		}

		// If generator wrote data, track the file for later shredding
		if bytesCreated > 0 {
			createdFiles = append(createdFiles, filename)
		}

		availableSpace = remainingSpace
		fileCount++

		// Update progress
		if fileCount%10 == 0 {
			fmt.Printf("Progress: %d files created, %s remaining\n",
				fileCount, formatBytes(availableSpace))
		}
	}

	fmt.Printf("\n=== Generation Complete ===\n")
	fmt.Printf("Total files created: %d\n", fileCount)

	// Final space check
	finalSpace, err := getAvailableSpace(outputDir)
	if err == nil {
		fmt.Printf("Space remaining before shredding: %s\n", formatBytes(finalSpace))
	}

	// Shred created files using DoD-like passes
	if len(createdFiles) > 0 {
		fmt.Println("\nShredding created chaff files...")
		shredFiles(createdFiles)
	} else {
		fmt.Println("No chaff files to shred.")
	}

	// Final space check after shredding
	finalSpaceAfter, err := getAvailableSpace(outputDir)
	if err == nil {
		fmt.Printf("Final available space: %s\n", formatBytes(finalSpaceAfter))
	}

	fmt.Printf("\n=== Operation Complete ===\n")
}
