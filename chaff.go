//go:build linux || darwin
// +build linux darwin

package main

import (
	"fmt"
	_ "os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ANSI colors
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorCyan   = "\033[36m"
	barWidth    = 40
)

func colorize(s, color string) string { return color + s + ColorReset }

// ================================================================
// Progress and Formatting
// ================================================================

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
	msg := fmt.Sprintf("\r%s %s %6.2f%%", prefix, bar, percent)
	unix.Write(unix.Stdout, []byte(msg))
	if written >= total {
		unix.Write(unix.Stdout, []byte("\n"))
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ================================================================
// System Utilities
// ================================================================

func getAvailableSpace(path string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func readUrandom(buf []byte) error {
	fd, err := unix.Open("/dev/urandom", unix.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	total := 0
	for total < len(buf) {
		n, err := unix.Read(fd, buf[total:])
		if err != nil {
			return err
		}
		total += n
	}
	return nil
}

// ================================================================
// File Generation (Direct Syscalls)
// ================================================================

func generateChaffFile(filename string, sizeMB int64, available uint64) (uint64, uint64, error) {
	sizeBytes := uint64(sizeMB) * 1024 * 1024
	if sizeBytes > available {
		sizeBytes = available
	}
	if sizeBytes == 0 {
		return available, 0, nil
	}

	fd, err := unix.Open(filename, unix.O_CREAT|unix.O_WRONLY|unix.O_TRUNC, 0644)
	if err != nil {
		return available, 0, err
	}
	defer unix.Close(fd)

	const chunkSize = 1024 * 1024
	buf := make([]byte, chunkSize)
	var written uint64
	base := filepath.Base(filename)
	prefix := colorize(fmt.Sprintf("Writing %s", base), ColorCyan)

	for written < sizeBytes {
		remain := sizeBytes - written
		n := chunkSize
		if remain < uint64(n) {
			n = int(remain)
		}
		if err := readUrandom(buf[:n]); err != nil {
			return available - written, written, err
		}
		w, err := unix.Write(fd, buf[:n])
		if err != nil {
			return available - written, written, err
		}
		written += uint64(w)
		printProgress(prefix, written, sizeBytes)
	}

	unix.Fsync(fd)
	fmt.Printf("%s %s\n", colorize("Created:", ColorGreen), filename)
	return available - sizeBytes, written, nil
}

// ================================================================
// Shredding Logic (Low-Level)
// ================================================================

func shredFile(path string) error {
	fd, err := unix.Open(path, unix.O_RDWR, 0)
	if err != nil {
		unix.Unlink(path)
		return err
	}
	defer unix.Close(fd)

	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Unlink(path)
		return err
	}

	if st.Size == 0 {
		unix.Unlink(path)
		return nil
	}

	size := int(st.Size)
	const chunkSize = 1024 * 1024
	buf := make([]byte, chunkSize)
	base := filepath.Base(path)
	passes := []string{"0xFF", "0x00", "random"}

	for i, pass := range passes {
		var filled byte
		switch pass {
		case "0xFF":
			filled = 0xFF
			for i := range buf {
				buf[i] = filled
			}
		case "0x00":
			filled = 0x00
			for i := range buf {
				buf[i] = filled
			}
		case "random":
			_ = readUrandom(buf)
		}

		unix.Seek(fd, 0, 0)
		remaining := size
		var written int

		prefix := fmt.Sprintf("%s %s - %s:",
			colorize("Shredding", ColorRed), base,
			colorize(fmt.Sprintf("Pass %d (%s)", i+1, pass), ColorYellow))

		for remaining > 0 {
			toWrite := chunkSize
			if remaining < chunkSize {
				toWrite = remaining
			}
			n, err := unix.Write(fd, buf[:toWrite])
			if err != nil {
				return err
			}
			remaining -= n
			written += n
			printProgress(prefix, uint64(written), uint64(size))
		}
		unix.Fsync(fd)
	}

	unix.Close(fd)
	unix.Unlink(path)
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

// ================================================================
// TRIM/Discard Handling
// ================================================================

func runTrim(path string) error {
	fmt.Println()
	switch runtime.GOOS {
	case "linux":
		fmt.Println("Detected Linux: attempting direct fstrim syscall...")
		const FITRIM = 0x00009409
		type fstrimRange struct {
			Start  uint64
			Len    uint64
			Minlen uint64
		}
		fd, err := unix.Open(path, unix.O_RDONLY, 0)
		if err != nil {
			return err
		}
		defer unix.Close(fd)

		rng := fstrimRange{Start: 0, Len: ^uint64(0), Minlen: 0}
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), FITRIM, uintptr(unsafe.Pointer(&rng)))
		if errno != 0 {
			return fmt.Errorf("ioctl FITRIM failed: %v", errno)
		}
		fmt.Println("TRIM operation completed successfully.")
		return nil

	case "darwin":
		fmt.Println("Detected macOS: TRIM/discard must be performed manually.")
		fmt.Printf("Suggested: sudo diskutil secureErase freespace 0 %s\n", path)
		return nil

	default:
		fmt.Printf("TRIM/discard not supported for OS: %s\n", runtime.GOOS)
		return nil
	}
}

// ================================================================
// Entry Point
// ================================================================

func main() {
	outputDir := "./chaff"
	fileSizeMB := int64(100)
	filePrefix := "chaff_"

	fmt.Println(colorize("=== Low-Level Chaff Generator ===", ColorCyan))
	fmt.Println(colorize("WARNING: This will fill your disk with random data!", ColorYellow))
	abs, _ := filepath.Abs(outputDir)
	fmt.Printf("%s %s\n", colorize("Target directory:", ColorCyan), abs)
	fmt.Printf("%s %d MB\n", colorize("File size:", ColorCyan), fileSizeMB)
	fmt.Println()

	fmt.Print(colorize("Are you sure you want to continue? (yes/NO): ", ColorRed))
	var resp string
	fmt.Scanln(&resp)
	if resp != "yes" {
		fmt.Println(colorize("Operation cancelled.", ColorGreen))
		return
	}

	unix.Mkdir(outputDir, 0755)

	available, err := getAvailableSpace(outputDir)
	if err != nil {
		fmt.Printf("%s %v\n", colorize("Error getting disk space:", ColorRed), err)
		return
	}

	fmt.Printf("\n%s %s\n", colorize("Starting with", ColorCyan), formatBytes(available))
	fmt.Println(colorize("Generating chaff files...", ColorCyan))

	fileCount := 0
	created := []string{}

	for available > 0 {
		filename := filepath.Join(outputDir, fmt.Sprintf("%s%06d.dat", filePrefix, fileCount))
		if available < 10*1024*1024 {
			fmt.Println(colorize("Less than 10MB remaining, final small file...", ColorYellow))
			final := filepath.Join(outputDir, fmt.Sprintf("%sFINAL.dat", filePrefix))
			rem, written, err := generateChaffFile(final, 1, available)
			if err == nil && written > 0 {
				created = append(created, final)
			}
			available = rem
			break
		}

		rem, written, err := generateChaffFile(filename, fileSizeMB, available)
		if err != nil {
			fmt.Printf("%s %s: %v\n", colorize("Error creating", ColorRed), filename, err)
			fileCount++
			continue
		}
		if written > 0 {
			created = append(created, filename)
		}
		available = rem
		fileCount++

		if fileCount%10 == 0 {
			fmt.Printf("%s %d %s\n", colorize("Progress:", ColorCyan), fileCount, formatBytes(available))
		}
	}

	fmt.Printf("\n%s\n", colorize("=== Generation Complete ===", ColorCyan))
	fmt.Printf("%s %d\n", colorize("Files created:", ColorCyan), fileCount)

	finalSpace, _ := getAvailableSpace(outputDir)
	fmt.Printf("%s %s\n", colorize("Space before shredding:", ColorCyan), formatBytes(finalSpace))

	if len(created) > 0 {
		fmt.Println()
		fmt.Println(colorize("Shredding chaff files...", ColorRed))
		shredFiles(created)
	} else {
		fmt.Println(colorize("No files to shred.", ColorYellow))
	}

	finalAfter, _ := getAvailableSpace(outputDir)
	fmt.Printf("%s %s\n", colorize("Final available space:", ColorCyan), formatBytes(finalAfter))

	fmt.Print("\nAttempt TRIM/discard? (yes/NO): ")
	fmt.Scanln(&resp)
	if resp == "yes" {
		if err := runTrim(outputDir); err != nil {
			fmt.Printf("TRIM failed: %v\n", err)
		}
	} else {
		fmt.Println("Skipping TRIM/discard step.")
	}

	fmt.Printf("\n%s\n", colorize("=== Operation Complete ===", ColorGreen))
}
