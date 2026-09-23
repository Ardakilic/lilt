## Context

Lilt currently processes audio and standalone images as separate concerns. `processAudioFiles()` creates or converts FLAC, M4A/ALAC, and MP3 outputs, while `copyImageFiles()` copies JPG/PNG files as sidecars. The existing metadata merge preserves source cover art with FFmpeg's `-map 0:v?`, but no code associates a conventional sidecar image with a produced audio file.

The output extension is not always the source extension. Default ALAC processing writes FLAC, and `--enforce-output-format` can change FLAC, M4A/ALAC, and MP3 outputs. A sidecar operation therefore must use the actual final target path rather than deriving a new path from the source filename.

The recently completed hardlink feature is a hard constraint. Unchanged audio and image files may share source/target inodes when `--prefer-hardlinks` is enabled. Embedding artwork rewrites audio and must never write through such a hardlink.

## Goals / Non-Goals

**Goals:**

- Add an opt-in `--embed-copied-image` flag.
- Resolve artwork beside each source audio file with deterministic `cover` > `front` > `folder` precedence and JPG-before-PNG ordering.
- Support case-insensitive JPG/PNG names in the same directory without parent-directory search.
- Apply the selected sidecar to all successfully produced audio outputs, including direct copies, transcoded outputs, and format-enforced outputs.
- Replace existing embedded artwork when a sidecar is selected.
- Preserve current behavior when no sidecar is selected.
- Keep sidecar copying and sidecar embedding independently controllable through `--copy-images` and `--embed-copied-image`.
- Keep audio stream data unchanged while remuxing the selected image.
- Prevent embedding from modifying a source file through a hardlink.
- Keep local and Docker FFmpeg command behavior equivalent.
- Provide deterministic tests for selection, output paths, command construction, failure handling, and hardlink safety.

**Non-Goals:**

- Adding image formats beyond the existing `.jpg` and `.png` support.
- Searching parent directories for album artwork.
- Renaming standalone sidecar files to match converted audio extensions.
- Removing sidecar files after embedding them.
- Rewriting or broadening the behavior of `--no-preserve-metadata` for ordinary byte-for-byte copies that have no selected sidecar.
- Using SoX to attach artwork; FFmpeg remains the metadata and container-muxing layer.
- Automatically embedding arbitrary `track.jpg` files or every image copied by `--copy-images`.

## Decisions

### 1. Make the new flag independent from `--copy-images`

`--copy-images` will continue to control copying supported sidecar files. `--embed-copied-image` will control reading and embedding a sidecar. The flags may be used together, but neither will implicitly enable or disable the other.

This avoids a surprising dependency and permits users who want embedded artwork without retaining standalone copies in the target tree.

### 2. Use a deterministic same-directory resolver

The resolver will inspect only the source audio file's containing directory. It will test candidates in this exact order:

1. `cover.jpg`
2. `cover.png`
3. `front.jpg`
4. `front.png`
5. `folder.jpg`
6. `folder.png`

Names will be matched case-insensitively, and only regular files will be accepted. A selected `folder` image will apply to every audio file in that directory. Parent-directory inheritance is intentionally excluded from the initial scope because it introduces an implicit hierarchy and ambiguous override behavior.

The resolver will return the source image path. `--copy-images` will remain responsible for copying all supported JPG/PNG files, not just the selected candidate.

### 3. Track actual final audio outputs and perform a post-processing pass

The implementation will refactor the audio walk to retain an inventory of source and final target paths. The inventory will be populated from the actual branch that produced each output, including direct copies, transcodes, format-enforced outputs, and fallback copies.

The recommended pipeline is:

```text
process audio
    │
    ▼
source audio → final target audio inventory
    │
    ▼
resolve sidecar beside source audio
    │
    ├── no sidecar: leave current output untouched
    │
    └── sidecar found
            │
            ├── remux final target audio + sidecar
            ├── write temporary output
            └── replace target only after success
```

A post-processing pass is preferred over adding image parameters to every conversion helper because it covers all current direct-copy and transcoding branches with one final-output operation. It also avoids treating `copyFile()`, which is shared with image copying and hardlinks, as a media-muxing abstraction.

### 4. Use FFmpeg for final artwork attachment

SoX and SoX_ng are responsible for audio transformation in the existing pipeline, but FFmpeg is already the final metadata layer. The embedding pass will use the actual final target audio as its audio input and the selected source-sidecar image as its picture input.

When a sidecar is selected, the pass will map only the final audio stream and the selected image stream. It will not map the target's existing video/attached-picture streams, ensuring that the sidecar replaces rather than duplicates existing artwork.

The command will use format-aware options:

