#!/usr/bin/env bash

# Flac to 16bit converter
# This script converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
# It also copies MP3 files and image files (JPG, PNG) to the target directory.
# Usage: flac-converter.sh <source_directory> [options]
# https://github.com/Ardakilic/flac-to-16bit-converter
# Copyright (C) 2025 Arda Kilicdagi
# Licensed under MIT License

# Function to display usage
show_usage() {
    echo "Usage: $0 <source_directory> [options]"
    echo "Options:"
    echo "  --target-dir <dir>   Specify target directory (default: ./transcoded)"
    echo "  --copy-images        Copy JPG and PNG files"
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
        *)
            echo "Unknown option: $1"
            show_usage
            ;;
    esac
done

# Check if sox is installed
if ! command -v sox &> /dev/null; then
    echo "Error: sox is not installed. Please install sox to continue."
    exit 1
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
    local info=$(sox --i "$file")
    local bits=$(echo "$info" | grep "Sample Encoding" | grep -o "[0-9]\+")
    local rate=$(echo "$info" | grep "Sample Rate" | grep -o "[0-9]\+")
    echo "$bits $rate"
}

# Function to create target directory structure
create_target_dir() {
    local source_file="$1"
    local rel_path=$(dirname "${source_file#"$SOURCE_DIR"/}")
    local target_dir="$TRANSCODED_DIR/$rel_path"
    mkdir -p "$target_dir"
    echo "$target_dir"
}

# Process audio files
find "$SOURCE_DIR" \( -name "*.flac" -o -name "*.mp3" \) | while read -r file; do
    echo "Processing: $file"

    # Get target directory
    target_dir=$(create_target_dir "$file")
    target_file="$target_dir/$(basename "$file")"

    # Check if it's an MP3 file
    if [[ "$file" == *.mp3 ]]; then
        echo "Copying MP3 file: $file"
        cp "$file" "$target_file"
        continue
    fi
    
    # Process FLAC files
    read -r bits rate <<< "$(get_audio_info "$file")"

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
        # Debugging
        # echo "sox --multi-threaded -G '$file' $bitrate_args '$target_file' $sample_rate_args dither"
        # shellcheck disable=SC2086
        sox --multi-threaded -G "$file" $bitrate_args "$target_file" $sample_rate_args dither
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