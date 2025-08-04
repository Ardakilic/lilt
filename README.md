# FLAC to 16-bit Converter

A cross-platform command-line tool that converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz. Written in Go for excellent performance and cross-platform compatibility.

## Features

- 🎵 Converts 24-bit FLAC files to 16-bit FLAC using SoX
- 📉 Downsamples high sample rate files:
  - 384kHz, 192kHz, or 96kHz → 48kHz
  - 88.2kHz → 44.1kHz
- 🔄 Preserves existing 16-bit FLAC files without unnecessary conversion
- 🎶 Copies MP3 files without modification
- 🖼️ Optional: Copies JPG and PNG images from the source directory
- 🐳 Docker support for containerized SoX execution
- 💻 Cross-platform: Windows, macOS, Linux (x64, ARM64, x86, ARM)

## Installation

### Quick Install (Unix-like systems)

For Linux and macOS, you can use the installation script:

```bash
curl -sSL https://raw.githubusercontent.com/Ardakilic/flac-to-16bit-converter/main/install.sh | bash
```

Or download and run it manually:

```bash
wget https://raw.githubusercontent.com/Ardakilic/flac-to-16bit-converter/main/install.sh
chmod +x install.sh
./install.sh
```

### Download Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/Ardakilic/flac-to-16bit-converter/releases) page:

- **Windows**: `flac-converter-windows-amd64.exe` (x64) or `flac-converter-windows-arm64.exe` (ARM64)
- **macOS**: `flac-converter-darwin-amd64` (Intel) or `flac-converter-darwin-arm64` (Apple Silicon)
- **Linux**: `flac-converter-linux-amd64` (x64), `flac-converter-linux-arm64` (ARM64), `flac-converter-linux-386` (x86), or `flac-converter-linux-arm` (ARM)

### Build from Source

```bash
git clone https://github.com/Ardakilic/flac-to-16bit-converter.git
cd flac-to-16bit-converter
go build -o flac-converter .
```

## Requirements

You can use this tool in one of two ways:

1. **Using Docker (recommended)**:
   - Docker must be installed on your system
   - No local SoX installation required
   - Uses `ardakilic/sox_ng:latest` by default

2. **Using Local SoX Installation**:
   - **SoX (Sound eXchange)** must be installed. [SoX Project](http://sox.sourceforge.net/)
     - Install on Debian/Ubuntu: `sudo apt install sox`
     - Install on macOS: `brew install sox`
     - Install on Windows: Use WSL and install depending on the subsystem, or download SoX Windows binaries
   - You can also use SoX-NG: A drop-in replacement for SoX ([SoX-NG Project](https://codeberg.org/sox_ng/sox_ng/))

## Usage

```bash
flac-converter <source_directory> [options]
```

### Options:

```
--target-dir <dir>   Specify target directory (default: ./transcoded)
--copy-images        Copy JPG and PNG files
--use-docker         Use Docker to run Sox instead of local installation
--docker-image <img> Specify Docker image (default: ardakilic/sox_ng:latest)
```

### Examples:

Using local SoX installation:
```bash
# Windows
flac-converter.exe "C:\Music\MyAlbum" --target-dir "C:\Music\MyAlbum-16bit" --copy-images

# macOS/Linux
./flac-converter ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --copy-images
```

Using Docker:
```bash
# Windows
flac-converter.exe "C:\Music\MyAlbum" --target-dir "C:\Music\MyAlbum-16bit" --use-docker

# macOS/Linux
./flac-converter ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --use-docker
```

## Docker Support

When using the `--use-docker` option:

- Docker must be installed on your system
- The tool mounts your source and target directories as volumes in the container
- No local SoX installation is required
- Uses `ardakilic/sox_ng:latest` by default, which is a containerized version of SoX-NG
- Source code of the Docker image is available [here](https://github.com/Ardakilic/sox_ng_dockerized)

You can specify a different Docker image with the `--docker-image` option:
```bash
flac-converter ~/Music/MyAlbum --use-docker --docker-image your/sox-image:tag
```

Alternative Docker images you can use:
- `bigpapoo/sox`: Another SoX Docker image
- Any image that provides SoX installed as the `sox` command

## How It Works

1. The tool scans the source directory recursively for `.flac` and `.mp3` files
2. If a FLAC file is **24-bit**, it is converted to **16-bit** using SoX
3. If a FLAC file has a sample rate of **96kHz, 192kHz, or 384kHz**, it is downsampled to **48kHz**
4. If a FLAC file has a sample rate of **88.2kHz**, it is downsampled to **44.1kHz**
5. MP3 files are copied without modification
6. If `--copy-images` is enabled, `.jpg` and `.png` files are copied to the target directory
7. The original folder structure is preserved in the target directory

## Technical Details

- Written in Go for excellent cross-platform compatibility and performance
- Uses SoX's `--multi-threaded` option for performance
- The `-G` flag ensures proper gain handling
- Uses `dither` when downsampling to 16-bit for better quality
- Maintains the same folder structure in the target directory
- Graceful error handling - if conversion fails, the original file is copied

## Development

### Requirements

- Go 1.21 or later
- Make (optional, for convenience)

### Building

```bash
# Clone the repository
git clone https://github.com/Ardakilic/flac-to-16bit-converter.git
cd flac-to-16bit-converter

# Build for current platform
go build -o flac-converter .

# Or use Make
make build
```

### Testing

```bash
# Run tests
go test -v ./...

# Or use Make
make test
```

### Building for Different Platforms

```bash
# Linux x64
GOOS=linux GOARCH=amd64 go build -o flac-converter-linux-amd64 .

# Windows x64
GOOS=windows GOARCH=amd64 go build -o flac-converter-windows-amd64.exe .

# macOS ARM64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o flac-converter-darwin-arm64 .

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -o flac-converter-linux-arm64 .

# Build all platforms at once
make build-all
```

### Code Quality

```bash
# Format code
go fmt ./...

# Run linter (requires golangci-lint)
golangci-lint run

# Or use Make
make fmt
make lint
```

### CI/CD

The project uses GitHub Actions to automatically build binaries for all supported platforms on every commit. The workflow:

- Builds for Windows, macOS, and Linux
- Supports x64, ARM64, x86, and ARM architectures
- Creates downloadable artifacts
- Can be triggered manually via GitHub Actions
- Creates pre-releases for development builds

## Migration from Bash Script

If you're migrating from the original bash script (`flac-converter.sh`), the usage is identical. Simply replace:

```bash
# Old
./flac-converter.sh ~/Music/Album --target-dir ~/Music/Album-16bit

# New
./flac-converter ~/Music/Album --target-dir ~/Music/Album-16bit
```

All command-line arguments remain the same for seamless migration.

## License

This project is open-source under the MIT License.

## Author

Arda Kilicdagi

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