- FLAC: attach one or more images with `attached_pic` disposition, subject to FFmpeg's FLAC muxer support.
- MP3: map the image as an attached picture and set the required picture metadata such as title/comment.
- M4A/ALAC: use the MP4-compatible picture mapping required by the FFmpeg muxer; the exact command must be verified with integration tests because MP4 cover-art representation is less uniform than FLAC/MP3 picture streams.
- All formats: copy the audio stream rather than re-encoding it.

Metadata mapping will honor `--no-preserve-metadata`: an explicitly selected sidecar may still be attached, but source tags are not copied. If a target already contains metadata from a prior conversion, the helper must explicitly prevent unintended source metadata from leaking into the result when preservation is disabled.

Command construction should be isolated into a pure helper or otherwise made straightforward to unit-test without launching Docker or FFmpeg.

### 5. Protect hardlinked source files with temporary replacement

Embedding will never pass the final target as FFmpeg's output path. It will:

1. Create a unique temporary output in the target directory.
2. Read from the existing target audio.
3. Write only to the temporary output.
4. Remove/replace the target only after a successful FFmpeg exit.
5. Remove temporary files on failure.

Replacing a hardlinked target with a newly created inode leaves the source untouched. Files without a selected sidecar will continue to use the existing `copyFile()` and hardlink paths.

The resulting behavior is intentional: an audio output modified to contain new embedded artwork must not remain a hardlink to its source. Sidecar image copies may still be hardlinked.

### 6. Treat embedding failures as non-destructive warnings

If no sidecar is found, embedding is a no-op. If FFmpeg cannot attach a selected image, the successfully produced audio remains in place and the failure is reported with the target and image paths. A failed embedding attempt must not leave a partial target or delete a previously valid target.

This follows the existing metadata-merge behavior, which preserves converted audio when metadata preservation fails. Sidecar copying will still follow its existing behavior and may independently return an error when `--copy-images` is enabled.

### 7. Make FFmpeg a feature dependency when embedding is enabled

Local setup will require FFmpeg when `--embed-copied-image` is enabled, even if `--no-preserve-metadata` is set. Docker mode will continue to require only the local Docker executable and will assume the configured image contains FFmpeg, consistent with the existing pipeline.

## Risks / Trade-offs

- **FFmpeg muxer differences** → Verify JPEG/PNG attachment commands independently for FLAC, MP3, and M4A/ALAC with generated real-media fixtures; keep format-specific options isolated.
- **Existing artwork representation differs by container** → Map only the final audio and selected sidecar when a sidecar exists, then assert the resulting stream/metadata with `ffprobe`; add explicit handling if a container stores artwork outside a video stream.
- **Extra remux pass for transcoded files** → Prefer correctness and uniform coverage initially; optimize later by integrating the sidecar into the existing final metadata merge if profiling shows the second pass is significant.
- **Hardlink behavior changes for embedded outputs** → Document the exception in the `prefer-hardlinks` delta spec and assert source inode/content safety.
- **Case-insensitive matching on case-sensitive filesystems** → Enumerate and normalize directory entries rather than assuming filesystem case folding; add precedence tests.
- **Invalid or disappearing sidecar images** → Use a non-destructive failure policy, clean temporary files, and continue processing other audio files with warnings.
- **Target path tracking complexity** → Keep an explicit current-run output inventory rather than scanning the target tree; cover every conversion and fallback branch.
- **Metadata-disabled mode** → Verify explicitly that source tags are not reintroduced while the selected sidecar remains embedded.

## Migration Plan

This is an opt-in feature with no data migration. Existing invocations remain unchanged unless `--embed-copied-image` is supplied.

Implementation and rollout order:

1. Add the configuration/flag and pure image resolver.
2. Add final-output inventory without changing default behavior.
3. Add and test format-specific FFmpeg command construction.
4. Add safe temporary replacement and hardlink protection.
5. Enable the post-processing pass behind the new flag.
6. Update local/Docker dependency checks and documentation.
7. Run deterministic unit tests and optional real-media integration tests.
8. Verify ordinary copies, hardlinked copies, transcodes, and Docker mode manually before release.

Rollback is simply omitting the new flag. No target files are automatically modified unless the feature is enabled. Failed embedding operations preserve the prior audio output and do not mutate the source.

## Open Questions

- Confirm whether the final implementation should keep the image flags independent as designed or require `--copy-images` as a prerequisite. The design recommends independence.
- Confirm whether parent-directory artwork should remain unsupported. The design recommends same-directory-only lookup for predictable behavior.
- Confirm the exact FFmpeg picture mapping for the project's supported FFmpeg and container versions, especially M4A/ALAC. This should be resolved with a small media fixture spike during implementation.
- Confirm the desired warning and exit-code policy for embedding failures. The design recommends preserving processed audio and returning warnings without discarding the run.
