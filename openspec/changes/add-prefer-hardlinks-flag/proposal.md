## Why

Lilt currently copies source files byte-for-byte whenever no audio conversion is required: 16-bit FLAC/ALAC files, MP3 files, and image files copied via `--copy-images`. On large music libraries this duplicates significant storage even though the source and target content are identical. A `--prefer-hardlinks` flag lets users avoid this duplication by creating filesystem hardlinks instead of full copies whenever it is safe to do so, falling back to copy when hardlinks cannot be created.

## What Changes

- Add a new CLI flag `--prefer-hardlinks` to the root `lilt` command.
- When the flag is enabled, replace every file-copy operation performed by `copyFile()` with a hardlink attempt first.
- If the hardlink cannot be created (cross-device link, unsupported filesystem, existing destination cannot be replaced, etc.), log a warning and fall back to the existing byte-by-byte copy behavior.
- Update caller log messages from `Copying <type>: <path>` to `Transcode not needed: Copying or Hardlinking <type>: <path>` so the output reflects the new preference.
- Preserve existing overwrite semantics: if the destination file already exists, remove it before attempting the hardlink.
- Add comprehensive unit tests covering hardlink success, hardlink fallback, normal copy when the flag is off, destination overwrite, and cross-platform edge cases.
- Add docblocks to the modified `copyFile()` function and any new helper functions introduced.

## Capabilities

### New Capabilities

- `prefer-hardlinks`: Optional hardlink-based copy optimization for all files that would otherwise be duplicated by `copyFile()`, including 16-bit FLAC/ALAC files, MP3 files, and image files.

### Modified Capabilities

- None. This change does not alter existing spec-level requirements; it adds a new optional optimization path that falls back to the existing copy behavior.

## Impact

- **User-facing**: New `--prefer-hardlinks` flag available on the `lilt` command.
- **Behavioral**: When enabled and the source/target reside on the same filesystem, no duplicate inode data is written for files that do not require transcoding. Source and target share the same inode, so subsequent in-place modifications to either path affect both.
- **Logging**: Existing `Copying …` log lines are reworded to `Transcode not needed: Copying or Hardlinking …`. Additional `Created hardlink: …` or fallback warning lines may be emitted from `copyFile()`.
- **Code**: `main.go` (`Config`, `init()`, `copyFile()`, and callers) and `main_test.go`.
- **Compatibility**: No breaking changes. Default behavior remains unchanged (flag is opt-in). The feature works whenever `os.Link` succeeds on the current filesystem and automatically falls back to a copy on any failure (filesystem type, cross-device paths, permission issues, etc.).
