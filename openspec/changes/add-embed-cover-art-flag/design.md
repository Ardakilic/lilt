## Context

Lilt is a single-file Go CLI (`main.go`, ~1270 lines, deps: `cobra`/`pflag` only per `go.mod`) that walks a source music directory and either **transcodes** (SoX audio + FFmpeg metadata merge) or **copies** (`copyFile()` → `doCopyFile()` byte copy, optionally hardlink via `--prefer-hardlinks`).

```
┌──────────────────┐      ┌─────────────────────┐      ┌──────────────────┐
│  source library  │      │   lilt decision      │      │ target library   │
│                  │      │                      │      │                  │
│ album/           │─────▶│ needsConversion? ────┼──Yes─▶│ SoX transcode    │
│  01.flac (24/96) │      │ ALAC? enforce fmt?   │      │ + FFmpeg merge   │
│  02.flac (16/44) │─────▶│                      ├──No──▶│ copyFile()  ◀── NEW: embed
│  cover.jpg       │      │                      │      │ 01.flac (16/48)  │  step runs
│  front.png       │      │ sibling image is     │      │ 02.flac (+art)   │  here
│  folder.jpg      │      │ NEVER consulted      │      │ cover.jpg (only  │
└──────────────────┘      └─────────────────────┘      │  if --copy-images)│
                                                       └──────────────────┘
```

Current art handling (verified by codebase analysis + Context7 docs):

- **Transcoded files**: `mergeMetadataWithFFmpeg(source, temp, target)` at `main.go:1096-1144` runs `ffmpeg -i source -i temp -map 1 -map 0:v? -map_metadata 0 -c copy target`. This *preserves* source-embedded art (exposed as `0:v?`), never injects sibling JPGs. Documented in `Development.md:341-360`.
- **Copied files**: `copyFile()` at `main.go:1260-1273` (with `sameFile` guard, `PreferHardlinks` branch, `doCopyFile` fallback at `main.go:1196-1237`). Zero FFmpeg involvement — art preserved byte-identical, sibling images ignored. All 12 audio copy call sites (`main.go:264, 271, 304, 308, 326, 334, 364, 372, 401, 420, 438, 976`) plus image copies (`main.go:1178`, excluded from embed targets).
- **Sibling images**: `copyImageFiles()` at `main.go:1148-1180` copies `.jpg`/`.png` (case-insensitive, no `.jpeg`) preserving tree, only when `--copy-images` is set. Runs after `processAudioFiles()` in `runConverter()` (`main.go:142-151`).
- **Tooling** (Context7-verified): FFmpeg official docs (`/websites/ffmpeg_documentation`) confirm the embed recipe — `ffmpeg -i INPUT -i pic -map 0:a -map 1 -disposition:v attached_pic OUTPUT` for FLAC, `-c copy -map 0 -map 1 -metadata:s:v title="Album cover" -metadata:s:v comment="Cover (Front)"` for MP3, `-map 0 -map 1 -c copy -c:v:1 png -disposition:v:1 attached_pic` for MP4-family. SoX/sox_ng (`/git_codeberg_org/sox_ng_sox_ng`, `/rbouqueau/sox`) is audio-only and drops art — hence the SoX→FFmpeg two-step pattern this change reuses. Cobra (`/spf13/cobra`) pattern is `rootCmd.Flags().BoolVar(&config.X, "kebab-case", false, "help")`, matching `main.go:80-87`.

Naming: the flag is `--embed-cover-art` (`Config.EmbedCoverArt`). The working name `--embed-copied-image` was rejected because "copied image" reads as "an image copied via `--copy-images`", implying a prerequisite that does not exist — embedding reads sibling images from the source directory directly and works with or without `--copy-images`.

## Goals / Non-Goals

**Goals:**

- Add opt-in `--embed-cover-art` flag (default `false`).
- For every **copied** audio file, resolve a sibling image in the *source* folder in strict priority order and embed it into the *target copy*, replacing existing embedded art.
- Do it losslessly (`-c:a copy`, never re-encode audio; `-c:v copy`, never re-encode the image unless a muxer forces it).
- Work for `.flac`, `.mp3`, `.m4a` targets, locally and under `--use-docker`.
- Never mutate the source file — even when `--prefer-hardlinks` created a hardlink (embed via temp + atomic rename, which breaks the link).
- Warn-not-fatal on every failure path (no image, corrupt image, FFmpeg error): keep the copy, continue the run.
- Cover with unit + integration tests; document with docblocks, `rootCmd.Long`, `README.md`, `docs/embed-cover-art.md`.

