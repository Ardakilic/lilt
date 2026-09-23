## 1. CLI configuration and dependency setup

- [x] 1.1 Add `EmbedCopiedImage bool` to `Config` with a comment describing the opt-in sidecar embedding behavior.
- [x] 1.2 Register `--embed-copied-image` with Cobra using `BoolVar`, defaulting to `false`, and add clear root help text.
- [x] 1.3 Update local `setupSoxCommand()` so FFmpeg is required when `--embed-copied-image` is enabled, including with `--no-preserve-metadata`.
- [x] 1.4 Keep Docker setup behavior consistent by relying on the configured Docker image's FFmpeg binary when embedding is enabled.
- [x] 1.5 Add flag/default/help coverage and ensure existing configuration tests restore global `config` state safely.

## 2. Sidecar resolution

- [x] 2.1 Add a documented resolver that searches only the source audio file's containing directory for regular JPG/PNG files.
- [x] 2.2 Implement the exact precedence order `cover.jpg`, `cover.png`, `front.jpg`, `front.png`, `folder.jpg`, `folder.png`.
- [x] 2.3 Make candidate matching case-insensitive without adding unsupported image extensions or parent-directory inheritance.
- [x] 2.4 Ensure the resolver returns no match for missing, unsupported, non-regular, and parent-directory-only candidates.
- [x] 2.5 Add table-driven unit tests for precedence, case variants, same-directory reuse, nested directories, unsupported files, and no-match behavior.

## 3. Final audio output tracking

- [x] 3.1 Define a documented output record containing the source audio path and actual final target path.
- [x] 3.2 Refactor the audio walk to record outputs from every successful branch: direct copies, transcode paths, format-enforced paths, and valid fallback copies.
- [x] 3.3 Preserve the existing `processAudioFiles()` behavior for callers that do not need output records, or update all callers consistently without scanning unrelated target files.
- [x] 3.4 Add tests for default FLAC, default ALAC-to-FLAC, MP3, enforced FLAC/MP3/ALAC, and extension-preserving MP3 exceptions.
- [x] 3.5 Ensure the inventory contains only files produced by the current invocation and preserves the source-relative directory structure.

## 4. FFmpeg image attachment

- [x] 4.1 Add documented, format-aware FFmpeg argument construction for attaching a JPEG or PNG to FLAC, MP3, and M4A/ALAC outputs.
- [x] 4.2 Map the final target audio stream and selected image stream without intentionally mapping the target's existing picture stream when a sidecar replaces existing artwork.
- [x] 4.3 Preserve the audio stream with stream copy and apply the required `attached_pic` disposition, picture metadata, and image codec/muxer options for each supported output format.
- [x] 4.4 Make metadata mapping explicitly follow `--no-preserve-metadata` while still attaching the selected sidecar when that flag is enabled.
- [x] 4.5 Implement local execution through the configured FFmpeg binary and Docker execution through `docker run --entrypoint ffmpeg` with `/source` and `/target` path conversion.
- [x] 4.6 Add command-construction tests for local and Docker paths, FLAC/MP3/M4A-specific options, source-art replacement, audio stream copy, and metadata-disabled behavior.

## 5. Safe output replacement and pipeline integration

- [x] 5.1 Add a helper that creates a temporary output in the target directory, runs the embedding command, and replaces the final target only after successful completion.
- [x] 5.2 Add cleanup and warning handling for missing targets, invalid images, FFmpeg failures, and temporary-file failures without deleting a valid prior audio output.
- [x] 5.3 Add the post-processing embedding pass after audio processing and before or independently of `copyImageFiles()`, using the final target paths from the output inventory.
- [x] 5.4 Make a missing sidecar a no-op so the existing conversion, copy, metadata, and image-copy behavior is unchanged.
- [x] 5.5 Keep `--copy-images` and `--embed-copied-image` independent; when both are enabled, copy all supported sidecars but embed only the highest-priority candidate.
- [x] 5.6 Ensure embedding never writes FFmpeg output directly to a hardlinked target and replaces the affected target with an independent inode only after success.
- [x] 5.7 Ensure no-sidecar outputs retain the existing `copyFile()` hardlink behavior, while standalone copied sidecars may still be hardlinked.

## 6. Test coverage and validation

- [x] 6.1 Add unit tests for flag parsing, default behavior, resolver precedence, and same-directory-only lookup.
- [x] 6.2 Add tests for all direct-copy and format-enforced output paths, verifying that the selected sidecar is applied to the actual final extension.
- [x] 6.3 Add tests proving that an existing embedded image is replaced by the selected sidecar and that no duplicate picture stream is intentionally emitted.
- [x] 6.4 Add tests proving that `--no-preserve-metadata` still embeds a selected sidecar without preserving source tags and that FFmpeg is required in local mode.
- [x] 6.5 Add tests for failed embedding, warning output, continuation of the batch, and temporary-file cleanup.
- [x] 6.6 Add hardlink safety tests asserting that the source content and inode relationship remain unchanged during embedding and that the final target is independent.
- [x] 6.7 Add optional real-media integration tests using generated temporary FLAC, MP3, M4A/ALAC, JPEG, and PNG fixtures; skip only when required media tools are unavailable.
- [x] 6.8 Add Docker command/path tests without requiring Docker to run.
- [x] 6.9 Run `go test ./...`, `go vet ./...`, formatting checks, and the CI build/help/version verification commands; fix regressions before marking tasks complete.

## 7. Documentation and release readiness

- [x] 7.1 Update `README.md` and CLI help with image precedence, same-directory lookup, existing-art replacement, flag independence, and hardlink interaction.
- [x] 7.2 Document the FFmpeg dependency for local embedding and the expected Docker image dependency.
- [x] 7.3 Confirm the new OpenSpec capability and the `prefer-hardlinks` delta are complete and consistent with the implemented behavior.
- [x] 7.4 Update this task list to mark only implementation and verification work complete after the full test suite passes.
