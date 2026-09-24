## ADDED Requirements

### Requirement: CLI exposes an embed-cover-art flag

The lilt CLI SHALL expose a boolean flag `--embed-cover-art` on the root command. When the flag is omitted, the default value SHALL be `false` and lilt SHALL behave exactly as before (byte-identical copies, no sibling-image lookup, no FFmpeg invocation on the copy path).

#### Scenario: Flag defaults to off

- **WHEN** the user runs `lilt <source>` without `--embed-cover-art`, with a sibling `cover.jpg` present
- **THEN** the copied target file is byte-identical to the source and no `Embedding cover art` log line is emitted

#### Scenario: Flag is explicitly enabled

- **WHEN** the user runs `lilt <source> --embed-cover-art`
- **THEN** every copied audio file triggers a sibling-image lookup and, when an image is found, an embed attempt

### Requirement: Sibling image resolution follows strict priority order

When the flag is enabled, for each copied audio file lilt SHALL look in the **source file's own directory** (`filepath.Dir(sourcePath)`) for the first existing file in this exact order: `cover.jpg` → `cover.jpeg` → `cover.png` → `front.jpg` → `front.jpeg` → `front.png` → `folder.jpg` → `folder.jpeg` → `folder.png`. Matching SHALL be case-insensitive (`Cover.JPG` matches `cover.jpg`). Only regular files count. Unsupported extensions (`.webp`, `.bmp`, `.gif`, …) SHALL be treated as absent. Each directory resolves independently (no parent-directory fallback, no cross-folder leakage).

#### Scenario: Highest-priority image wins

- **WHEN** a source folder contains `folder.jpg`, `front.png`, and `cover.jpg`
- **THEN** `cover.jpg` is embedded into every copied audio file from that folder

#### Scenario: PNG fallback within same basename

- **WHEN** a source folder contains only `cover.png` (no `cover.jpg`/`cover.jpeg`)
- **THEN** `cover.png` is embedded

#### Scenario: Case-insensitive match

- **WHEN** a source folder contains `Cover.JPG` (uppercase)
- **THEN** it is treated as `cover.jpg` and embedded

#### Scenario: Independent per-folder resolution

- **WHEN** `album/disc1/` contains `cover.jpg` and `album/disc2/` contains `front.jpg`
- **THEN** copies from `disc1` embed `disc1/cover.jpg` and copies from `disc2` embed `disc2/front.jpg`

#### Scenario: No image found

- **WHEN** a source folder contains no recognized image
- **THEN** the copied file is left byte-identical (silent no-op, no warning, run continues)

### Requirement: Embed applies to copied audio files only

The embed step SHALL apply to every audio file (`.flac`, `.mp3`, `.m4a`) that is written via the copy path — MP3 passthrough, 16-bit FLAC passthrough, 16-bit ALAC passthrough, MP3-kept-as-MP3 under format enforcement, audio-info-failure fallbacks, and conversion-failure fallbacks (the 12 audio `copyFile()` call sites in `main.go`) — and SHALL NOT apply to transcoded outputs (which keep `mergeMetadataWithFFmpeg()` behavior) nor to image files copied by `--copy-images`.

#### Scenario: 16-bit FLAC passthrough gains art

- **WHEN** a 16-bit/44.1kHz FLAC needs no conversion and its folder has `cover.jpg`
- **THEN** the target `.flac` contains the embedded picture and the audio stream is unchanged

#### Scenario: MP3 passthrough gains art

- **WHEN** an MP3 is copied and its folder has `front.png`
- **THEN** the target `.mp3` contains an APIC frame with the image and the audio stream is unchanged

#### Scenario: Transcoded file is untouched by this flag

- **WHEN** a 24-bit/96kHz FLAC is transcoded (SoX + merge path)
- **THEN** its art comes solely from the existing `mergeMetadataWithFFmpeg()` behavior, even with `--embed-cover-art` set

#### Scenario: Copied images are never targets

- **WHEN** `--copy-images` and `--embed-cover-art` are both set
- **THEN** `.jpg`/`.png` files are copied normally and never passed through the embed step

### Requirement: Embed replaces existing art without re-encoding audio

When an image is embedded, lilt SHALL replace any pre-existing embedded picture (result contains exactly one attached picture) and SHALL NOT re-encode audio (`-c:a copy`) or the image (`-c:v copy`). The invocation SHALL map only audio streams from the copied file plus the image stream (`-map 0:a -map 1:v` shape), with per-format flags: FLAC `–disposition:v attached_pic`; MP3 `-id3v2_version 3 -write_id3v1 1 -metadata:s:v title="Album cover" -metadata:s:v comment="Cover (front)"`; M4A `-disposition:v:0 attached_pic`. The embed SHALL write to a temp file and atomically rename over the target (never in-place), and SHALL restore file mode/mtime from the pre-embed copy.

