package main

import (
	"crypto/rand"
	"fmt"
	_ "io"
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
func generateChaffFile(filename string, sizeMB int64, availableSpace uint64) (uint64, error) {
	sizeBytes := uint64(sizeMB) * 1024 * 1024

	// Don't exceed available space
	if sizeBytes > availableSpace {
		sizeBytes = availableSpace
	}

	if sizeBytes == 0 {
		return availableSpace, nil
	}

	file, err := os.Create(filename)
	if err != nil {
		return availableSpace, err
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
			return availableSpace - bytesWritten, err
		}

		// Write to file
		written, err := file.Write(buffer)
		if err != nil {
			return availableSpace - bytesWritten, err
		}

		bytesWritten += uint64(written)

		// Print progress every 100MB
		if bytesWritten%(100*1024*1024) == 0 {
			fmt.Printf("Written: %d MB\n", bytesWritten/(1024*1024))
		}
	}

	fmt.Printf("Created: %s (%s)\n", filename, formatBytes(sizeBytes))
	return availableSpace - sizeBytes, nil
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

	fileCount := 0
	availableSpace, err := getAvailableSpace(outputDir)
	if err != nil {
		fmt.Printf("Error getting disk space: %v\n", err)
		return
	}

	fmt.Printf("\nStarting with %s available\n", formatBytes(availableSpace))
	fmt.Println("Generating chaff files...\n")

	for availableSpace > 0 {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s%06d.dat", filePrefix, fileCount))

		// Check if we're about to run out of space for even a small file
		if availableSpace < 10*1024*1024 { // Less than 10MB left
			fmt.Println("Less than 10MB remaining, creating final small file...")
			// Create one final small file with remaining space
			finalFilename := filepath.Join(outputDir, fmt.Sprintf("%sFINAL.dat", filePrefix))
			remainingSpace, err := generateChaffFile(finalFilename, 1, availableSpace)
			if err != nil {
				fmt.Printf("Error creating final file: %v\n", err)
			}
			availableSpace = remainingSpace
			break
		}

		// Generate regular chaff file
		remainingSpace, err := generateChaffFile(filename, fileSizeMB, availableSpace)
		if err != nil {
			fmt.Printf("Error creating file %s: %v\n", filename, err)
			// Try to continue with next file
			fileCount++
			continue
		}

		availableSpace = remainingSpace
		fileCount++

		// Update progress
		if fileCount%10 == 0 {
			fmt.Printf("Progress: %d files created, %s remaining\n",
				fileCount, formatBytes(availableSpace))
		}
	}

	fmt.Printf("\n=== Operation Complete ===\n")
	fmt.Printf("Total files created: %d\n", fileCount)

	// Final space check
	finalSpace, err := getAvailableSpace(outputDir)
	if err == nil {
		fmt.Printf("Final available space: %s\n", formatBytes(finalSpace))
	}
}
