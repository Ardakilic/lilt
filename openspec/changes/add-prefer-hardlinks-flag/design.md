## Context

Lilt is a single-file Go CLI (`main.go`) that walks a source music directory, determines whether each audio file needs bit-depth/sample-rate conversion, and writes the result into a target directory. Files that already meet the target format (e.g., 16-bit FLAC) or are not lossless audio (MP3) or are companion images (JPG/PNG) are duplicated via `copyFile()`. This duplication is wasteful for users who keep source and target libraries on the same filesystem and do not need independent copies.

All copy paths converge on the unexported `copyFile(src, dst string)` helper in `main.go`. That makes it the ideal single integration point for a hardlink optimization.

## Goals / Non-Goals

**Goals:**

- Add an opt-in `--prefer-hardlinks` CLI flag.
- When enabled, `copyFile()` attempts to create a filesystem hardlink before falling back to a byte-by-byte copy.
- Preserve existing overwrite semantics (a pre-existing destination file is replaced).
- Log hardlink successes and fallback warnings so the user understands what happened.
- Update caller log messages to read `Transcode not needed: Copying or Hardlinking <type>: <path>`.
- Cover the new behavior with unit tests, including the fallback path.
- Document all modified and new functions with docblocks.

**Non-Goals:**

- Changing the default behavior (flag is opt-in; existing users are unaffected).
- Using symbolic links instead of hardlinks (symlinks are fragile and break when source/target paths move independently).
- Performing hardlinks across filesystems (this is impossible; we fall back to copy).
- Refactoring the rest of the conversion pipeline.
- Adding hardlinks inside Docker conversions themselves; `copyFile()` runs on the host, so host FS rules apply.

## Decisions

### 1. Centralize the hardlink attempt inside `copyFile()`

**Decision:** Modify `copyFile()` to check `config.PreferHardlinks` and call a new helper. Do not change the 13 existing call sites.

**Rationale:** Keeps the change minimal and guarantees that every future copy path automatically supports the flag. The alternative—wrapping each call site—would be more intrusive and error-prone.

### 2. Remove the destination before calling `os.Link`

**Decision:** If `dst` already exists, remove it, then attempt `os.Link(src, dst)`. If removal fails, propagate the error (the hardlink attempt fails).

**Rationale:** Matches current overwrite semantics. `os.Create(dst)` used by the existing copy path truncates/replaces an existing file. Without removal, `os.Link` would fail with `EEXIST`/`ERROR_ALREADY_EXISTS` on an existing destination.

### 3. Log from `copyFile()` rather than changing all callers

**Decision:** Keep caller messages generic (`Transcode not needed: Copying or Hardlinking <type>: <path>`) and emit `Created hardlink: <dst>` or a fallback warning from inside `copyFile()`.

**Rationale:** Callers do not know whether a hardlink or copy will succeed. Centralizing the success/failure log avoids duplicated decision logic across 13 call sites.

### 4. Make `os.Link` swappable for tests

**Decision:** Introduce a small package-level variable (e.g., `osLink = os.Link`) used only by the hardlink helper. Tests can temporarily replace it to simulate hardlink failures and assert the copy fallback.

**Rationale:** Cross-device links and unsupported filesystems are hard to reproduce deterministically in unit tests. A swappable link function allows reliable coverage of the fallback path without changing public APIs.

### 5. Do not special-case Docker mode

**Decision:** Let `copyFile()` run on the host. If `sourceDir` and `targetDir` are on the same host filesystem, hardlinks work even when `--use-docker` is enabled; otherwise they fall back to copy.

**Rationale:** Docker mounts `/source` and `/target` separately inside the container, but the Go process executes `copyFile()` on the host after the containerized conversion finishes. The host filesystem layout determines hardlink viability.

## Implementation Sketch

### Config change

Add `PreferHardlinks bool` to the `Config` struct in `main.go`:

```go
type Config struct {
    SourceDir           string
    TargetDir           string
    CopyImages          bool
    UseDocker           bool
    DockerImage         string
    SoxCommand          string
    NoPreserveMetadata  bool
    EnforceOutputFormat string
    PreferHardlinks     bool // NEW
}
```

### Flag registration

Register the flag in `init()`:

```go
rootCmd.Flags().BoolVar(
    &config.PreferHardlinks,
    "prefer-hardlinks",
    false,
    "Prefer filesystem hardlinks over copying for files that do not need transcoding",
)
```

### Package-level swappable link function

```go
// osLink is a package-level alias for os.Link to allow tests to inject failures.
var osLink = os.Link
```

### New helper: createHardlink

```go
// createHardlink attempts to create a hardlink from src to dst.
// If dst already exists, it is removed first so that the operation
// matches the overwrite semantics of the existing copyFile behavior.
func createHardlink(src, dst string) error {
    if _, err := os.Stat(dst); err == nil {
        if err := os.Remove(dst); err != nil {
            return fmt.Errorf("failed to remove existing destination: %w", err)
        }
    }
    return osLink(src, dst)
}
```

### Refactored copyFile

