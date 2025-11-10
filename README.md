# makeChaff

A small Go CLI that fills a directory with randomly generated "chaff" files until disk space is exhausted, then securely overwrites and removes the created files using a DoD-like 3-pass shred (0xFF, 0x00, random). Intended for testing and demonstration; use with extreme caution.

## Files
- `chaff.go` — main program
- `README.md` — this file

## Features
- Generates fixed-size chaff files until the target filesystem is full (or nearly full).
- Tracks created files and performs a 3-pass overwrite on each (0xFF, 0x00, cryptographic random).
- Fsyncs after each pass and removes files.
- ANSI-colored terminal output and inline ASCII progress bars.
- Optional TRIM/discard step (best-effort): the program can invoke platform-specific utilities to issue discard/TRIM operations after shredding.

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

The program by default writes to `./chaff` with 100 MB files (configurable in `chaff.go`). After shredding the created files the program will prompt whether you want to attempt a TRIM/discard operation on the target filesystem (best-effort, platform-dependent).

## TRIM / Low-level discard (what the program does and recommended commands)

Important: TRIM/discard and low-level device commands are powerful operations that often require root and may affect the entire device. Use extreme caution and run only on test hardware or after ensuring you have backups.

What the program does
- After overwriting and removing the chaff files, the program offers to run a best-effort TRIM/discard step.
- On Linux the program will attempt to run `fstrim -v <path>` (this requires `fstrim` to be available and typically needs root). `fstrim` tells the filesystem to inform the underlying block device which blocks are unused, allowing SSDs to free physical pages.
- On macOS the program cannot safely run a generic TRIM command from userland for APFS/SSD devices, so it prints recommended commands and guidance (for example `sudo diskutil secureErase freespace 0 <mountpoint>` or vendor tools/`trimforce`), and instructs the user to run those with care.

Platform examples

- Linux (recommended, requires root):

```bash
sudo fstrim -v /path/to/mountpoint
```

- macOS (read the manual and be careful):

- To zero free space (not a direct TRIM but may be useful):

```bash
sudo diskutil secureErase freespace 0 /path/to/mountpoint
```

- To enable TRIM support for third-party SSDs (dangerous, requires reboot and may void warranty):

```bash
sudo trimforce enable
```

Notes and limitations
- Running `fstrim` or `diskutil` may require root privileges.
- TRIM/discard only affects the underlying device and does not guarantee that previously written data is unrecoverable in all scenarios (snapshots, backups, encryption layers). On filesystems with copy-on-write or with snapshotting (APFS, btrfs, some ZFS setups), data may still exist in snapshots until they are removed.
- For SSDs and flash storage, overwriting files is not a guaranteed secure wipe. Consider whole-disk encryption from the start, hardware secure-erase (vendor ATA Secure Erase), or wiping the whole device when secure deletion is required.

## Configuration (edit `chaff.go`)
- `outputDir` — target directory (default `./chaff`)
- `fileSizeMB` — size of each generated file in MB (default `100`)
- `filePrefix` — file name prefix (default `chaff_`)

## Safety & warnings
- This program will intentionally fill disk space. Do not run on systems where full disks will cause data loss or service interruption.
- Always run on disposable test systems, not on production hosts.
- Confirm the prompt (`yes`) to proceed. Anything else cancels the operation.
- Low-level TRIM/discard operations may require root and can have irreversible effects. Read the platform docs before using.

## Recommended safe testing
- Modify `fileSizeMB` to a small value (e.g., `1`) or run in a temporary filesystem (RAM disk or loopback file) to validate behavior without consuming large amounts of storage.

## License
Include your project license here.