#### Scenario: Existing embedded art is replaced, not duplicated

- **WHEN** a copied file already contains an embedded picture and the folder has a different `cover.jpg`
- **THEN** the target contains exactly one attached picture matching the folder image (verified via `ffprobe`: one `attached_pic` disposition stream)

#### Scenario: Audio is lossless through embed

- **WHEN** art is embedded into any supported container
- **THEN** audio codec, sample rate, bit depth, and duration are identical before and after (only picture/metadata streams change)

#### Scenario: Rerun is idempotent

- **WHEN** the embed step runs twice on the same target with the same folder image
- **THEN** the result still contains exactly one attached picture (no accumulation)

#### Scenario: Mode and mtime preserved

- **WHEN** art is embedded
- **THEN** the target's permission bits and modification time equal those of the pre-embed copy

### Requirement: Failures warn and preserve the copy, never abort the run

If no image is found, the image is corrupt/unsupported, FFmpeg is missing/fails, or the rename fails, lilt SHALL keep the pre-embed copied file and continue processing remaining files. Corrupt-image and FFmpeg failures SHALL emit a warning containing the image path, target path, and reason (`Warning: Failed to embed cover art from <image> into <target>: <err>, keeping copied audio without embedded art`). Missing-image SHALL be silent. When source and target are the same path, the embed SHALL be skipped silently (hardlinked copies with distinct paths are still embedded; the temp file + rename breaks the link instead of mutating the source).

#### Scenario: Corrupt image keeps copy

- **WHEN** `cover.jpg` contains invalid bytes
- **THEN** the target remains the byte-identical copy, a warning is logged, and the run completes successfully

#### Scenario: FFmpeg failure keeps copy

- **WHEN** FFmpeg exits non-zero during embed
- **THEN** any temp file is removed (best effort), the copy is preserved, a warning is logged, and the run continues

#### Scenario: In-place source==target skips embed

- **WHEN** source and target are the same path
- **THEN** no embed is attempted and no temp file residue remains

### Requirement: Safe interaction with hardlinks, metadata, images, format, and Docker flags

- With `--prefer-hardlinks`, the copy MAY hardlink first; the subsequent embed (temp + rename) SHALL break the link for embedded files so the source inode is never mutated. Files with no sibling image SHALL remain hardlinked.
- With `--no-preserve-metadata`, embed SHALL still run (orthogonal paths).
- Embedding SHALL NOT require `--copy-images`: lookup reads sibling images from the source directory directly and works whether or not `--copy-images` is set; when both flags are on, the image is both embedded and copied.
- With `--enforce-output-format`, lookup SHALL use the source directory and embed SHALL use the target container template (target ext may differ from source ext).
- With `--use-docker`, FFmpeg SHALL run via `docker run --rm --entrypoint ffmpeg` with both `-v <src>:/source` and `-v <tgt>:/target` mounts and translated paths; without Docker, local `ffmpeg` is required — if `--embed-cover-art` is set and `ffmpeg` is not found (and Docker is off), lilt SHALL fail fast in `setupSoxCommand()` with a clear error.
- The source file SHALL never be modified by the embed step under any flag combination.

#### Scenario: Hardlink source is never mutated

- **WHEN** `--prefer-hardlinks --embed-cover-art` are both set and a sibling image exists
- **THEN** after processing, source bytes are unchanged, source and target are no longer the same inode, and the target contains the embedded picture

#### Scenario: Embed works without copy-images

- **WHEN** `--embed-cover-art` is set without `--copy-images` and the folder has `cover.jpg`
- **THEN** the target audio still gains embedded art (the image itself is not copied to target)

#### Scenario: Missing ffmpeg fails fast

- **WHEN** `--embed-cover-art` is set, Docker is off, and `ffmpeg` is not on PATH
- **THEN** startup fails with an error naming `ffmpeg` and the flag

#### Scenario: Docker embed uses translated paths

- **WHEN** `--use-docker --embed-cover-art` are set
- **THEN** the FFmpeg invocation references `/source/...` for the image and `/target/...` for audio in/out with both volume mounts present

### Requirement: Logging follows repo conventions

On each successful embed lilt SHALL print `Embedding cover art from <imageBasename> into <targetPath>`. New and modified functions SHALL carry docblocks describing purpose, parameters, and hardlink/temp-file behavior (matching the `prefer-hardlinks` docblock requirement).

#### Scenario: Success log line

- **WHEN** `cover.jpg` is embedded into `target/01.flac`
- **THEN** output contains `Embedding cover art from cover.jpg into <target/01.flac>`

#### Scenario: Helpers documented

- **WHEN** `findCoverImage` / `embedImageIntoAudio` / `embedCoverArtIfRequested` (or equivalent names) are added
- **THEN** each has a docblock covering purpose, parameters, priority/temp-file semantics, and failure behavior
