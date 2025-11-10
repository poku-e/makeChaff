# makeChaff

makeChaff is a utility designed for the forensic destruction of sensitive data. It works across platforms, with usage details particularly tuned for MacOS, but it can be adapted to any UNIX-like operating system.

## Features

- Securely overwrites and destroys files to prevent data recovery.
- Can recursively wipe directories.
- Designed with simplicity and effectiveness in mind.

## Why "forensic destruction"?

Ordinary file deletion only removes references to files from the filesystem, leaving data potentially recoverable by forensic tools. makeChaff ensures sensitive content is overwritten to make recovery impractical, following best practices for data sanitization.

## Installation

### MacOS

1. **Clone the repository:**
   ```sh
   git clone https://github.com/poku-e/makeChaff.git
   cd makeChaff
   ```

2. **Build (if applicable):**
   - If makeChaff is a script (e.g., Bash or Python), no build is needed; just ensure it has execute permissions:
     ```sh
     chmod +x makeChaff
     ```
   - If makeChaff is written in a compiled language (e.g., C/C++/Rust), follow the instructions in [BUILD.md](BUILD.md) or use:
     ```sh
     make
     ```
     or
     ```sh
     gcc -o makeChaff makeChaff.c
     ```

3. **(Optional) Install in your PATH:**
   ```sh
   cp makeChaff /usr/local/bin/
   ```

### Other OS

The same process applies for Linux and other UNIX-like systems. For Windows, consider using the Windows Subsystem for Linux (WSL) or adapt the code for native compatibility.

## Usage

**Overwrite and destroy a file:**
```sh
./makeChaff path/to/your/secret.file
```

**Recursively wipe a directory:**
```sh
./makeChaff -r path/to/your/directory
```

**View help:**
```sh
./makeChaff --help
```

## Security Notes

- Data destruction is only as effective as the filesystem allows. SSDs, certain filesystems, and cloud storage may retain data in ways not completely erased by overwriting.
- For highly sensitive data, consider physical destruction of the device after software-based wiping.

## Disclaimer

Use at your own risk. This tool will permanently destroy data and cannot be reversed.

## Contributing

Pull requests are welcome. Please open issues to discuss ideas or report bugs.

## License

[MIT License](LICENSE)

---

For questions, contact [poku-e](https://github.com/poku-e).
