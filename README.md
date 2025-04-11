# FLAC to 16-bit Converter

This is a Bash script for transcoding and downsampling 24-bit FLAC files to 16-bit FLAC using SoX. The script is compatible with Linux, Windows (via WSL or Git Bash), and macOS.

## Features

- Converts 24-bit FLAC files to 16-bit FLAC using SoX.
- Downsamples high sample rate files:
  - 384kHz, 192kHz, or 96kHz → 48kHz
  - 88.2kHz → 44.1kHz
- Preserves existing 16-bit FLAC files without unnecessary conversion.
- Copies MP3 files without modification.
- Optional: Copies JPG and PNG images from the source directory.

## Requirements

- **SoX (Sound eXchange)** must be installed. [SoX Project](http://sox.sourceforge.net/)
  - Install on Debian/Ubuntu: `sudo apt install sox`
  - Install on macOS: `brew install sox`
  - Install on Windows: Use WSL and install depending on the subsystem.

You can also use alternative SoX implementations:
- Docker image: `bigpapoo/sox`
- SoX-NG: A drop-in replacement for SoX ([SoX-NG Project](https://codeberg.org/sox_ng/sox_ng/))
- My Docker image for SoX-NG: `ardakilic/sox_ng`. Source code of the Image is [here](https://github.com/Ardakilic/sox_ng_dockerized).

## Usage

Run the script with the source directory as an argument:

```bash
./flac-converter.sh <source_directory> [options]
```

### Options:

- `--target-dir <dir>` : Specify the target directory (default: `./transcoded`).
- `--copy-images` : Copy JPG and PNG image files.

### Environment Variables:

- `SOX_COMMAND`: Override the default `sox` command (default: "sox")

### Example Usage

Convert FLAC files from `Music/HiRes` and store the transcoded files in `Music/Converted`:

```bash
./flac-converter.sh Music/HiRes --target-dir Music/Converted
```

Convert FLAC files and copy images:

```bash
./flac-converter.sh Music/HiRes --copy-images
```

Using with Docker:

```bash
SOX_COMMAND="docker run -v $(pwd):/work --rm bigpapoo/sox mysox" ./flac-converter.sh Music/HiRes
# or my builds of sox_ng, which is better maintained and up-to-date
SOX_COMMAND="docker run --rm -v $(pwd):/audio ardakilic/sox_ng" ./flac-converter.sh Music/HiRes
```

Using with SoX-NG directly:

```bash
SOX_COMMAND="sox_ng" ./flac-converter.sh Music/HiRes
```

## How It Works

1. The script scans the source directory for `.flac` and `.mp3` files.
2. If a FLAC file is **24-bit**, it is converted to **16-bit** using SoX.
3. If a FLAC file has a sample rate of **96kHz or 192kHz**, it is downsampled to **48kHz**.
4. If a FLAC file has a sample rate of **88.2kHz**, it is downsampled to **44.1kHz**.
5. MP3 files are copied without modification.
6. If `--copy-images` is enabled, `.jpg` and `.png` files are copied to the target directory.

## Notes

- The script uses SoX’s `--multi-threaded` option for performance.
- The `-G` flag ensures proper gain handling.
- Uses `dither` when downsampling to 16-bit for better quality.
- Creates the same folder structure in the target directory.

## License

This project is open-source under the MIT License.

## Author

Arda Kilicdagi