```go
// copyFile copies the source file to the destination path.
// If config.PreferHardlinks is true, it first attempts to create a
// filesystem hardlink. If the hardlink cannot be created, it logs a
// warning and falls back to a byte-by-byte copy that preserves
// permissions and modification time.
func copyFile(src, dst string) error {
    if config.PreferHardlinks {
        if err := createHardlink(src, dst); err == nil {
            fmt.Printf("Created hardlink: %s\n", dst)
            return nil
        }
        fmt.Printf(
            "Warning: Could not create hardlink for %s, falling back to copy: %v\n",
            src, err,
        )
    }
    return doCopyFile(src, dst)
}
```

### Existing copy body becomes doCopyFile

Move the current implementation of `copyFile` (open, copy bytes, sync, chmod, chtimes) into a new helper:

```go
// doCopyFile performs a byte-by-byte copy of src to dst,
// preserving permissions and modification time.
func doCopyFile(src, dst string) error {
    // existing copyFile body
}
```

### Caller log message updates

All messages that precede `copyFile()` for files that do not require transcoding should be updated. Examples:

- `Copying MP3 file: %s` → `Transcode not needed: Copying or Hardlinking MP3 file: %s`
- `Copying FLAC: %s` → `Transcode not needed: Copying or Hardlinking FLAC: %s`
- `Copying FLAC: %s (already 16-bit)` → `Transcode not needed: Copying or Hardlinking FLAC: %s (already 16-bit)`
- `Copying MP3: %s (already in target format)` → `Transcode not needed: Copying or Hardlinking MP3: %s (already in target format)`
- `Copying ALAC: %s (already 16-bit)` → `Transcode not needed: Copying or Hardlinking ALAC: %s (already 16-bit)`

Messages for conversion failures that fall back to copy (e.g., `Error: Audio conversion failed. Copying original file instead.`) should keep their existing warning text because they describe an error path, but they still invoke `copyFile()` and therefore still benefit from hardlinks when the flag is enabled.

## Test Strategy

Use `os.SameFile` for cross-platform hardlink assertions. Inject hardlink failures via the `osLink` variable.

Example success test:

```go
func TestCopyFileHardlinkSuccess(t *testing.T) {
    tmpDir := t.TempDir()
    src := filepath.Join(tmpDir, "src.txt")
    dst := filepath.Join(tmpDir, "dst.txt")
    os.WriteFile(src, []byte("hello"), 0644)

    originalConfig := config
    config.PreferHardlinks = true
    defer func() { config = originalConfig }()

    if err := copyFile(src, dst); err != nil {
        t.Fatalf("copyFile failed: %v", err)
    }

    srcInfo, _ := os.Stat(src)
    dstInfo, _ := os.Stat(dst)
    if !os.SameFile(srcInfo, dstInfo) {
        t.Errorf("expected dst to be a hardlink to src")
    }
}
```

Example fallback test:

```go
func TestCopyFileHardlinkFallback(t *testing.T) {
    originalLink := osLink
    osLink = func(_, _ string) error { return errors.New("mock hardlink failure") }
    defer func() { osLink = originalLink }()

    tmpDir := t.TempDir()
    src := filepath.Join(tmpDir, "src.txt")
    dst := filepath.Join(tmpDir, "dst.txt")
    os.WriteFile(src, []byte("hello"), 0644)

    originalConfig := config
    config.PreferHardlinks = true
    defer func() { config = originalConfig }()

    if err := copyFile(src, dst); err != nil {
        t.Fatalf("copyFile failed: %v", err)
    }

    content, _ := os.ReadFile(dst)
    if string(content) != "hello" {
        t.Errorf("fallback copy did not copy content")
    }
}
```

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| **Hardlinks share inode data.** Modifying the source file after conversion also modifies the target, which may surprise users who expected an independent copy. | Document the behavior in the flag help text and proposal; this is inherent to hardlinks and is the intended trade-off for saving space. |
| **Cross-device or unsupported filesystems cause hardlinks to fail.** | Catch the `os.Link` error and fall back to byte copy with a warning. |
| **Existing destination removal before hardlink is not atomic.** If removal succeeds but `os.Link` then fails and copy also fails, the destination is gone. | This matches existing copy semantics where `os.Create` truncates the destination early. The window is small and acceptable for a CLI conversion tool. |
| **Log noise on systems where hardlinks always fail** (e.g., FAT32 USB drive used as target). | Emit fallback as a warning; users who see repeated warnings can disable the flag or move the target to a compatible filesystem. |
| **Test portability.** Hardlink inode equality is easy to assert on Unix but requires `os.SameFile` on Windows. | Use `os.SameFile(srcInfo, dstInfo)` in tests, which works cross-platform. |

## Migration Plan

No migration is required. The change is purely additive and opt-in.

1. Merge the implementation.
2. Update release notes to mention `--prefer-hardlinks`.
3. Users who want the optimization can start using the flag immediately; existing commands continue to work unchanged.

## Open Questions

- Should the fallback warning be rate-limited or summarized at the end of the run? (Current decision: per-file warning for transparency.)
