#!/bin/bash

# FLAC to 16-bit Converter - Installation Script
# This script downloads and installs the latest release

set -e

# Configuration
REPO="Ardakilic/flac-to-16bit-converter"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
BINARY_NAME="flac-converter"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Detect OS and architecture
detect_platform() {
    local os
    local arch
    
    # Detect OS
    case "$(uname -s)" in
        Linux*)     os="linux";;
        Darwin*)    os="darwin";;
        *)          print_error "Unsupported operating system: $(uname -s)"; exit 1;;
    esac
    
    # Detect architecture
    case "$(uname -m)" in
        x86_64)     arch="amd64";;
        amd64)      arch="amd64";;
        arm64)      arch="arm64";;
        aarch64)    arch="arm64";;
        armv7l)     arch="arm";;
        i386)       arch="386";;
        i686)       arch="386";;
        *)          print_error "Unsupported architecture: $(uname -m)"; exit 1;;
    esac
    
    echo "${os}-${arch}"
}

# Get latest release version
get_latest_version() {
    curl -s "https://api.github.com/repos/${REPO}/releases/latest" | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/'
}

# Download and install
install_binary() {
    local platform="$1"
    local version="$2"
    
    print_status "Detected platform: ${platform}"
    print_status "Latest version: ${version}"
    
    # Construct download URL
    local filename="${BINARY_NAME}-${platform}.tar.gz"
    local url="https://github.com/${REPO}/releases/download/${version}/${filename}"
    
    print_status "Downloading ${filename}..."
    
    # Create temporary directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "${tmp_dir}"' EXIT
    
    # Download and extract
    if ! curl -sL "${url}" | tar -xz -C "${tmp_dir}"; then
        print_error "Failed to download or extract ${filename}"
        exit 1
    fi
    
    # Find the binary
    local binary_path
    binary_path=$(find "${tmp_dir}" -name "${BINARY_NAME}*" -type f | head -1)
    
    if [[ ! -f "${binary_path}" ]]; then
        print_error "Binary not found in downloaded archive"
        exit 1
    fi
    
    # Make it executable
    chmod +x "${binary_path}"
    
    # Install to target directory
    print_status "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."
    
    if [[ ! -w "${INSTALL_DIR}" ]]; then
        print_warning "Installing to ${INSTALL_DIR} requires sudo"
        sudo cp "${binary_path}" "${INSTALL_DIR}/${BINARY_NAME}"
    else
        cp "${binary_path}" "${INSTALL_DIR}/${BINARY_NAME}"
    fi
    
    print_status "Installation complete!"
    print_status "Run '${BINARY_NAME} --help' to get started"
}

# Main installation function
main() {
    print_status "Installing FLAC to 16-bit Converter..."
    
    # Check dependencies
    for cmd in curl tar; do
        if ! command -v "$cmd" &> /dev/null; then
            print_error "$cmd is required but not installed"
            exit 1
        fi
    done
    
    # Detect platform
    local platform
    platform=$(detect_platform)
    
    # Get latest version
    local version
    version=$(get_latest_version)
    
    if [[ -z "${version}" ]]; then
        print_error "Failed to get latest version"
        exit 1
    fi
    
    # Install
    install_binary "${platform}" "${version}"
}

# Handle command line arguments
case "${1:-}" in
    --help|-h)
        echo "FLAC to 16-bit Converter Installation Script"
        echo ""
        echo "Usage: $0 [options]"
        echo ""
        echo "Options:"
        echo "  --help, -h    Show this help message"
        echo ""
        echo "Environment variables:"
        echo "  INSTALL_DIR   Installation directory (default: /usr/local/bin)"
        echo ""
        echo "Examples:"
        echo "  $0                           # Install to /usr/local/bin"
        echo "  INSTALL_DIR=~/bin $0         # Install to ~/bin"
        exit 0
        ;;
    *)
        main
        ;;
esac
