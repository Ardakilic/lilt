# Lilt

![Lilt Logo](assets/logo.svg)

Lilt is a cross-platform command-line tool that converts Hi-Res FLAC and ALAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz. Written in Go for excellent performance and cross-platform compatibility.

Lilt stands for "lightweight intelligent lossless transcoder". It is also a form of traditional singing common in the Goidelic speaking areas of Ireland, Scotland and the Isle of Man, though singing styles like it occur in many other countries.

## Features

- 🎵 **FLAC Support**: Converts 24-bit FLAC files to 16-bit FLAC using SoX
- 🍎 **ALAC Support**: Converts ALAC (.m4a) files to FLAC format
  - 16-bit 44.1kHz/48kHz ALAC files are converted to FLAC with the same quality
  - Hi-Res ALAC files are converted to 16-bit FLAC following the same rules as FLAC files
- 📉 Downsamples high sample rate files:
  - 384kHz, 192kHz, or 96kHz → 48kHz
  - 352.8kHz, 176.4kHz, 88.2kHz → 44.1kHz
- 🔄 Preserves existing 16-bit FLAC files without unnecessary conversion
- 📝 Preserves ID3 tags and cover art from original files using FFmpeg (default: enabled; use --no-preserve-metadata to disable)
- 🎶 Copies MP3 files without modification
- 🖼️ Optional: Copies JPG and PNG images from the source directory
- 🐳 Docker support for containerized execution
- 💻 Cross-platform: Windows, macOS, Linux (x64, ARM64, x86, ARM)

## Installation

### Quick Install (Unix-like systems)

For Linux and macOS, you can use the installation script:

```bash
curl -sSL https://raw.githubusercontent.com/Ardakilic/lilt/main/install.sh | bash
```

Or download and run it manually:

```bash
wget https://raw.githubusercontent.com/Ardakilic/lilt/main/install.sh
chmod +x install.sh
./install.sh
```

### Download Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/Ardakilic/lilt/releases) page:

- **Windows**: `lilt-windows-amd64.exe` (x64) or `lilt-windows-arm64.exe` (ARM64)
- **macOS**: `lilt-darwin-amd64` (Intel) or `lilt-darwin-arm64` (Apple Silicon)
- **Linux**: `lilt-linux-amd64` (x64), `lilt-linux-arm64` (ARM64), `lilt-linux-386` (x86), or `lilt-linux-arm` (ARM)

### Build from Source

```bash
git clone https://github.com/Ardakilic/lilt.git
cd lilt
go build -o lilt .
```

## Requirements

You can use this tool in one of two ways:

1. **Using Docker (recommended)**:
   - Docker must be installed on your system
   - No local SoX installation required
   - Uses [`ardakilic/sox_ng:latest`](https://hub.docker.com/r/ardakilic/sox_ng) by default, which includes both sox_ng and FFmpeg.

2. **Using Local Installation**:
   - **SoX (Sound eXchange)** must be installed. [SoX Project](http://sox.sourceforge.net/)
     - Install on Debian/Ubuntu: `sudo apt install sox`
     - Install on macOS: `brew install sox`
     - Install on Windows: Use WSL and install depending on the subsystem, or download SoX Windows binaries
   - **FFmpeg** must be installed for ALAC support and metadata preservation. [FFmpeg Downloads](https://ffmpeg.org/download.html)
     - Install on Debian/Ubuntu: `sudo apt install ffmpeg`
     - Install on macOS: `brew install ffmpeg`
     - Install on Windows: Download from official site or use package manager
   - You can also use SoX-NG: A drop-in replacement for SoX ([SoX-NG Project](https://codeberg.org/sox_ng/sox_ng/))

## Usage

```bash
lilt <source_directory> [options]
```

### Options:

```
--target-dir <dir>   Specify target directory (default: ./transcoded)
--copy-images        Copy JPG and PNG files
--no-preserve-metadata  Do not preserve ID3 tags and cover art using FFmpeg (default: false)
--use-docker         Use Docker to run Sox instead of local installation
--docker-image <img> Specify Docker image (default: ardakilic/sox_ng:latest)
--self-update        Check for updates and self-update if newer version available
```

### Examples:

Using local SoX installation:
```bash
# Windows
lilt.exe "C:\Music\MyAlbum" --target-dir "C:\Music\MyAlbum-16bit" --copy-images

# macOS/Linux
./lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --copy-images
```

Using Docker:
```bash
# Windows
lilt.exe "C:\Music\MyAlbum" --target-dir "C:\Music\MyAlbum-16bit" --use-docker

# macOS/Linux
./lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --use-docker
```

Check for updates:
```bash
lilt --self-update
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
lilt ~/Music/MyAlbum --use-docker --docker-image your/sox-image:tag
```

Alternative Docker images you can use:
- `bigpapoo/sox`: Another SoX Docker image
- Any image that provides SoX installed as the `sox` command

## How It Works

1. The tool scans the source directory recursively for `.flac`, `.m4a` (ALAC), and `.mp3` files
2. **For FLAC files:**
   - If a FLAC file is **24-bit**, it is converted to **16-bit** using SoX
   - If a FLAC file has a sample rate of **96kHz, 192kHz, or 384kHz**, it is downsampled to **48kHz**
   - If a FLAC file has a sample rate of **88.2kHz**, it is downsampled to **44.1kHz**
   - 16-bit FLAC files at 44.1kHz or 48kHz are copied without conversion
3. **For ALAC files (.m4a):**
   - All ALAC files are converted to FLAC format using FFmpeg
   - 16-bit 44.1kHz/48kHz ALAC files are converted to FLAC maintaining the same quality
   - Hi-Res ALAC files follow the same bit depth and sample rate conversion rules as FLAC files
4. ID3 tags and cover art are preserved from source to converted files using FFmpeg (unless --no-preserve-metadata is used)
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

For detailed development information, including advanced build options, testing procedures, and contribution guidelines, see [Development.md](Development.md).

### Quick Start

#### Requirements

- Go 1.24.5 or later
- Make (optional, for convenience)

#### Building

```bash
# Clone the repository
git clone https://github.com/Ardakilic/lilt.git
cd lilt

# Build for current platform
go build -o lilt .

# Or use Make
make build
```

#### Testing

```bash
# Run tests
go test -v ./...

# Or use Make
make test
```

### Setting Version for Self-Update

To enable the self-update feature, set the version during build:

```bash
# Set specific version
go build -ldflags="-X main.version=v1.2.3" -o lilt .

# Use git tags (recommended)
make build  # Automatically uses git describe for versioning
```

See [Development.md](Development.md) for detailed version management and build options.

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
./lilt ~/Music/Album --target-dir ~/Music/Album-16bit
```

All command-line arguments remain the same for seamless migration.

## License

This project is open-source under the MIT License.

## Author

Arda Kilicdagi

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

