# Development Guide

This guide covers development workflows, building, testing, and deployment for Lilt.

## Setting Version During Build

The application version is embedded at build time using Go's `-ldflags` with the `-X` flag. This version is used by the `--self-update` feature to determine if updates are available.

### Method 1: Manual Version Setting

Set a specific version during build:

```bash
# Set version to v1.2.3
go build -ldflags="-X main.version=v1.2.3" -o lilt .

# Use environment variable
VERSION=v1.2.3 go build -ldflags="-X main.version=$VERSION" -o lilt .
```

### Method 2: Using Git Tags (Recommended)

The Makefile automatically uses git tags for versioning:

```bash
# Create and push a git tag
git tag v1.0.0
git push origin v1.0.0

# Build with automatic version detection
make build
# or
go build -ldflags="-X main.version=$(git describe --tags --always --dirty)" -o lilt .
```

The `git describe` command will output something like:
- `v1.0.0` (exact tag match)
- `v1.0.0-1-g1234567` (1 commit after tag)
- `v1.0.0-dirty` (uncommitted changes)

### Method 3: Using Makefile

The Makefile provides convenient targets:

```bash
# Build for current platform with git version
make build

# Build for all platforms
make build-all

# Clean and rebuild
make clean && make build
```

## Development Workflow

### Prerequisites

- Go 1.25.1 or later
- Git
- Make (optional, for convenience)
- Docker (for containerized testing)
- golangci-lint (for linting)

### Setting Up Development Environment

```bash
# Clone the repository
git clone https://github.com/Ardakilic/lilt.git
cd lilt

# Install dependencies
go mod download

# Run tests
make test
# or
go test -v ./...

# Build for development
make build
```

### Code Quality

```bash
# Format code
make fmt
# or
go fmt ./...

# Lint code
make lint
# or
golangci-lint run

# Run tests with coverage
go test -cover ./...
```

## Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Coverage

Current test coverage: ~68%

Key test areas:
- Version comparison logic
- Audio file processing
- Docker integration
- Self-update functionality
- Error handling

## Building for Different Platforms

### Single Platform Build

```bash
# Linux x64
GOOS=linux GOARCH=amd64 go build -ldflags="-X main.version=v1.0.0" -o lilt-linux-amd64 .

# Windows x64
GOOS=windows GOARCH=amd64 go build -ldflags="-X main.version=v1.0.0" -o lilt-windows-amd64.exe .

# macOS ARM64
GOOS=darwin GOARCH=arm64 go build -ldflags="-X main.version=v1.0.0" -o lilt-darwin-arm64 .

# Linux ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-X main.version=v1.0.0" -o lilt-linux-arm64 .
```

### Cross-Platform Build (Using Makefile)

```bash
# Build for all supported platforms
make build-all

# Create distribution packages
make package
```

This creates binaries in the `dist/` directory with proper naming and compression.

## Self-Update Feature

The self-update feature uses the embedded version to:

1. Check current version against GitHub releases
2. Download platform-specific binaries
3. Replace the running binary safely

### Version Format

The version should follow semantic versioning:
- `v1.0.0` - major.minor.patch
- `v1.0.0-rc.1` - release candidate
- `v1.0.0-dev` - development version

### Testing Self-Update

```bash
# Test with development version (skips update)
go build -ldflags="-X main.version=dev" -o lilt .
./lilt --self-update

# Test with specific version
go build -ldflags="-X main.version=v1.0.0" -o lilt .
./lilt --self-update
```

## Release Process

### Creating a Release

1. Update version in code/docs if needed
2. Create and push git tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. GitHub Actions will automatically build and create release
4. Update release notes with changes

### Pre-release Builds

For development/testing releases:

```bash
# Build with pre-release version
go build -ldflags="-X main.version=v1.0.0-rc.1" -o lilt .

# Or use git describe for automatic pre-release detection
make build  # Will include commit hash if not on exact tag
```

## Docker Development

### Building Docker Image

```bash
# Build the Docker image for testing
docker build -t lilt-test .

# Run tests in Docker
docker run --rm lilt-test go test ./...
```

### Using Docker for SoX

The application supports using Docker for SoX operations:

```bash
# Build with Docker support
./lilt /source/dir --use-docker --docker-image ardakilic/sox_ng:latest
```

## Debugging

### Enable Debug Output

The application uses standard Go logging. For more verbose output:

```bash
# Run with debug flags if implemented
./lilt --verbose /source/dir
```

### Common Issues

1. **Version not updating**: Ensure you're using `-ldflags="-X main.version=..."` during build
2. **Self-update fails**: Check network connectivity and GitHub API access
3. **Cross-platform builds**: Use correct GOOS/GOARCH combinations
4. **Docker issues**: Ensure Docker is running and accessible

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Ensure all tests pass: `make test`
5. Format code: `make fmt`
6. Submit pull request

## Recent Fixes and Improvements

### Cover Art Preservation Fix

Previously, when ID tags were copied using FFmpeg, cover images (album artwork) were not copied to the converted file. This has been fixed by adding video stream mapping to the FFmpeg command.

**Technical Details:**
- Added `-map 0:v?` parameter to copy video streams (cover art) from source file
- The `?` makes it optional, so the command won't fail if there are no video streams
- Cover art in FLAC files is stored as video streams, which is why this mapping was necessary

**FFmpeg Command Before:**
```bash
ffmpeg -i source.flac -i converted.flac -map 1 -map_metadata 0 -c copy output.flac
```

**FFmpeg Command After:**
```bash
ffmpeg -i source.flac -i converted.flac -map 1 -map 0:v? -map_metadata 0 -c copy output.flac
```

This fix applies to both local FFmpeg execution and Docker-based execution.

## CI/CD

The project uses GitHub Actions for:
- Automated testing on all platforms
- Cross-platform builds
- Release creation
- Code quality checks

Workflow files are in `.github/workflows/`.