## Why

Lilt preserves embedded cover art when it **transcodes** (via `mergeMetadataWithFFmpeg()` in `main.go:1096-1144`: `ffmpeg -i source -i temp -map 1 -map 0:v? -map_metadata 0 -c copy target`), and it byte-copies files that need no conversion (16-bit FLAC, passthrough MP3/ALAC, fallback copies). But many libraries ship cover art only as **sibling files** (`cover.jpg`, `front.jpg`, `folder.jpg`) with **no embedded picture** in the audio itself. Copied files therefore arrive in the target library with no embedded art, even though the art was right next to them in the source folder. Players that only read embedded art (most mobile/car players) show a blank cover.

A new opt-in flag `--embed-cover-art` closes this gap: after a file is copied (not transcoded), look for a sibling image in the source directory in priority order and embed it into the **copied target file**, replacing any pre-existing embedded picture.

## What Changes

- Add a new CLI flag `--embed-cover-art` to the root `lilt` command (default `false`, opt-in).
- When enabled, every audio file that goes through the **copy path** (`copyFile()` call sites for `.flac`/`.mp3`/`.m4a`) gets a post-copy embed step:
  1. Resolve the source file's directory (`filepath.Dir(sourcePath)`).
  2. Look for the first existing file in strict order: `cover.jpg` → `cover.png` → `front.jpg` → `front.png` → `folder.jpg` → `folder.png` (case-insensitive match, see design for `.jpeg` handling).
  3. If found, embed it into the **target copy** with FFmpeg (`-c:a copy`, lossless audio), **replacing** any existing embedded picture (map only `0:a` from audio + image stream, never bare `-map 0`).
  4. If no image is found, or the image is unreadable/corrupt, keep the byte-identical copy and continue (warn, never fail the run).
- Scope is **copied files only** (see design for the full call-site list). Transcoded outputs keep their existing `mergeMetadataWithFFmpeg()` behavior unchanged.
- Independent of `--copy-images`: the lookup reads sibling images straight from the source directory, so embedding works whether or not `--copy-images` is set (`--copy-images` only controls whether image files are additionally copied to the target).
- Support local and `--use-docker` FFmpeg execution, all three output containers (FLAC/MP3/M4A), and safe interaction with `--prefer-hardlinks` (embed via temp + rename so the source inode is never mutated), `--no-preserve-metadata`, `--copy-images`, and `--enforce-output-format`.
- Update `rootCmd.Long` help, `README.md`, add `docs/embed-cover-art.md`, and add comprehensive unit/integration tests.

## Capabilities

### New Capabilities

- `embed-cover-art`: Opt-in embedding of a sibling folder image into copied (non-transcoded) audio files, with basename priority `cover` > `front` > `folder`, replacing existing embedded art.

### Modified Capabilities

- None at the spec level. Default behavior (flag off) is byte-identical to today. When the flag is on, only the copy path gains a post-copy FFmpeg step; the transcode pipeline is untouched.

## Impact

- **User-facing**: New `--embed-cover-art` flag. Example: `lilt ~/Music/MyAlbum --target-dir ~/Music/MyAlbum-16bit --embed-cover-art`.
- **Behavioral**: Copied `.flac`/`.mp3`/`.m4a` files whose source folder contains a recognized image gain an embedded picture (previous embedded picture, if any, is replaced). Files without a sibling image are unchanged. Each track in one folder embeds the same folder image (per-file size cost, same as if every file had embedded art upstream).
- **Logging**: New `Embedding cover art from <image> into <target>` success lines and `Warning: ... keeping copied audio without embedded art` lines on failure (warning-not-fatal, matching existing merge-fallback style at `main.go:626-632`).
- **Code**: `main.go` (`Config`, `init()`, copy path / new helpers `findCoverImage`, `embedImageIntoAudio` or similar, Docker path helpers) and `main_test.go`. No new Go module dependencies (shell out to the already-required `ffmpeg`).
- **Compatibility**: No breaking changes. Requires `ffmpeg` (local) or Docker image with `ffmpeg` entrypoint when the flag is on — same requirement already imposed by metadata preservation. SoX/sox_ng behavior unchanged (SoX drops art; this flag only touches the copy path, never the SoX path).
