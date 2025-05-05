# FLAC to 16-bit Converter

This script converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
It also copies MP3 files and image files (JPG, PNG) to the target directory if requested.

## Features

- Converts 24-bit FLAC files to 16-bit FLAC using SoX.
- Downsamples high sample rate files:
  - 384kHz, 192kHz, or 96kHz → 48kHz
  - 88.2kHz → 44.1kHz
- Preserves existing 16-bit FLAC files without unnecessary conversion.
- Copies MP3 files without modification.
- Optional: Copies JPG and PNG images from the source directory.

## Requirements

You can use this script in one of two ways:

1. **Using Docker (recommended)**:
   - Docker must be installed on your system
   - No local SoX installation required
   - Uses `ardakilic/sox_ng:latest` by default

2. **Using Local SoX Installation**:
   - **SoX (Sound eXchange)** must be installed. [SoX Project](http://sox.sourceforge.net/)
     - Install on Debian/Ubuntu: `sudo apt install sox`
     - Install on macOS: `brew install sox`
     - Install on Windows: Use WSL and install depending on the subsystem.
   - You can also use SoX-NG: A drop-in replacement for SoX ([SoX-NG Project](https://codeberg.org/sox_ng/sox_ng/))

## Usage

```bash
./flac-converter.sh <source_directory> [options]
```

### Options:

```
--target-dir <dir>   Specify target directory (default: ./transcoded)
--copy-images        Copy JPG and PNG files
--use-docker         Use Docker to run Sox instead of local installation
--docker-image <img> Specify Docker image (default: ardakilic/sox_ng:latest)
```

### Examples:

Using local sox installation:
```bash
./flac-converter.sh ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --copy-images
```

Using Docker:
```bash
./flac-converter.sh ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --use-docker
```

## Docker Support

When using the `--use-docker` option:

- Docker must be installed on your system
- The script mounts your source and target directories as volumes in the container
- No local SoX installation is required
- Uses `ardakilic/sox_ng:latest` by default, which is a containerized version of SoX-NG
- Source code of the Docker image is available [here](https://github.com/Ardakilic/sox_ng_dockerized)

You can specify a different Docker image with the `--docker-image` option:
```bash
./flac-converter.sh ~/Music/MyAlbum --use-docker --docker-image your/sox-image:tag
```

Alternative Docker images you can use:
- `bigpapoo/sox`: Another SoX Docker image
- Any image that provides SoX installed as the `sox` command

## How It Works

1. The script scans the source directory for `.flac` and `.mp3` files.
2. If a FLAC file is **24-bit**, it is converted to **16-bit** using SoX.
3. If a FLAC file has a sample rate of **96kHz or 192kHz**, it is downsampled to **48kHz**.
4. If a FLAC file has a sample rate of **88.2kHz**, it is downsampled to **44.1kHz**.
5. MP3 files are copied without modification.
6. If `--copy-images` is enabled, `.jpg` and `.png` files are copied to the target directory.

## Notes

- The script uses SoX's `--multi-threaded` option for performance.
- The `-G` flag ensures proper gain handling.
- Uses `dither` when downsampling to 16-bit for better quality.
- Creates the same folder structure in the target directory.

## License

This project is open-source under the MIT License.

## Author

Arda Kilicdagi

