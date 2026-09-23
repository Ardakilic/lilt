## MODIFIED Requirements

### Requirement: Prefer-hardlinks applies to all copy operations except outputs modified by sidecar embedding

When `--prefer-hardlinks` is enabled, `copyFile()` SHALL attempt a hardlink for every file that would otherwise be copied, including 16-bit FLAC/ALAC files, MP3 files, and image files copied via `--copy-images`, as well as fallback copies after conversion or metadata-read failures. If `--embed-copied-image` selects a sidecar for an audio output, Lilt SHALL preserve the hardlink attempt for the initial copy but SHALL replace the affected audio target with an independent file when the sidecar is successfully embedded.

#### Scenario: 16-bit FLAC file is hardlinked
- **WHEN** a FLAC source file is already 16-bit at a supported sample rate and `--prefer-hardlinks` is enabled
- **THEN** the target file is a hardlink to the source file if the filesystem allows it and no sidecar is selected for embedding

#### Scenario: MP3 file is hardlinked
- **WHEN** an MP3 source file is processed and `--prefer-hardlinks` is enabled and no sidecar is selected for embedding
- **THEN** the target file is a hardlink to the source file if the filesystem allows it

#### Scenario: Image file is hardlinked
- **WHEN** `--copy-images` and `--prefer-hardlinks` are both enabled
- **THEN** copied JPG/PNG files are hardlinks to the source files if the filesystem allows it

#### Scenario: Conversion failure fallback uses hardlinks
- **WHEN** audio conversion fails and `--prefer-hardlinks` is enabled and no sidecar is selected for embedding
- **THEN** the fallback copy operation attempts a hardlink before copying

#### Scenario: Sidecar embedding breaks the affected audio hardlink
- **WHEN** `--prefer-hardlinks` is enabled, a sidecar is selected for an audio output, and embedding succeeds
- **THEN** the source file remains unchanged, the final audio target contains the embedded sidecar, and the final target no longer shares the source inode

#### Scenario: No-sidecar audio retains hardlink behavior
- **WHEN** `--prefer-hardlinks` is enabled and no eligible sidecar is selected for an unchanged audio file
- **THEN** the existing hardlink behavior remains unchanged

#### Scenario: Sidecar image copies remain eligible for hardlinks
- **WHEN** both `--copy-images` and `--embed-copied-image` are enabled and the selected image is copied as a standalone file
- **THEN** the standalone image copy may still be a hardlink to the source image while the audio target is independently remuxed
