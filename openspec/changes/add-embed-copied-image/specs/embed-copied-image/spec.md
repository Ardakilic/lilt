## ADDED Requirements

### Requirement: CLI exposes an embed-copied-image flag

The lilt CLI SHALL expose a boolean flag `--embed-copied-image` on the root command. When the flag is omitted, its default value SHALL be `false` and Lilt SHALL behave exactly as before.

#### Scenario: Flag defaults to off
- **WHEN** the user runs `lilt <source>` without `--embed-copied-image`
- **THEN** Lilt performs no sidecar-to-audio embedding operation

#### Scenario: Flag is explicitly enabled
- **WHEN** the user runs `lilt <source> --embed-copied-image`
- **THEN** Lilt searches for an eligible sidecar image and embeds it in produced audio outputs when one is found

### Requirement: Sidecar selection uses deterministic same-directory precedence

When embedding is enabled, Lilt SHALL search only the source audio file's containing directory and SHALL select the first existing regular file in this order: `cover.jpg`, `cover.png`, `front.jpg`, `front.png`, `folder.jpg`, `folder.png`. Name matching SHALL be case-insensitive.

#### Scenario: Cover artwork takes precedence
- **WHEN** a directory contains `cover.jpg`, `front.jpg`, and `folder.jpg` beside an audio file
- **THEN** Lilt selects `cover.jpg`

#### Scenario: PNG is used when no JPG cover exists
- **WHEN** a directory contains `cover.png` and `front.jpg` beside an audio file
- **THEN** Lilt selects `cover.png`

#### Scenario: Front artwork is used before folder artwork
- **WHEN** a directory contains `front.jpg` and `folder.png` beside an audio file
- **THEN** Lilt selects `front.jpg`

#### Scenario: Matching is case-insensitive
- **WHEN** a directory contains `Cover.JPG` beside an audio file
- **THEN** Lilt treats it as the `cover.jpg` candidate

#### Scenario: Parent directories are not searched
- **WHEN** an audio file has no eligible sidecar in its containing directory but its parent directory contains `cover.jpg`
- **THEN** Lilt does not select the parent-directory image

#### Scenario: Non-eligible files are ignored
- **WHEN** a directory contains a non-regular file or an unsupported image extension beside an audio file
- **THEN** Lilt does not select that file

### Requirement: Sidecar copying and embedding are independently controllable

The `--copy-images` flag SHALL continue to copy all supported JPG/PNG files to the target tree. The `--embed-copied-image` flag SHALL control whether a selected sidecar is attached to audio. Neither flag SHALL implicitly enable the other.

#### Scenario: Copy-only behavior is unchanged
- **WHEN** the user enables `--copy-images` without `--embed-copied-image`
- **THEN** Lilt copies supported sidecar images but does not modify audio files to attach them

#### Scenario: Embed-only behavior does not copy sidecars
- **WHEN** the user enables `--embed-copied-image` without `--copy-images`
- **THEN** Lilt may embed a selected sidecar while leaving the target sidecar image absent

#### Scenario: Both flags perform both operations
- **WHEN** the user enables both `--copy-images` and `--embed-copied-image`
- **THEN** Lilt copies the supported sidecar files and embeds the selected precedence image in the audio outputs

### Requirement: Selected sidecar artwork is attached to final audio outputs

When a sidecar is selected, Lilt SHALL attach that image to every successfully produced final audio output for the source audio file, including direct-copy outputs, transcoded outputs, format-enforced outputs, and fallback outputs that are still valid audio files.

#### Scenario: Unchanged MP3 receives selected sidecar
- **WHEN** an MP3 would normally be copied unchanged and its directory contains `cover.jpg`
- **THEN** the produced MP3 contains the selected cover image

#### Scenario: FLAC conversion receives selected sidecar
- **WHEN** a FLAC file is transcoded and its directory contains `front.png`
- **THEN** the produced FLAC contains the selected `front.png` image

#### Scenario: ALAC default conversion receives selected sidecar
- **WHEN** an ALAC source is converted to the default FLAC output and its directory contains `folder.jpg`
- **THEN** the produced FLAC contains the selected `folder.jpg` image

