## 1. Configuration and CLI flag

- [x] 1.1 Add `EmbedCoverArt bool` field to the `Config` struct in `main.go` (after `PreferHardlinks`, pattern at `main.go:25-35`).
- [x] 1.2 Register `--embed-cover-art` boolean flag in `init()` using `rootCmd.Flags().BoolVar`, default `false`, help text such as `"Embed folder cover image (cover/front/folder.jpg/png) as cover art into copied audio files (replaces existing embedded art)"` (pattern at `main.go:80-87`).
- [x] 1.3 Extend `rootCmd.Long` (`main.go:57-72`) with a paragraph documenting the flag, priority order, and hardlink interaction (precedent: `--prefer-hardlinks` paragraph at `main.go:67-69`).
- [x] 1.4 Extend `setupSoxCommand()` (`main.go:157-224`): when `config.EmbedCoverArt` is true and Docker is off, require `exec.LookPath("ffmpeg")` and return a clear error naming the flag if missing (mirror check at `main.go:196-198`).

## 2. Sibling image lookup

- [x] 2.1 Add `orderedCoverNames` package var: `cover.jpg, cover.jpeg, cover.png, front.jpg, front.jpeg, front.png, folder.jpg, folder.jpeg, folder.png` with a docblock stating the strict priority.
- [x] 2.2 Add `findCoverImage(sourceDir string) (string, bool)` with docblock: exact-case `os.Stat` fast path over candidates, then single `os.ReadDir` case-insensitive fallback (`strings.ToLower` compare), regular-files only, returns joined path + found flag.
- [x] 2.3 Unit tests: `TestFindCoverImagePriority` (all six + winner assertions, subdir isolation, empty-dir `false`), `TestFindCoverImageCaseInsensitive` (`Cover.JPG`, `FRONT.Png`), `TestFindCoverImageJpegAlias` (`.jpeg` accepted), `TestFindCoverImageIgnoresUnsupported` (`.webp`/`.bmp` only → `false`).

## 3. Per-format FFmpeg embed (local + Docker)

- [x] 3.1 Add `embedImageIntoAudio(audioPath, imagePath string) error` with docblock: ext switch (`.flac`/`.mp3`/`.m4a`), temp path `<target>.embed.tmp.<ext>`, pre-embed `Stat` for mode/mtime, per-format args (FLAC `-disposition:v attached_pic`; MP3 `-id3v2_version 3 -write_id3v1 1 -metadata:s:v title="Album cover" -metadata:s:v comment="Cover (front)"`; M4A `-disposition:v:0 attached_pic`), base `-y -i audio -i image -map 0:a -map 1:v -c:a copy -c:v copy`, `cmd.Run`, temp cleanup on error, `Chmod`/`Chtimes` + `os.Rename` on success.
- [x] 3.2 Add Docker branch: `getDockerPath(image)` for the image input, `getDockerTargetPath(audio/tmp)` for audio in/out, `docker run --rm --entrypoint ffmpeg -v <src>:/source -v <tgt>:/target <image>` prefix (mirror `mergeMetadataWithFFmpeg` at `main.go:1104-1121`).
- [x] 3.3 Verify templates manually once with `ffprobe -show_streams` (`attached_pic=1`; MP3 `title=Album cover`): FLAC/JPEG, FLAC/PNG, MP3/JPEG, MP3/PNG, M4A/JPEG, M4A/PNG — record results in the PR description.

## 4. Copy-path integration (wrapper + call sites)

- [x] 4.1 Add `embedCoverArtIfRequested(sourceAudioPath, targetAudioPath string)` with docblock: no-op when flag off / ext not in {`.flac`,`.mp3`,`.m4a`} / same path (in-place run) / no sibling image (silent); else `findCoverImage(filepath.Dir(source))` → `embedImageIntoAudio(target, image)` → success log `Embedding cover art from <base> into <target>` / failure warning `Warning: Failed to embed cover art from <image> into <target>: <err>, keeping copied audio without embedded art`, never returns fatal error. Deliberately does NOT skip hardlinked copies (same inode, different paths): temp + rename breaks the link instead of mutating the source.
- [x] 4.2 Wire the wrapper after every successful audio `copyFile()`: `main.go:264` (MP3 passthrough), `:271`/`:334` (audio-info fallback, audio exts only), `:304` (conversion-failure fallback), `:308` (16-bit FLAC), `:326` (enforced-mp3), `:364`/`:438` (enforced MP3-kept), `:372` (enforced 16-bit FLAC), `:401` (toMP3 passthrough), `:420` (16-bit ALAC), `:976` (`processFlac` passthrough). Explicitly NOT at `:1178` (image copy).
- [x] 4.3 Add coverage test proving each branch family reaches the wrapper (e.g. table test over passthrough/fallback paths with flag on + fixture image, asserting `attached_pic` present; plus negative test that `copyImageFiles()` never embeds).