**Non-Goals:**

- Changing transcoded-file behavior (`mergeMetadataWithFFmpeg` untouched).
- Re-encoding audio or normalizing/converting image formats (no resizing, no JPEG↔PNG conversion, no WebP/BMP support) — except where an FFmpeg muxer strictly requires it (document, don't silently convert).
- Embedding into non-audio files (images copied by `--copy-images` are never targets).
- Recursive/parent-folder search (only the file's own directory; no walking up to parent album art).
- Refactoring the conversion pipeline or adding Go image/tag libraries (`mutagen`, `taglib`, etc.). Shell out to the already-required `ffmpeg`.
- Touching SoX/sox_ng invocation.

## Decisions

### 1. Scope = copied files only, resolved from the source folder

**Decision:** The embed step applies to every audio file that flows through `copyFile()` (the 12 audio call sites listed in Context), and to no other path. The lookup directory is `filepath.Dir(sourcePath)` (source side), not the target side.

**Rationale:** The feature request names "copied files". Transcoded files already have a merge step with different semantics (preserve source-embedded art); mixing external-image injection into that step would change default art for Hi-Res conversions and complicate the SoX→FFmpeg ordering. Source-side lookup is required because `--enforce-output-format` renames extensions on the target side (`main.go:357, 397, 414`: `.flac`→`.m4a` etc.), while the image always lives next to the source. "Copied" here means audio files written via the copy path (no transcoding required) — it does NOT mean images copied via `--copy-images`, and `--copy-images` is not a prerequisite (see Decision 7).

**Consequence (document):** In a mixed folder (one 24-bit FLAC transcoded + one 16-bit FLAC copied), the transcoded file keeps source-embedded art while the copied sibling gets folder-image art — divergent covers are possible. Call this out in docs; a follow-up change could unify the behavior.

### 2. Strict 6-entry priority order, case-insensitive, with `.jpeg` alias

**Decision:** Check in exactly this order, first existing regular file wins:

```
cover.jpg → cover.png → front.jpg → front.png → folder.jpg → folder.png
```

Matching is case-insensitive on both basename and extension (`Cover.JPG`, `FRONT.Png` match). Additionally accept `cover.jpeg`, `front.jpeg`, `folder.jpeg` as aliases immediately after their `.jpg` counterpart (i.e. `cover.jpg` → `cover.jpeg` → `cover.png` → …). Rationale: `.jpeg` is the same codec as `.jpg` and Windows rippers emit it; `copyImageFiles()` (`main.go:1160-1163`) only copies `.jpg`/`.png`, but the *embed* lookup should not miss a valid JPEG just because of its extension spelling. Do NOT add `.webp`/`.bmp`/`.gif`: FFmpeg FLAC/M4A muxers officially support JPEG/PNG only (Context7: "only support a few formats, like JPEG or PNG"; FLAC muxer: "exactly one FLAC audio stream… additionally images with disposition attached_pic"). If siblings include only unsupported types, behave as "no image found".

**Alternative considered:** basename-only priority (any `cover.*` beats any `front.*` regardless of extension). Rejected — the request lists `cover.jpg/png, front.jpg/png, folder.jpg/png in this order`, which reads as the 6-entry sequence; it is also deterministic and trivially testable.

### 3. Replace, never duplicate — map only `0:a` + image

**Decision:** The embed FFmpeg invocation maps **only the audio stream(s)** from the copied file plus the image stream, so any pre-existing embedded picture is dropped in the same pass (no separate strip step):

- Base shape (all formats): `ffmpeg -y -i <copiedAudio> -i <image> -map 0:a -map 1:v -c:a copy -c:v copy <format flags> <tmpOut>`
- FLAC (`.flac`): add `-disposition:v attached_pic`. (Context7: FLAC muxer accepts `attached_pic`; disposition required for players to see it as cover, not video.)
- MP3 (`.mp3`): add `-id3v2_version 3 -write_id3v1 1 -metadata:s:v title="Album cover" -metadata:s:v comment="Cover (front)"`. (Matches yt-dlp production recipe + FFmpeg docs; v3 for max player compat — car stereos/old iTunes/Explorer. Note FFmpeg docs example uses `Cover (Front)` capital-F; subagent research shows canonical APIC type 3 string is `Cover (front)` lowercase-f. Implement lowercase-f, verify with `ffprobe` in tests.)
- M4A/ALAC (`.m4a`): add `-disposition:v:0 attached_pic` (indexed form; bare `-disposition:v` warns when multiple streams exist). Stores as `covr` atom, not a video track. No ID3 flags (MP4 muxer ignores them).

**Why not `-map 0`:** bare `-map 0` carries the old `attached_pic` forward → duplicate covers (verified in FFmpeg community guidance). `-map 0:a` strips it implicitly. Reruns are therefore idempotent: embedding twice yields one picture.

### 4. Temp + rename, never in-place; re-apply mode/mtime

**Decision:** FFmpeg always writes to a temp file next to the target (`<target>.embed.tmp.<ext>` or reuse the `.tmp.` convention), then `os.Rename(tmp, target)`. Never pass the same path as input and output. After a successful embed, re-apply source mode/mtime (`os.Chmod`/`os.Chtimes` mirroring `doCopyFile` at `main.go:1227-1234`), because the FFmpeg rewrite otherwise loses them.

**Rationale:** FFmpeg cannot edit in place safely. Temp+rename additionally solves the `--prefer-hardlinks` hazard: if `copyFile()` created a hardlink (shared inode), writing a new file + renaming replaces the *target directory entry* without touching the source inode. An in-place embed would corrupt the user's source library.

### 5. Integrate at the copy call sites via a wrapper, not inside `copyFile()`

**Decision:** Do NOT put embed logic inside `copyFile()` (which is also used for `.jpg`/`.png` themselves at `main.go:1178`). Instead add:

```go
// findCoverImage returns the highest-priority sibling image for a source dir.
func findCoverImage(sourceDir string) (string, bool)

// embedImageIntoAudio embeds imagePath into audioPath (FLAC/MP3/M4A aware,
// local + Docker), via temp + rename. Returns error on any FFmpeg failure.
func embedImageIntoAudio(audioPath, imagePath string) error

// embedCoverArtIfRequested is called after a successful audio copy when
// config.EmbedCoverArt is true. No-op when flag off, target is not audio,
// source==target (sameFile), or no sibling image found.
func embedCoverArtIfRequested(sourceAudioPath, targetAudioPath string)
```

Call `embedCoverArtIfRequested(source, target)` after each successful audio `copyFile()` (12 sites), or centrally at the two funnel points that cover them: end of `processAudioFiles()` walk callback's copy branches + `processAudioFileWithEnforcedFormat`/`processTo*` copy branches. Prefer explicit per-branch calls (visible, greppable) over hidden magic inside `copyFile()` — the image-copy path must never embed.

**Alternative considered:** centralize in `copyFile()` with an `ext` check. Rejected: `copyFile` has no knowledge of source-vs-target image dirs, and a future caller could copy audio for a different reason. Explicit wrapper keeps the "copied audio" semantic.

### 6. Docker parity via existing path translators

**Decision:** Under `config.UseDocker`, translate paths with the existing `getDockerPath()` (source side, `/source/...`) and `getDockerTargetPath()` (target side, `/target/...`) helpers (`main.go:1042-1091`), and mount both `-v <src>:/source -v <tgt>:/target` exactly as `mergeMetadataWithFFmpeg` does (`main.go:1109-1112`). The image input uses the `/source` mapping; audio input + output use `/target` mapping. Local `ffmpeg` availability is checked in `setupSoxCommand()` (`main.go:196-198`); extend that gate so the flag requires `ffmpeg` (or Docker) and fails fast with a clear error if missing.

### 7. Flag interaction matrix (normative)

| Other flag | Behavior with `--embed-cover-art` |
|---|---|
| (flag off, default) | Zero behavior change. No lookup, no FFmpeg, byte-identical copies. |
| `--prefer-hardlinks` | Copy may hardlink first (unchanged), then embed runs via temp+rename, which **breaks** the link for files that get art (target becomes an independent file). Files with no sibling image stay hardlinked. Log both `Created hardlink` and `Embedding cover art…` lines. Never mutate source inode (assert with `os.SameFile` in tests). |
| `--no-preserve-metadata` | Orthogonal. Copied files never went through `mergeMetadataWithFFmpeg` anyway; embed still runs (it is the *only* metadata write on the copy path). |
| `--copy-images` | Orthogonal. Lookup works whether or not `--copy-images` is set (image is *read* from source; `--copy-images` only controls whether it is *also* copied to target). Both flags on → image is embedded AND copied. |
| `--enforce-output-format` | Lookup uses source dir; embed handles the *target* container type (target ext may differ from source ext). `processToFLAC` MP3-passthrough (`main.go:364`) and `processToALAC` MP3-passthrough (`main.go:438`) are copy paths → embed applies with MP3 template. |
| `--use-docker` | FFmpeg runs via `docker run --rm --entrypoint ffmpeg …` with dual volume mounts (Decision 6). |
| source == target (`sameFile`) | Skip embed entirely (return nil), mirroring `copyFile()` guard at `main.go:1261-1263`. |

### 8. Logging (follow existing conventions)

- Success: `Embedding cover art from <imageName> into <targetPath>\n` (basename of image, full target path).
- No image: silent (no per-file spam across 15-track albums) — or debug-level only. Do not emit warnings for the common no-art case.
- Corrupt image / FFmpeg failure: `Warning: Failed to embed cover art from <image> into <target>: <err>, keeping copied audio without embedded art\n` and keep the pre-embed copy. Never return an error that aborts `processAudioFiles()` (mirror merge-fallback style at `main.go:626-632`).
- Docblocks on all new/unexported functions (repo convention per `design.md:17` of previous change).

## Implementation Sketch

### Config + flag (pattern at `main.go:25-35`, `79-91`)

```go
type Config struct {
    // ... existing ...
    PreferHardlinks  bool
    EmbedCoverArt bool // NEW
}

rootCmd.Flags().BoolVar(
    &config.EmbedCoverArt,
    "embed-cover-art",
    false,
    "Embed folder cover image (cover/front/folder.jpg/png) as cover art into copied audio files (replaces existing embedded art)",
)
```

Also extend `rootCmd.Long` (`main.go:57-72`) with a paragraph for the new flag (precedent: `--prefer-hardlinks` paragraph at `67-69`).

### Lookup helper

```go
// orderedCoverNames is the strict priority order for sibling image lookup.
var orderedCoverNames = []string{
    "cover.jpg", "cover.jpeg", "cover.png",
    "front.jpg", "front.jpeg", "front.png",
    "folder.jpg", "folder.jpeg", "folder.png",
}

// findCoverImage returns the first existing sibling image in sourceDir
// according to orderedCoverNames. Matching is case-insensitive: it first
// tries the exact names, then falls back to a case-insensitive directory
// scan. The second return value is false when no image is found.
func findCoverImage(sourceDir string) (string, bool)
```

Implementation notes: `os.Stat` each candidate (fast path, exact case); if none hit, `os.ReadDir(sourceDir)` once and compare `strings.ToLower(entry.Name())` against the lowercased candidate list in order. Only regular files (`!IsDir`). Return the joined path.

### Embed helper (per-format FFmpeg)

```go
// embedImageIntoAudio embeds imagePath as cover art into audioPath,
// replacing any existing embedded picture without re-encoding audio.
// It writes to a temp file and renames over audioPath, then restores
// file mode/mtime from the pre-embed copy.
func embedImageIntoAudio(audioPath, imagePath string) error
```

Pseudo-logic:

1. `ext := strings.ToLower(filepath.Ext(audioPath))`; switch `flac/mp3/m4a`, else return error (should not happen — caller filters).
2. `tmp := strings.TrimSuffix(audioPath, ext) + ".embed.tmp" + ext`.
3. Build args per Decision 3 (local). Under Docker, map `audioPath→getDockerTargetPath`, `imagePath→getDockerPath`, `tmp→getDockerTargetPath`, prefix `docker run --rm --entrypoint ffmpeg -v src:/source -v tgt:/target <image>`.
4. `cmd.Run()`; on error remove tmp (best effort) and return wrapped error.
5. Capture pre-embed `Stat` (mode/mtime) before FFmpeg; after success `Chmod`/`Chtimes` tmp→ then `os.Rename(tmp, audioPath)`.
6. Caller (`embedCoverArtIfRequested`) logs warning and keeps original copy on error.

FFmpeg templates (final, replace-safe):

```bash
# FLAC
ffmpeg -y -i <target.flac> -i <cover.jpg|png> \
  -map 0:a -map 1:v -c:a copy -c:v copy \
  -disposition:v attached_pic <tmp.flac>
# MP3
ffmpeg -y -i <target.mp3> -i <cover.jpg|png> \
  -map 0:a -map 1:v -c copy \
  -id3v2_version 3 -write_id3v1 1 \
  -metadata:s:v title="Album cover" \
  -metadata:s:v comment="Cover (front)" <tmp.mp3>
# M4A/ALAC
ffmpeg -y -i <target.m4a> -i <cover.jpg|png> \
  -map 0:a -map 1:v -c:a copy -c:v copy \
  -disposition:v:0 attached_pic <tmp.m4a>
```

Verify each with `ffprobe -show_streams <out> | grep attached_pic` expecting `DISPOSITION:attached_pic=1`; MP3 additionally `TAG:title=Album cover`.

### Wrapper + call sites

```go
// embedCoverArtIfRequested embeds a sibling folder image into a copied
// audio file when config.EmbedCoverArt is set. It is a no-op when the
// flag is off, the target is not .flac/.mp3/.m4a, src and dst are the same
// file, or no sibling image exists. Failures are logged as warnings and
// the copied file is preserved.
func embedCoverArtIfRequested(sourceAudioPath, targetAudioPath string)
```

Call after every successful audio `copyFile()`: `main.go:264` (MP3 passthrough), `:271` + `:334` (audio-info failure fallback — only when ext is audio), `:304` (conversion-failure fallback), `:308` (16-bit FLAC passthrough), `:326` (enforced-mp3 passthrough), `:364`/`:438` (enforced MP3-kept-as-MP3), `:372` (enforced 16-bit FLAC), `:401` (toMP3 passthrough), `:420` (16-bit ALAC passthrough), `:976` (`processFlac` passthrough). NOT at `:1178` (image copy). Simplest robust alternative: single call inside the walk callbacks right after `copyFile` returns nil for audio exts — implementer may choose either as long as all 12 sites are covered and image copies excluded; tasks require a coverage test per branch family.

### `setupSoxCommand` gate (`main.go:157-224`)

When `config.EmbedCoverArt` is true, require FFmpeg availability (same `exec.LookPath("ffmpeg")` check used for metadata preservation at `main.go:196-198`, skipped under Docker where only `docker` is checked). Error message: `embed-cover-art requires ffmpeg (or --use-docker with an image containing ffmpeg)`.

## Test Strategy

Mirror previous change's conventions (`t.TempDir()`, config save/restore, `osLink` injection, `os.SameFile`, stdout capture, `go test ./...`, `go vet`, `gofmt`):

- `TestFindCoverImagePriority`: fixture dir with all six names (+ uppercase + `.jpeg` variants in separate subdirs); assert winner per order; assert `( "", false)` on empty dir; assert subdir images don't leak across dirs.
- `TestFindCoverImageCaseInsensitive`: `Cover.JPG` / `FRONT.Png` resolve.
- `TestEmbedCoverArtFlagOff`: flag off + sibling `cover.jpg` present → target byte-identical to source (`bytes.Equal`), no FFmpeg invoked (optionally stub exec or assert no `.embed.tmp` residue).
- `TestEmbedCoverArtNoImage`: flag on, no image → copy succeeds, no warning-as-error, content identical.
- `TestEmbedCoverArtFLAC/MP3/M4A` (integration, requires `ffmpeg` binary; skip with `t.Skip` if `exec.LookPath("ffmpeg")` fails): craft minimal audio (generate via `ffmpeg -f lavfi -i anullsrc` or `sox -n`), sibling `cover.jpg` (generate 1x1 JPEG via Go stdlib or testdata), run wrapper, assert `ffprobe -show_streams` shows `attached_pic=1` and audio stream params unchanged (duration/codec).
- `TestEmbedReplacesExistingArt`: source audio WITH embedded art (pre-bake via FFmpeg) + different sibling image → after embed exactly ONE attached_pic stream and image bytes differ from original (compare `ffprobe` stream count / extracted image hash).
- `TestEmbedCorruptImage`: invalid JPEG bytes as `cover.jpg` → wrapper warns, copied audio preserved (readable, size == pre-embed copy).
- `TestEmbedWithPreferHardlinks`: `PreferHardlinks=true` + flag on → after embed, `!os.SameFile(srcInfo, dstInfo)` (link broken by rename), source bytes unchanged (hash equal), target has art.
- `TestEmbedSameFile`: src==dst → no-op, no error, no temp residue.
- `TestEmbedPreservesModeMtime`: record mode/mtime of copy, embed, assert equal after.
- Full suite `go test ./...` must pass; existing copy/metadata tests unbroken.

Testdata: small committed fixtures under `testdata/embed/` (1s silent `.flac`/`.mp3`/`.m4a`, 1x1 `cover.jpg`/`front.png`) OR generate on the fly with `ffmpeg`/`sox` if available; prefer checked-in tiny fixtures so CI without audio tools still runs lookup/off-path tests, with FFmpeg-dependent tests skipping gracefully.

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **Per-file bloat**: same ~0.5 MB image embedded into every track (15 tracks ≈ +7 MB vs 1 shared `folder.jpg`). | Inherent to embedded art; document expected size growth and recommend small (≤1000px, JPEG) folder images. No resizing in v1. |
| **Hardlink silently broken**: with both flags on, embedded files become independent inodes (space saving lost for those files). | Document explicitly; log both lines so it's visible. Alternative (skip embed to preserve link) would silently drop requested art — worse. |
| **M4A/covr player quirks**: FFmpeg-written `covr` is invisible in some old players (yt-dlp issues #2125/#411 note `mutagen`/`AtomicParsley` as more compatible). | Accept FFmpeg (already a dependency); document `mutagen`/`AtomicParsley` escape hatch for problem players. Verify with `ffprobe` + at least one real player in manual QA. |
| **MP3 ID3v2.3 vs v2.4 compat**: v4 default breaks old players; forcing v3 + `write_id3v1` maximizes compat but rewrites tag version even for files that had v2.4. | Follow yt-dlp precedent (`-id3v2_version 3 -write_id3v1 1`); document the choice. |
| **Progressive-JPEG / odd PNG sources** may fail the mux. | Warn-and-keep-copy fallback; suggest baseline JPEG/RGB PNG in docs and warning text. |
| **Docker dual-mount assumption**: audio (`/target`) + image (`/source`) in one FFmpeg invocation requires both `-v` mounts (already the case in merge path). | Reuse exact mount construction from `mergeMetadataWithFFmpeg`; add Docker-mode integration test or manual QA step. |
| **Log volume**: per-file `Embedding…` lines on big libraries. | Keep success line concise (one per embedded file); stay silent when no image found. No summary needed for v1. |
| **Divergent art in mixed folders** (transcoded vs copied). | Document as known limitation; propose follow-up (`--embed-transcoded-image` or unify) as non-goal. |

## Migration Plan

No migration. Purely additive, opt-in, default off.

1. Merge implementation + tests + docs.
2. Release notes mention `--embed-cover-art` with the priority order and hardlink interaction.
3. Users opt in per-run; existing commands unchanged.

## Open Questions

- Should a future flag also inject folder images into **transcoded** files (replacing preserved source art), or merge both (prefer folder image, fall back to embedded)? Currently out of scope — flag which behavior users actually want after v1 ships.
- Should lookup support `cover.jpeg` (proposed yes as alias) — confirm maintainer preference, and whether `copyImageFiles()` should also copy `.jpeg` for consistency (separate change if desired).
- MP3 comment string casing: `Cover (front)` (canonical APIC type-3, recommended) vs `Cover (Front)` (FFmpeg docs example). Propose lowercase-f; confirm via `ffprobe` round-trip in implementation.
- M4A map form: `-map 0:a -map 1:v` (replace-safe, proposed) vs `-map 0 -map 1` (FFmpeg MP4 thumbnail example). Propose `0:a`; verify single-`covr` output with `ffprobe` during implementation.
- Per-file size warning threshold: should we warn when the folder image exceeds e.g. 2 MB (would bloat every track)? Propose no threshold in v1, docs guidance only.
