# `--embed-cover-art`

The `--embed-cover-art` flag tells Lilt to embed a folder cover image (`cover` / `front` / `folder` JPEG or PNG) as cover art into copied audio files, replacing any existing embedded art.

## What it does

When `--embed-cover-art` is enabled, for every audio file that is **copied** (not transcoded), Lilt looks in the source file's own directory for a sibling cover image and embeds it into the target copy:

- Lookup runs per directory, using the source side (`filepath.Dir(sourcePath)`).
- The first existing image in priority order wins.
- The embed replaces any pre-existing embedded picture (result has exactly one attached picture).
- Audio is never re-encoded (`-c:a copy`); the image is never re-encoded (`-c:v copy`).
- If no image is found, the copy is left byte-identical (silent no-op).
- If the image is corrupt or FFmpeg fails, Lilt keeps the copied audio, prints a warning, and continues.

## Image resolution

Strict priority order, case-insensitive on basename and extension (`Cover.JPG` matches `cover.jpg`):

1. `cover.jpg`
2. `cover.jpeg`
3. `cover.png`
4. `front.jpg`
5. `front.jpeg`
6. `front.png`
7. `folder.jpg`
8. `folder.jpeg`
9. `folder.png`

Rules:

- Only regular files count.
- Unsupported extensions (`.webp`, `.bmp`, `.gif`, …) are treated as absent.
- Each directory resolves independently — there is no parent-directory fallback and no cross-folder leakage. `album/disc1/cover.jpg` applies to `disc1` tracks only.

## Per-format behavior

The embed maps only the audio stream(s) from the copied file plus the image stream (`-map 0:a -map 1:v` shape), so old art is dropped implicitly. Reruns are idempotent.

- **FLAC (`.flac`)**: image stored as a FLAC `PICTURE` block with `attached_pic` disposition.
- **MP3 (`.mp3`)**: image stored as an ID3v2.3 `APIC` frame (`-id3v2_version 3 -write_id3v1 1`, `title="Album cover"`, `comment="Cover (front)"`). Version 2.3 is forced for maximum player compatibility.
- **M4A/ALAC (`.m4a`)**: image stored as a `covr` atom with `attached_pic` disposition. No ID3 flags (the MP4 muxer ignores them).

## Scope

- **Copied audio only**: 16-bit FLAC passthrough, MP3 passthrough, 16-bit ALAC passthrough, MP3-kept-as-MP3 under `--enforce-output-format`, and copy fallbacks after conversion or metadata-read failures.
- **Not transcoded files**: files converted by SoX keep the existing `mergeMetadataWithFFmpeg()` behavior (source-embedded art preserved, folder image not injected), even with this flag set. In a mixed folder (one 24-bit FLAC transcoded + one 16-bit FLAC copied), the two outputs can end up with different covers.
- **Not image copies**: `.jpg` / `.png` files copied by `--copy-images` are never embed targets.

## Why use it

Many players and car stereos only show embedded art and ignore sibling `cover.jpg` / `folder.jpg` files. This flag gives every copied track portable cover art without changing the audio.

## Usage

```bash
lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --embed-cover-art
```

You can combine it with other flags:

```bash
lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --embed-cover-art --copy-images
lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --embed-cover-art --prefer-hardlinks
```

Success log line per embedded file:

```text
Embedding cover art from cover.jpg into /path/to/target/01.flac
```

Failure keeps the copy and warns:

```text
Warning: Failed to embed cover art from /path/to/source/cover.jpg into /path/to/target/01.flac: <reason>, keeping copied audio without embedded art
```

## Flag interactions

- `--copy-images`: independent, **not** required. Lookup reads the image from the source directory directly. With both flags on, the image is embedded into each track *and* copied to the target tree.
- `--prefer-hardlinks`: the copy may hardlink first, then the embed writes to a temp file next to the target and atomically renames over it, which **breaks** the hardlink for embedded files (target becomes an independent file). Files with no sibling image stay hardlinked. The source inode is never mutated.
- `--no-preserve-metadata`: orthogonal. Copied files never go through the FFmpeg metadata-merge path anyway; the embed is the only metadata write on the copy path, so it still runs.
- `--enforce-output-format`: lookup uses the source directory; the embed template follows the *target* container type (target extension may differ from source extension).
- `--use-docker`: FFmpeg runs inside the container with both source (`/source`) and target (`/target`) mounts. Without Docker, a local `ffmpeg` binary is required — if `--embed-cover-art` is set and `ffmpeg` is not on `PATH`, Lilt fails fast at startup with an error naming the flag.
- Source == target: the embed is skipped silently (no temp residue), mirroring the copy guard.

## Size guidance

The folder image is embedded into **every** track, so size adds up: a ~500 KB `cover.jpg` across a 15-track album adds ~7.5 MB to the target tree (versus one shared `folder.jpg`). Prefer a small baseline JPEG (≤1000px) for the folder image. There is no resizing or format conversion in v1 and no size warning threshold.

## Player compatibility

FFmpeg-written M4A `covr` art is invisible in some old players. If you hit such a player, remux the art with `mutagen` or `AtomicParsley` as a workaround — audio does not need re-encoding.

MP3 art is written as ID3v2.3 (not v2.4) for the same reason: old car stereos, iTunes versions, and Explorer handle v2.3 more reliably.

## Requirements

- A sibling `cover` / `front` / `folder` image (`.jpg` / `.jpeg` / `.png`) next to the source audio.
- `ffmpeg` on `PATH`, or `--use-docker` with an image containing `ffmpeg`.
- Supported audio targets: `.flac`, `.mp3`, `.m4a`. Other files are never targets.

## Examples

### Embed art into copied tracks

```bash
lilt ~/Music/Album --target-dir ~/Music/Album-16bit --embed-cover-art
```

### Embed art and also copy the image file

```bash
lilt ~/Music/Album --target-dir ~/Music/Album-16bit --embed-cover-art --copy-images
```

### Embed art on the same filesystem

```bash
lilt /mnt/music/library --target-dir /mnt/music/library-16bit --embed-cover-art --prefer-hardlinks
```

## See also

- [README.md](../README.md) for general usage
- `--copy-images` for copying album artwork files
- `--prefer-hardlinks` for hardlinking unchanged files
- `--no-preserve-metadata` for disabling metadata preservation on transcoded files