## 5. Interaction tests

- [x] 5.1 `TestEmbedCoverArtFlagOff`: flag off + `cover.jpg` present → target byte-identical to source, no `Embedding` output.
- [x] 5.2 `TestEmbedReplacesExistingArt`: pre-embed art + different folder image → exactly one `attached_pic` stream post-embed (ffprobe stream count), extracted image hash differs from original.
- [x] 5.3 `TestEmbedCoverArtNoImage` / `TestEmbedSameFile`: silent no-op, content unchanged, no temp residue (`*.embed.tmp.*` absent).
- [x] 5.4 `TestEmbedCorruptImage` + `TestEmbedFFmpegFailure` (unreadable image / stubbed ffmpeg error): warning logged, copy preserved, run continues (no returned fatal error).
- [x] 5.5 `TestEmbedWithPreferHardlinks`: both flags on → pre-embed hardlink broken post-embed (`!os.SameFile`), source hash unchanged, target has art; and no-image case stays hardlinked (`os.SameFile` true).
- [x] 5.6 `TestEmbedPreservesModeMtime`: mode + mtime equal pre/post embed.
- [x] 5.7 Audio-lossless assertion in each format integration test: codec/rate/duration identical pre/post (ffprobe compare); skip gracefully via `t.Skip` when `ffmpeg`/`ffprobe` absent.

## 6. Fixtures, regression, docs

- [x] 6.1 Add tiny fixtures under `testdata/embed/` (≤5s silent `.flac`/`.mp3`/`.m4a` with and without pre-baked art; 1×1 `cover.jpg`/`front.png`/corrupt `cover.jpg`) OR generate on the fly with ffmpeg/sox when available — lookup/off-path unit tests must pass without external binaries.
- [x] 6.2 Run full suite `go test ./...` and fix regressions (especially existing `TestCopyFile*`, metadata-merge, and `processAudioFiles` tests).
  - Note: `TestProcessALAC`, `TestGetALACInfoError`, `TestSetupSoxCommandEdgeCases/LocalModeWithALACFiles` fail identically on the clean tree (verified via `git stash -u`) because they assume ffmpeg/sox are absent while this machine now has them installed. Pre-existing environmental failures, not regressions from this change.
- [x] 6.3 Run `go vet ./...`; run `gofmt -w main.go main_test.go`.
- [x] 6.4 Docs: update `README.md` (Options block `101-112`, Features `11-30`, How It Works `192-228`), add `docs/embed-cover-art.md` (priority order, per-format behavior, hardlink/docker/metadata interactions, size guidance, M4A caveat + mutagen/AtomicParsley escape hatch) following `docs/prefer-hardlinks.md` precedent, link it from README.
- [x] 6.5 Manual QA matrix (record in PR): local FLAC/MP3/M4A × JPEG/PNG; `--prefer-hardlinks` combined; `--enforce-output-format` × each format (ext-rename case); `--use-docker` at least one format; player spot-check (one desktop + one mobile/car if available) + `ffprobe` output pasted.
  - Done except player spot-check (no test device available; `ffprobe` disposition/tag verification is the objective check): local FLAC/JPEG, FLAC/PNG, MP3/JPEG, M4A/JPEG e2e + unit; `--prefer-hardlinks` combined e2e (link broken, source intact); `--enforce-output-format alac` e2e (m4a passthrough + mp3-kept both embedded); `--use-docker` e2e with `ardakilic/sox_ng:latest` (mp3 + flac embedded via containerized ffmpeg).
