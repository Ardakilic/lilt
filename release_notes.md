# 🎵 FLAC to 16-bit Converter v2.0.0

## 🚀 Major Release: Complete Go Rewrite

This is a complete rewrite of the FLAC to 16-bit converter from Bash to Go, providing better performance, cross-platform compatibility, and enhanced reliability.

## ✨ What's New

### 🔄 **Complete Go Implementation**
- **Cross-platform binaries** for Windows, macOS, and Linux (x64, ARM64, x86, ARM)
- **Native performance** without shell dependencies
- **Robust error handling** with detailed feedback
- **Memory efficient** processing of large audio collections

### 🛠️ **Enhanced Features**
- **Improved file copying** with permission and timestamp preservation
- **Better Docker integration** with automatic path handling
- **Progress indicators** and detailed logging
- **Comprehensive test coverage** ensuring reliability

### 📦 **Easy Installation**
- **Pre-built binaries** for all major platforms
- **One-line installer** for Unix-like systems
- **No compilation required** for end users

## 📥 Installation

### Quick Install (Linux/macOS)
```bash
curl -sSL https://raw.githubusercontent.com/Ardakilic/flac-to-16bit-converter/main/install.sh | bash
```

### Manual Download
Download the appropriate binary for your system:
- **Windows**: `flac-converter-windows-amd64.zip`
- **macOS**: `flac-converter-darwin-amd64.tar.gz` (Intel) / `flac-converter-darwin-arm64.tar.gz` (Apple Silicon)
- **Linux**: `flac-converter-linux-amd64.tar.gz` (x64) / `flac-converter-linux-arm64.tar.gz` (ARM64)

## 🎯 Key Improvements

### **Performance**
- ⚡ **Faster processing** with native Go implementation
- 🧠 **Lower memory usage** compared to shell script version
- 🔧 **Better resource management** with proper cleanup

### **Reliability**
- ✅ **Comprehensive error handling** for edge cases
- 🔒 **Data integrity** with proper file sync operations
- 🛡️ **Graceful fallbacks** when conversion fails

### **Cross-Platform**
- 🖥️ **Windows support** with native executable
- 🍎 **macOS support** for both Intel and Apple Silicon
- 🐧 **Linux support** for multiple architectures
- 📱 **ARM support** for Raspberry Pi and other devices

## 📋 Usage

The command-line interface remains **100% compatible** with the original bash script:

```bash
# Basic usage
flac-converter /path/to/music --target-dir /path/to/output

# With Docker
flac-converter /path/to/music --use-docker --copy-images

# Custom Docker image
flac-converter /path/to/music --use-docker --docker-image custom/sox:latest
```

## 🔧 Technical Details

### **File Processing**
- Preserves original directory structure
- Maintains file permissions and timestamps
- Handles large files efficiently
- Robust Unicode filename support

### **Audio Conversion**
- **24-bit → 16-bit** conversion with proper dithering
- **Sample rate conversion**: 384/192/96kHz → 48kHz, 88.2kHz → 44.1kHz
- **Lossless copying** for files that don't need conversion
- **MP3 passthrough** without modification

### **Docker Integration**
- Automatic volume mounting with absolute paths
- Proper container cleanup
- Support for custom Docker images
- Cross-platform Docker path handling

## 🧪 Quality Assurance

- **Unit tests** covering core functionality
- **Cross-platform CI/CD** with GitHub Actions
- **Shell script validation** with shellcheck
- **Code quality** checks and formatting

## 🔄 Migration from v1.x

The Go version is a **drop-in replacement** for the bash script:

```bash
# Old (bash)
./flac-converter.sh ~/Music/Album --target-dir ~/Music/Album-16bit

# New (Go) - same arguments
./flac-converter ~/Music/Album --target-dir ~/Music/Album-16bit
```

## 🐛 Bug Fixes

- Fixed file permission handling across different filesystems
- Improved Docker path resolution on Windows
- Better error messages for missing dependencies
- Resolved Unicode filename issues

## 📚 Documentation

- Updated README with comprehensive installation instructions
- Added development guide for contributors
- Improved error message clarity
- Added troubleshooting section

## 🙏 Acknowledgments

- Thanks to the community for feedback and testing
- Special thanks to contributors who helped with cross-platform testing
- Docker image maintainers for SoX-NG support

## 🔗 Links

- **Documentation**: [README.md](https://github.com/Ardakilic/flac-to-16bit-converter#readme)
- **Issues**: [GitHub Issues](https://github.com/Ardakilic/flac-to-16bit-converter/issues)
- **Docker Image**: [ardakilic/sox_ng](https://hub.docker.com/r/ardakilic/sox_ng)

---

**Full Changelog**: https://github.com/Ardakilic/flac-to-16bit-converter/compare/v1.0.0...v2.0.0