#### Scenario: Format-enforced output receives selected sidecar
- **WHEN** an audio file is processed with `--enforce-output-format=mp3` and a sidecar is selected
- **THEN** the final MP3 output receives the sidecar even when the source extension is FLAC or M4A

#### Scenario: No sidecar preserves current behavior
- **WHEN** no eligible sidecar exists beside a source audio file
- **THEN** Lilt leaves the produced audio output and its existing embedded artwork behavior unchanged

### Requirement: Selected sidecar replaces existing embedded artwork

When a sidecar is selected, Lilt SHALL exclude existing embedded picture streams from the source or final target when constructing the embedding output, and the selected sidecar SHALL be the resulting embedded artwork. Lilt SHALL not intentionally produce both the old artwork and the selected sidecar as separate attached pictures.

#### Scenario: Existing source artwork is overridden
- **WHEN** a source FLAC contains embedded artwork and `cover.jpg` exists beside it
- **THEN** the produced FLAC contains the selected `cover.jpg` instead of the original embedded artwork

#### Scenario: Existing artwork is retained without a sidecar
- **WHEN** a source audio file contains embedded artwork and no eligible sidecar exists
- **THEN** the existing artwork-preservation behavior remains unchanged

#### Scenario: Multiple candidate images produce one selection
- **WHEN** a directory contains both `cover.jpg` and `front.png`
- **THEN** only the selected `cover.jpg` is intentionally attached to the audio output

### Requirement: Embedding honors metadata preservation settings

When metadata preservation is enabled, the embedding pass SHALL preserve the existing final output metadata behavior while replacing artwork with the selected sidecar. When metadata preservation is disabled, Lilt SHALL still attach an explicitly selected sidecar when embedding is enabled, but SHALL not copy source tags into the resulting output.

#### Scenario: Metadata-preserving embedding
- **WHEN** metadata preservation is enabled and a sidecar is selected
- **THEN** source metadata is retained according to the existing pipeline and the selected sidecar replaces existing artwork

#### Scenario: Embedding without metadata preservation
- **WHEN** `--no-preserve-metadata` and `--embed-copied-image` are enabled and a sidecar is selected
- **THEN** the selected sidecar is embedded while source tags are not copied

#### Scenario: FFmpeg is required for embedding
- **WHEN** `--embed-copied-image` is enabled in local mode
- **THEN** Lilt requires an available FFmpeg executable even if metadata preservation is disabled

### Requirement: Embedding writes safely and protects hardlinked sources

Lilt SHALL write embedding output to a temporary file and replace the final audio target only after a successful embedding operation. Lilt SHALL NOT write FFmpeg output directly to a target that may be a hardlink to its source. A successfully embedded audio output SHALL be independent from the source file's inode.

#### Scenario: Temporary output is used
- **WHEN** a selected sidecar is embedded
- **THEN** FFmpeg writes to a temporary output before the final target is replaced

#### Scenario: Hardlinked source is protected
- **WHEN** `--prefer-hardlinks` created a hardlink for an audio file and that file receives an embedded sidecar
- **THEN** the source file remains unchanged and the final embedded target no longer shares the source inode

#### Scenario: No-sidecar hardlink behavior is preserved
- **WHEN** `--prefer-hardlinks` is enabled and no sidecar is selected for an unchanged audio file
- **THEN** the existing hardlink behavior remains unchanged

### Requirement: Embedding failures preserve the audio output

If a selected sidecar cannot be embedded, Lilt SHALL preserve the previously produced audio output, remove temporary artifacts, emit a warning identifying the target and sidecar, and continue processing other files. A partial or corrupt audio target SHALL NOT replace a valid prior target.

#### Scenario: Missing or invalid sidecar
- **WHEN** the selected sidecar is unreadable or cannot be processed by FFmpeg
- **THEN** Lilt keeps the valid audio output, cleans temporary files, and reports a warning

#### Scenario: Embedding failure does not stop the batch
- **WHEN** embedding fails for one audio file and other audio files remain
- **THEN** Lilt continues processing the remaining files and reports the failure

#### Scenario: No sidecar is a no-op
- **WHEN** no eligible sidecar is found
- **THEN** Lilt does not invoke an embedding operation or report an embedding failure
