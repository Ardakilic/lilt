#!/usr/bin/env bash

# Flac to 16bit converter
# This script converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
# It also copies MP3 files and image files (JPG, PNG) to the target directory.
# Usage: flac-converter.sh <source_directory> [options]
# https://github.com/Ardakilic/flac-to-16bit-converter
# Copyright (C) 2025 Arda Kilicdagi
# Licensed under MIT License

# Default values
USE_DOCKER=false
DOCKER_IMAGE="ardakilic/sox_ng:latest"
SOX_COMMAND="sox"

# Function to display usage
show_usage() {
    echo "Usage: $0 <source_directory> [options]"
    echo "Options:"
    echo "  --target-dir <dir>   Specify target directory (default: ./transcoded)"
    echo "  --copy-images        Copy JPG and PNG files"
    echo "  --use-docker         Use Docker to run Sox instead of local installation"
    echo "  --docker-image <img> Specify Docker image (default: ardakilic/sox_ng:latest)"
    exit 1
}

# Check if source directory is provided
if [ $# -lt 1 ]; then
    show_usage
fi

SOURCE_DIR="$1"
shift  # Remove first argument

# Default values
TRANSCODED_DIR="./transcoded"
COPY_IMAGES=false

# Parse remaining arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --target-dir)
            if [[ -n $2 ]]; then
                TRANSCODED_DIR="$2"
                shift 2
            else
                echo "Error: --target-dir requires a directory path"
                exit 1
            fi
            ;;
        --copy-images)
            COPY_IMAGES=true
            shift
            ;;
        --use-docker)
            USE_DOCKER=true
            shift
            ;;
        --docker-image)
            if [[ -n $2 ]]; then
                DOCKER_IMAGE="$2"
                shift 2
            else
                echo "Error: --docker-image requires an image name"
                exit 1
            fi
            ;;
        *)
            echo "Unknown option: $1"
            show_usage
            ;;
    esac
done

# Setup the appropriate SOX command
if [ "$USE_DOCKER" = true ]; then
    # Check if docker is installed
    if ! command -v docker &> /dev/null; then
        echo "Error: Docker is not installed. Please install Docker to use this option."
        exit 1
    fi
    
    # Get absolute paths for mounting
    SOURCE_DIR_ABS=$(cd "$(dirname "$SOURCE_DIR")" || exit 1; pwd -P)/$(basename "$SOURCE_DIR")
    TRANSCODED_DIR_ABS=$(cd "$(dirname "$TRANSCODED_DIR")" || exit 1; pwd -P)/$(basename "$TRANSCODED_DIR")
    
    # Setup Docker related variables
    declare -a SOX_DOCKER=(docker run --rm -v "$SOURCE_DIR_ABS:/source" -v "$TRANSCODED_DIR_ABS:/target" "$DOCKER_IMAGE")
    
    # Convert paths to Docker container paths
    SOURCE_DIR_DOCKER="/source"
    TRANSCODED_DIR_DOCKER="/target"
else
    # Check if sox is installed locally
    if ! command -v "$SOX_COMMAND" &> /dev/null; then
        echo "Error: sox is not installed. Please install sox or use --use-docker option."
        exit 1
    fi
    # Use local paths
    SOURCE_DIR_DOCKER="$SOURCE_DIR"
    TRANSCODED_DIR_DOCKER="$TRANSCODED_DIR"
fi

# Check if source directory exists
if [ ! -d "$SOURCE_DIR" ]; then
    echo "Error: Source directory does not exist"
    exit 1
fi

# Create base transcoded directory
mkdir -p "$TRANSCODED_DIR"

# Function to get audio file info using sox
get_audio_info() {
    local file="$1"
    local docker_file="$2"
    local info
    
    if [ "$USE_DOCKER" = true ]; then
        info=$("${SOX_DOCKER[@]}" --i "$docker_file")
    else
        info=$("$SOX_COMMAND" --i "$file")
    fi
    
    local bits
    local rate
    bits=$(echo "$info" | grep "Sample Encoding" | grep -o "[0-9]\+")
    rate=$(echo "$info" | grep "Sample Rate" | grep -o "[0-9]\+")
    echo "$bits $rate"
}

# Function to create target directory structure
create_target_dir() {
    local source_file="$1"
    local rel_path
    rel_path=$(dirname "${source_file#"$SOURCE_DIR"/}")
    local target_dir="$TRANSCODED_DIR/$rel_path"
    mkdir -p "$target_dir"
    echo "$target_dir"
}

# Function to get Docker path
get_docker_path() {
    local host_path="$1"
    local rel_path="${host_path#"$SOURCE_DIR"}"
    echo "$SOURCE_DIR_DOCKER$rel_path"
}

# Function to get Docker target path
get_docker_target_path() {
    local host_path="$1"
    local rel_path="${host_path#"$TRANSCODED_DIR"}"
    echo "$TRANSCODED_DIR_DOCKER$rel_path"
}

# Process audio files
find "$SOURCE_DIR" \( -name "*.flac" -o -name "*.mp3" \) | while read -r file; do
    echo "Processing: $file"

    # Get target directory
    target_dir=$(create_target_dir "$file")
    target_file="$target_dir/$(basename "$file")"
    
    # Get Docker paths if needed
    if [ "$USE_DOCKER" = true ]; then
        docker_file=$(get_docker_path "$file")
        docker_target=$(get_docker_target_path "$target_file")
    else
        docker_file="$file"
        docker_target="$target_file"
    fi

    # Check if it's an MP3 file
    if [[ "$file" == *.mp3 ]]; then
        echo "Copying MP3 file: $file"
        cp "$file" "$target_file"
        continue
    fi
    
    # Process FLAC files
    read -r bits rate <<< "$(get_audio_info "$file" "$docker_file")"

    # Determine if conversion is needed
    needs_conversion=false
    bitrate_args=""                             # Base bitrate conversion arguments
    sample_rate_args="rate -v -L"               # Base rate conversion arguments

    # Check bit depth
    if [ "$bits" -gt 16 ]; then
        needs_conversion=true
        bitrate_args="-b 16"
    fi

    # Check sample rate
    if [ "$rate" -eq 96000 ] || [ "$rate" -eq 192000 ] || [ "$rate" -eq 384000 ]; then
        needs_conversion=true
        sample_rate_args="$sample_rate_args 48000"
    elif [ "$rate" -eq 88200 ]; then
        needs_conversion=true
        sample_rate_args="$sample_rate_args 44100"
    fi

    # Process file
    if [ "$needs_conversion" = true ]; then
        echo "Converting FLAC: $file"
        if [ "$USE_DOCKER" = true ]; then
            # Use the Docker array for conversion
            "${SOX_DOCKER[@]}" --multi-threaded -G "$docker_file" $bitrate_args "$docker_target" $sample_rate_args dither
        else
            # Use local sox command
            # shellcheck disable=SC2086
            "$SOX_COMMAND" --multi-threaded -G "$file" $bitrate_args "$target_file" $sample_rate_args dither
        fi
    else
        echo "Copying FLAC: $file"
        cp "$file" "$target_file"
    fi
done

# Copy image files if requested
if [ "$COPY_IMAGES" = true ]; then
    echo "Copying image files..."
    find "$SOURCE_DIR" -type f \( -name "*.jpg" -o -name "*.png" \) | while read -r img_file; do
        target_dir=$(create_target_dir "$img_file")
        cp "$img_file" "$target_dir/"
    done
fi

echo "Processing complete!"