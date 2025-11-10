package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	barWidth    = 40
)

// Helpers
func colorize(s, color string) string { return color + s + ColorReset }

func renderBar(written, total uint64) string {
	if total == 0 {
		return "[" + strings.Repeat(" ", barWidth) + "] 0%"
	}
	ratio := float64(written) / float64(total)
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(barWidth))
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", barWidth-filled) + "]"
}

func printProgress(prefix string, written, total uint64) {
	bar := renderBar(written, total)
	percent := 0.0
	if total > 0 {
		percent = (float64(written) / float64(total)) * 100.0
	}
	fmt.Printf("\r%s %s %6.2f%%", prefix, bar, percent)
	if written >= total {
		fmt.Print("\n")
	}
}

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

	base := filepath.Base(filename)
	prefix := colorize(fmt.Sprintf("Writing %s", base), ColorCyan)

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

		// Update inline progress
		printProgress(prefix, bytesWritten, sizeBytes)
	}

	// Ensure data is flushed to disk
	if err := file.Sync(); err != nil {
		return availableSpace - bytesWritten, bytesWritten, err
	}

	fmt.Printf("%s %s\n", colorize("Created:", ColorGreen), filename)
	return availableSpace - sizeBytes, bytesWritten, nil
}

// shredFile overwrites the file with DoD-like 3 passes and removes it:
// pass 1: 0xFF, pass 2: 0x00, pass 3: random bytes.
func shredFile(path string) error {
	const chunkSize = 1024 * 1024 // 1MB

	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
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
	base := filepath.Base(path)

	for _, pass := range passes {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}

		remaining := size
		var writtenThisPass int64 = 0
		passLabel := ""
		switch pass {
		case 0:
			passLabel = colorize("Pass 1 (0xFF)", ColorYellow)
		case 1:
			passLabel = colorize("Pass 2 (0x00)", ColorYellow)
		case 2:
			passLabel = colorize("Pass 3 (random)", ColorYellow)
		}
		prefix := fmt.Sprintf("%s %s", colorize("Shredding", ColorRed), base)
		fullPrefix := fmt.Sprintf("%s - %s:", prefix, passLabel)

		for remaining > 0 {
			toWrite := int(chunkSize)
			if remaining < int64(toWrite) {
				toWrite = int(remaining)
			}

			switch pass {
			case 0:
				// 0xFF
				for i := 0; i < toWrite; i++ {
					buf[i] = 0xFF
				}
			case 1:
				// 0x00
				for i := 0; i < toWrite; i++ {
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
			if n != toWrite {
				return fmt.Errorf("short write while shredding %s", path)
			}

			remaining -= int64(n)
			writtenThisPass += int64(n)

			// Show inline progress for this pass
			printProgress(fullPrefix, uint64(writtenThisPass), uint64(size))
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

	fmt.Printf("%s %s\n", colorize("Shredded and removed:", ColorGreen), path)
	return nil
}

func shredFiles(files []string) {
	for _, f := range files {
		if err := shredFile(f); err != nil {
			fmt.Printf("%s %s: %v\n", colorize("Error shredding", ColorRed), f, err)
		}
	}
}

func main() {
	// Configuration
	outputDir := "./chaff"   // Current directory - change as needed
	fileSizeMB := int64(100) // Size of each chaff file in MB
	filePrefix := "chaff_"

	fmt.Println(colorize("=== Chaff Generator ===", ColorCyan))
	fmt.Println(colorize("WARNING: This will fill your disk with random data!", ColorYellow))
	absPath, _ := filepath.Abs(outputDir)
	fmt.Printf("%s %s\n", colorize("Target directory:", ColorCyan), absPath)
	fmt.Printf("%s %d %s\n", colorize("File size:", ColorCyan), fileSizeMB, "MB per file")
	fmt.Println()

	// Safety confirmation
	fmt.Print(colorize("Are you sure you want to continue? (yes/NO): ", ColorRed))
	var response string
	fmt.Scanln(&response)
	if response != "yes" {
		fmt.Println(colorize("Operation cancelled.", ColorGreen))
		return
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("%s %s: %v\n", colorize("Error creating directory", ColorRed), outputDir, err)
		return
	}

	fileCount := 0
	availableSpace, err := getAvailableSpace(outputDir)
	if err != nil {
		fmt.Printf("%s %v\n", colorize("Error getting disk space:", ColorRed), err)
		return
	}

	createdFiles := []string{}

	fmt.Printf("\n%s %s\n", colorize("Starting with", ColorCyan), formatBytes(availableSpace))
	fmt.Println(colorize("Generating chaff files...", ColorCyan))

	for availableSpace > 0 {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s%06d.dat", filePrefix, fileCount))

		// Check if we're about to run out of space for even a small file
		if availableSpace < 10*1024*1024 { // Less than 10MB left
			fmt.Println(colorize("Less than 10MB remaining, creating final small file...", ColorYellow))
			finalFilename := filepath.Join(outputDir, fmt.Sprintf("%sFINAL.dat", filePrefix))
			remainingSpace, bytesCreated, err := generateChaffFile(finalFilename, 1, availableSpace)
			if err != nil {
				fmt.Printf("%s %v\n", colorize("Error creating final file:", ColorRed), err)
			} else if bytesCreated > 0 {
				createdFiles = append(createdFiles, finalFilename)
			}
			availableSpace = remainingSpace
			break
		}

		// Generate regular chaff file
		remainingSpace, bytesCreated, err := generateChaffFile(filename, fileSizeMB, availableSpace)
		if err != nil {
			fmt.Printf("%s %s: %v\n", colorize("Error creating file", ColorRed), filename, err)
			fileCount++
			continue
		}

		if bytesCreated > 0 {
			createdFiles = append(createdFiles, filename)
		}

		availableSpace = remainingSpace
		fileCount++

		// Update progress every 10 files
		if fileCount%10 == 0 {
			fmt.Printf("%s %d %s\n", colorize("Progress:", ColorCyan), fileCount, formatBytes(availableSpace))
		}
	}

	fmt.Printf("\n%s\n", colorize("=== Generation Complete ===", ColorCyan))
	fmt.Printf("%s %d\n", colorize("Total files created:", ColorCyan), fileCount)

	// Final space check
	finalSpace, err := getAvailableSpace(outputDir)
	if err == nil {
		fmt.Printf("%s %s\n", colorize("Space remaining before shredding:", ColorCyan), formatBytes(finalSpace))
	}

	// Shred created files using DoD-like passes
	if len(createdFiles) > 0 {
		fmt.Println()
		fmt.Println(colorize("Shredding created chaff files...", ColorRed))
		shredFiles(createdFiles)
	} else {
		fmt.Println(colorize("No chaff files to shred.", ColorYellow))
	}

	// Final space check after shredding
	finalSpaceAfter, err := getAvailableSpace(outputDir)
	if err == nil {
		fmt.Printf("%s %s\n", colorize("Final available space:", ColorCyan), formatBytes(finalSpaceAfter))
	}

	fmt.Printf("\n%s\n", colorize("=== Operation Complete ===", ColorGreen))
}
