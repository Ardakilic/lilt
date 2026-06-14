## 1. Configuration and CLI flag

- [x] 1.1 Add `PreferHardlinks bool` field to the `Config` struct in `main.go` (after `EnforceOutputFormat`).
- [x] 1.2 Register `--prefer-hardlinks` boolean flag in `init()` using `rootCmd.Flags().BoolVar`, defaulting to `false`, with help text such as:
  `"Prefer filesystem hardlinks over copying for files that do not need transcoding"`.

## 2. Core hardlink implementation

- [x] 2.1 Add a package-level variable near the top of `main.go`:
  ```go
  // osLink is a package-level alias for os.Link to allow tests to inject failures.
  var osLink = os.Link
  ```
- [x] 2.2 Extract the existing byte-by-byte copy body of `copyFile()` into a new unexported helper `doCopyFile(src, dst string) error` and add a docblock describing that it copies content while preserving permissions and timestamps.
- [x] 2.3 Add a new unexported helper `createHardlink(src, dst string) error` that:
  - Checks if `dst` exists with `os.Stat`.
  - Removes `dst` with `os.Remove` if it exists.
  - Calls `osLink(src, dst)`.
  - Returns any error encountered.
  Add a docblock describing the overwrite behavior.
- [x] 2.4 Update `copyFile()` with a docblock and implement the hardlink preference:
  - When `config.PreferHardlinks` is true, call `createHardlink(src, dst)`.
  - On success, print `Created hardlink: <dst>` and return nil.
  - On failure, print a warning including the source path and error, then fall back to `doCopyFile(src, dst)`.
  - When the flag is false, call `doCopyFile(src, dst)` directly.

## 3. Logging updates

- [x] 3.1 Update the caller log message in `processAudioFiles()` for MP3 files from:
  ```go
  fmt.Printf("Copying MP3 file: %s\n", path)
  ```
  to:
  ```go
  fmt.Printf("Transcode not needed: Copying or Hardlinking MP3 file: %s\n", path)
  ```
- [x] 3.2 Update the caller log message in `processAudioFiles()` for 16-bit FLAC files from:
  ```go
  fmt.Printf("Copying FLAC: %s\n", path)
  ```
  to:
  ```go
  fmt.Printf("Transcode not needed: Copying or Hardlinking FLAC: %s\n", path)
  ```
- [x] 3.3 Update all other `Copying …` log messages that precede `copyFile()` calls in `processAudioFileWithEnforcedFormat()`, `processToFLAC()`, `processToMP3()`, and `processToALAC()` to use the `Transcode not needed: Copying or Hardlinking …` prefix where the file is not being transcoded. Keep error-path fallback messages unchanged.

## 4. Tests

- [x] 4.1 Add `TestCopyFileHardlinkSuccess` in `main_test.go`:
  - Create a temp directory.
  - Write a source file and set `config.PreferHardlinks = true` (restore config afterward).
  - Call `copyFile(src, dst)`.
  - Assert no error.
  - Assert `os.SameFile(srcInfo, dstInfo)` is true.
- [x] 4.2 Add `TestCopyFileHardlinkFallback` in `main_test.go`:
  - Temporarily replace `osLink` with a function that returns `errors.New("mock hardlink failure")` and restore it afterward.
  - Enable `config.PreferHardlinks` and restore afterward.
  - Create source and destination temp files in the same directory.
  - Call `copyFile(src, dst)`.
  - Assert no error.
  - Assert destination content equals source content (copy fallback worked).
  - Capture stdout and assert the warning contains "falling back to copy".
- [x] 4.3 Add `TestCopyFileHardlinkOverwrite` in `main_test.go`:
  - Enable `config.PreferHardlinks` and restore afterward.
  - Create source and pre-existing destination files with different content.
  - Call `copyFile(src, dst)`.
  - Assert `os.SameFile(srcInfo, dstInfo)` is true and destination content matches source content.
- [x] 4.4 Add `TestCreateHardlink` in `main_test.go` covering the helper directly:
  - Success case: source exists, destination does not, hardlink is created.
  - Overwrite case: source exists, destination exists, destination is replaced by a hardlink.
  - Failure case: source does not exist, returns an error.
- [x] 4.5 Add `TestCopyFileWithoutHardlinks` in `main_test.go`:
  - Ensure `config.PreferHardlinks = false` (default).
  - Call `copyFile(src, dst)`.
  - Assert `os.SameFile(srcInfo, dstInfo)` is false (normal copy, independent file).
- [x] 4.6 Ensure existing copy tests (`TestCopyFile`, `TestCopyFileErrors`, `TestCopyFilePermissions`, `TestCopyFileDestinationExists`, `TestCopyFileSyncError`, `TestCopyFileLargeFile`, `TestCopyFileReadOnlySource`, `TestFileOperationEdgeCases`) continue to pass with the refactored `doCopyFile()`.
- [x] 4.7 Run the full test suite (`go test ./...`) and fix any regressions.

## 5. Verification and documentation

- [x] 5.1 Run `go vet ./...` and resolve any issues.
- [x] 5.2 Run `gofmt -w main.go main_test.go`.
- [x] 5.3 Update the long help text in `rootCmd.Long` or the README if necessary to mention `--prefer-hardlinks`.
- [x] 5.4 Mark all tasks complete and run `openspec status --change "add-prefer-hardlinks-flag"` to confirm the change is ready for archive.
