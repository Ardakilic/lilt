## ADDED Requirements

### Requirement: CLI exposes a prefer-hardlinks flag

The lilt CLI SHALL expose a boolean flag `--prefer-hardlinks` on the root command. When the flag is omitted, the default value SHALL be `false` and lilt SHALL behave exactly as before.

#### Scenario: Flag defaults to off
- **WHEN** the user runs `lilt <source>` without `--prefer-hardlinks`
- **THEN** `copyFile()` performs byte-by-byte copies and never attempts to create hardlinks

#### Scenario: Flag is explicitly enabled
- **WHEN** the user runs `lilt <source> --prefer-hardlinks`
- **THEN** `copyFile()` attempts to create hardlinks before falling back to copy

### Requirement: Prefer-hardlinks applies to all copy operations

When `--prefer-hardlinks` is enabled, `copyFile()` SHALL attempt a hardlink for every file that would otherwise be copied, including 16-bit FLAC/ALAC files, MP3 files, and image files copied via `--copy-images`, as well as fallback copies after conversion or metadata-read failures.

#### Scenario: 16-bit FLAC file is hardlinked
- **WHEN** a FLAC source file is already 16-bit at a supported sample rate and `--prefer-hardlinks` is enabled
- **THEN** the target file is a hardlink to the source file if the filesystem allows it

#### Scenario: MP3 file is hardlinked
- **WHEN** an MP3 source file is processed and `--prefer-hardlinks` is enabled
- **THEN** the target file is a hardlink to the source file if the filesystem allows it

#### Scenario: Image file is hardlinked
- **WHEN** `--copy-images` and `--prefer-hardlinks` are both enabled
- **THEN** copied JPG/PNG files are hardlinks to the source files if the filesystem allows it

#### Scenario: Conversion failure fallback uses hardlinks
- **WHEN** audio conversion fails and `--prefer-hardlinks` is enabled
- **THEN** the fallback copy operation attempts a hardlink before copying

### Requirement: Hardlink failures fall back to copy with a warning

If a hardlink cannot be created for any reason (cross-device link, unsupported filesystem, existing destination cannot be removed, etc.), `copyFile()` SHALL fall back to the existing byte-by-byte copy and emit a warning message that includes the source path and the error.

#### Scenario: Source and target are on different filesystems
- **WHEN** `--prefer-hardlinks` is enabled and the source and target paths are on different filesystems
- **THEN** `copyFile()` logs a warning and performs a normal byte copy

#### Scenario: Existing destination is replaced by a hardlink
- **WHEN** `--prefer-hardlinks` is enabled and the destination file already exists
- **THEN** the destination is removed first and a hardlink is created, matching existing overwrite semantics

#### Scenario: Hardlink helper fails when destination cannot be removed
- **WHEN** `--prefer-hardlinks` is enabled and the existing destination cannot be removed
- **THEN** `copyFile()` logs a warning and falls back to byte copy

### Requirement: Logging reflects the copy-or-hardlink preference

All log messages emitted immediately before `copyFile()` is invoked for files that do not need transcoding SHALL read `Transcode not needed: Copying or Hardlinking <type>: <path>`. `copyFile()` SHALL additionally emit `Created hardlink: <dst>` on success or a fallback warning on failure.

#### Scenario: Successful hardlink logs the result
- **WHEN** `--prefer-hardlinks` is enabled and a hardlink is created
- **THEN** the output contains `Created hardlink: <target-path>`

#### Scenario: Fallback logs the reason
- **WHEN** `--prefer-hardlinks` is enabled and a hardlink cannot be created
- **THEN** the output contains a warning that includes the source path and the failure reason

#### Scenario: Default mode logs the updated prefix
- **WHEN** a file that does not need transcoding is processed
- **THEN** the output contains `Transcode not needed: Copying or Hardlinking` instead of `Copying`

### Requirement: New and modified functions are documented

Every new exported or unexported function introduced for this feature SHALL have a docblock describing its purpose, parameters, and behavior. The modified `copyFile()` function SHALL also receive a docblock that documents the hardlink preference and fallback behavior.

#### Scenario: Helper function has a docblock
- **WHEN** a new helper such as `createHardlink` is added
- **THEN** it has a docblock explaining that it removes an existing destination before creating the hardlink

#### Scenario: copyFile has an updated docblock
- **WHEN** `copyFile()` is modified to support hardlinks
- **THEN** its docblock states that it prefers hardlinks when the configuration flag is enabled and falls back to copy on failure

#### Scenario: doCopyFile has a docblock
- **WHEN** the existing copy body is extracted into `doCopyFile()`
- **THEN** it has a docblock describing that it performs a byte-by-byte copy while preserving permissions and timestamps
