# Add `--prefer-hardlinks` flag to avoid duplicate storage for unchanged files

## Summary

This PR introduces an opt-in `--prefer-hardlinks` CLI flag. When enabled, `lilt` attempts to create filesystem hardlinks instead of performing byte-by-byte copies for any file that does not require audio transcoding (16-bit FLAC/ALAC, MP3, and image files copied via `--copy-images`). If a hardlink cannot be created, the tool falls back to the existing copy behavior with a warning.

## Motivation

For users who keep source and target music libraries on the same filesystem, copying identical files wastes significant disk space. Hardlinks allow the source and target to share the same inode data, eliminating that duplication while preserving the existing directory structure.

## Changes

### User-facing

- **New flag:** `--prefer-hardlinks` (boolean, default `false`)
- **Updated log lines:** Messages that previously read `Copying <type>: <path>` now read `Transcode not needed: Copying or Hardlinking <type>: <path>`
- **New log lines:**
  - `Created hardlink: <dst>` on successful hardlink creation
  - `Warning: Could not create hardlink for <src>, falling back to copy: <error>` when hardlink creation fails
- **Updated `--help` output:** `rootCmd.Long` now documents the new flag

### Code changes (`main.go`)

1. Added `PreferHardlinks bool` to the `Config` struct.
2. Registered the `--prefer-hardlinks` flag in `init()` using `rootCmd.Flags().BoolVar`.
3. Added a swappable package-level variable `osLink = os.Link` to enable deterministic fallback testing.
4. Extracted the existing byte-by-byte copy logic from `copyFile()` into a new unexported helper `doCopyFile(src, dst string) error` with a docblock.
5. Added a new unexported helper `createHardlink(src, dst string) error` that removes an existing destination before calling `os.Link`, matching existing overwrite semantics.
6. Updated `copyFile()` with a docblock and hardlink preference logic:
   - When `config.PreferHardlinks` is true, attempt `createHardlink()` first.
   - On success, log and return.
   - On failure, log a warning and fall back to `doCopyFile()`.
   - When the flag is false, call `doCopyFile()` directly.
7. Updated all non-error "Copying …" log messages that precede `copyFile()` calls to use the new `Transcode not needed: Copying or Hardlinking …` prefix.
8. Updated `rootCmd.Long` help text to describe `--prefer-hardlinks`.

### Test changes (`main_test.go`)

Added comprehensive unit tests:

- `TestCopyFileHardlinkSuccess` — verifies a hardlink is created when the flag is enabled and the filesystem supports it (uses `os.SameFile`).
- `TestCopyFileHardlinkFallback` — injects a mock `osLink` failure and verifies the fallback copy succeeds and emits a warning.
- `TestCopyFileHardlinkOverwrite` — verifies an existing destination is replaced by a hardlink.
- `TestCreateHardlink` — directly tests the helper with success, overwrite, and failure sub-cases.
- `TestCopyFileWithoutHardlinks` — verifies normal independent copy behavior when the flag is disabled.

Also verified that existing copy tests continue to pass with the refactored `doCopyFile()`:

- `TestCopyFile`
- `TestCopyFileErrors`
- `TestCopyFilePermissions`
- `TestCopyFileDestinationExists`
- `TestCopyFileSyncError`
- `TestCopyFileLargeFile`
- `TestCopyFileReadOnlySource`
- `TestFileOperationEdgeCases`

## Behavior

### Default behavior (flag omitted)

No changes. `lilt` continues to perform byte-by-byte copies exactly as before.

### With `--prefer-hardlinks`

For every file that would otherwise be copied via `copyFile()`:

1. If the destination already exists, it is removed first.
2. `lilt` attempts `os.Link(src, dst)`.
3. If the hardlink succeeds, `lilt` prints `Created hardlink: <dst>`.
4. If the hardlink fails (cross-device link, unsupported filesystem such as FAT32/exFAT, permission issues, etc.), `lilt` prints a warning and falls back to `doCopyFile()`.

### Important caveats

- **Shared inode:** Because hardlinks share inode data, any in-place modification to either the source or target path will affect the other. This is the intended trade-off for saving space.
- **Cross-device / unsupported filesystems:** Automatically fall back to copy with a warning.
- **Overwrite semantics:** Pre-existing destinations are removed before the hardlink attempt, matching the existing copy behavior.
- **Docker mode:** `copyFile()` runs on the host, so host filesystem rules apply. Hardlinks work when source and target directories are on the same host filesystem.

## Verification

- `go vet ./...` — passed
- `gofmt -w main.go main_test.go` — applied
- `go test ./...` — passed

## Checklist

- [x] New feature is opt-in and does not change default behavior
- [x] All new and modified functions have docblocks
- [x] New behavior is covered by unit tests
- [x] Existing copy tests continue to pass
- [x] `go vet` passes
- [x] Code is formatted with `gofmt`
- [x] Help text updated
- [x] OpenSpec tasks marked complete

## Related

OpenSpec change: `add-prefer-hardlinks-flag`
