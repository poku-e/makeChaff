# makeChaff

A small Go CLI that fills a directory with randomly generated "chaff" files until disk space is exhausted, then securely overwrites and removes the created files using a DoD\-like 3\-pass shred (0xFF, 0x00, random). Intended for testing and demonstration; use with extreme caution.

## Files
- `chaff.go` — main program
- `README.md` — this file

## Features
- Generates fixed\-size chaff files until the target filesystem is full (or nearly full).
- Tracks created files and performs a 3\-pass overwrite on each (0xFF, 0x00, cryptographic random).
- Fsyncs after each pass and removes files.
- ANSI\-colored terminal output and inline ASCII progress bars.

## Build
Requires Go toolchain.

```bash
go build -o makeChaff .
```

or

```bash
go run .
```

## Basic usage
1. Ensure you understand the risks and have backups.
2. Run the binary and confirm when prompted:

```bash
./makeChaff
```

The program by default writes to `./chaff` with 100 MB files (configurable in `chaff.go`).

## Configuration (edit `chaff.go`)
- `outputDir` — target directory (default `./chaff`)
- `fileSizeMB` — size of each generated file in MB (default `100`)
- `filePrefix` — file name prefix (default `chaff_`)

## Safety & warnings
- This program will intentionally fill disk space. Do not run on systems where full disks will cause data loss or service interruption.
- Always run on disposable test systems, not on production hosts.
- Confirm the prompt (`yes`) to proceed. Anything else cancels the operation.

## Limitations
- Overwriting files does not guarantee data destruction on modern SSDs, copy\-on\-write filesystems (btrfs, APFS), or systems using snapshots. For SSDs or enterprise needs consider whole\-disk encryption, hardware secure erase (ATA Secure Erase), or wiping unused blocks and snapshots.
- If a file cannot be re\-opened for writing during shredding, the program attempts removal but may report an error.
- The program uses the standard library only and does not attempt low\-level device operations.

## Recommended safe testing
- Modify `fileSizeMB` to a small value (e.g., `1`) or run in a temporary filesystem (RAM disk or loopback file) to validate behavior without consuming large amounts of storage.



