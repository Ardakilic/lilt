## Why

Lilt can copy standalone JPG/PNG artwork and preserve artwork already embedded in audio, but it cannot use a conventional sidecar such as `cover.jpg` as the embedded artwork for a produced FLAC, MP3, or M4A file. Users therefore need to run a separate remux step, and inconsistent artwork can remain when a source already contains an embedded image.

## What Changes

- Add an opt-in `--embed-copied-image` flag to the root command.
- Resolve sidecar artwork beside each source audio file using this priority: `cover.jpg`, `cover.png`, `front.jpg`, `front.png`, `folder.jpg`, `folder.png`.
- Match the supported JPG/PNG image names case-insensitively in the audio file's containing directory without searching parent directories.
- Attach the selected sidecar to every successfully produced audio output, including direct copies, transcoded outputs, and format-enforced outputs.
- When a sidecar is selected, replace any artwork already embedded in the source/output with the selected sidecar rather than retaining both images.
- Keep `--copy-images` independent: it continues to copy supported JPG/PNG sidecars, while `--embed-copied-image` controls embedding.
- Require FFmpeg when embedding is enabled, including when `--no-preserve-metadata` is used; the selected sidecar may still be embedded even when source tags are not preserved.
- Write embedding output to a temporary file and replace the final target only after success, preserving the existing audio output and source file if attachment fails.
- Define the interaction with `--prefer-hardlinks`: files that do not receive embedded artwork retain the existing hardlink behavior, while files that are remuxed with a sidecar no longer share the source inode.

## Capabilities

### New Capabilities

- `embed-copied-image`: Embed conventionally named JPG/PNG sidecar artwork into final audio outputs with deterministic precedence and format-aware media handling.

### Modified Capabilities

- `prefer-hardlinks`: Clarify that embedding a sidecar intentionally breaks a hardlink for the affected audio output because the output is rewritten with new embedded artwork.

## Impact

- **CLI/config:** `Config`, Cobra flag registration, help text, and README documentation.
- **Audio pipeline:** final source-to-target output tracking for default and `--enforce-output-format` processing paths.
- **Media processing:** local and Docker FFmpeg command construction for embedding artwork into FLAC, MP3, and M4A/ALAC outputs.
- **File safety:** temporary-output and replacement logic that prevents FFmpeg from modifying hardlinked source files in place.
- **Dependency checks:** FFmpeg availability when embedding is requested.
- **Testing:** image precedence, output-path coverage, FFmpeg argument construction, format-specific integration, failure cleanup, and hardlink safety.
