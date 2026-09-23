package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbedCopiedImageFlag verifies the new Cobra flag is registered and
// defaults to the opt-in false value.
func TestEmbedCopiedImageFlag(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })

	flag := rootCmd.Flags().Lookup("embed-copied-image")
	if flag == nil {
		t.Fatal("expected --embed-copied-image flag")
	}
	if flag.DefValue != "false" {
		t.Errorf("flag default = %q, want false", flag.DefValue)
	}
	if !strings.Contains(flag.Usage, "sidecar") {
		t.Errorf("flag usage %q does not describe sidecar artwork", flag.Usage)
	}
	if config.EmbedCopiedImage {
		t.Error("EmbedCopiedImage should default to false")
	}
}

// TestSetupSoxCommandWithLookPathEmbeddingRequiresFFmpeg verifies that
// embedding requires FFmpeg even when metadata preservation is disabled.
func TestSetupSoxCommandWithLookPathEmbeddingRequiresFFmpeg(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })

	sourceDir := t.TempDir()
	config.SourceDir = sourceDir
	config.TargetDir = t.TempDir()
	config.UseDocker = false
	config.SoxCommand = "sox"
	config.NoPreserveMetadata = true
	config.EmbedCopiedImage = true

	calls := []string{}
	lookPath := func(name string) (string, error) {
		calls = append(calls, name)
		if name == "ffmpeg" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}

	if err := setupSoxCommandWithLookPath(lookPath); err == nil || !strings.Contains(err.Error(), "ffmpeg is not installed") {
		t.Fatalf("setup error = %v, want missing FFmpeg error", err)
	}
	if len(calls) != 2 || calls[0] != "sox" || calls[1] != "ffmpeg" {
		t.Errorf("lookPath calls = %v, want [sox ffmpeg]", calls)
	}
}

// TestSetupSoxCommandWithLookPathNoEmbeddingSkipsFFmpeg verifies that a
// FLAC-only no-metadata run without embedding checks only SoX.
func TestSetupSoxCommandWithLookPathNoEmbeddingSkipsFFmpeg(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })

	config.SourceDir = t.TempDir()
	config.TargetDir = t.TempDir()
	config.UseDocker = false
	config.SoxCommand = "sox"
	config.NoPreserveMetadata = true
	config.EmbedCopiedImage = false

	calls := []string{}
	lookPath := func(name string) (string, error) {
		calls = append(calls, name)
		return "/usr/bin/" + name, nil
	}
	if err := setupSoxCommandWithLookPath(lookPath); err != nil {
		t.Fatalf("setupSoxCommandWithLookPath returned error: %v", err)
	}
	if len(calls) != 1 || calls[0] != "sox" {
		t.Errorf("lookPath calls = %v, want [sox]", calls)
	}
}

// TestBuildEmbedImageCommand verifies local and Docker command construction,
// stream replacement, metadata policy, and format-specific options.
func TestBuildEmbedImageCommand(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })

	contains := func(args []string, want string) bool {
		for _, arg := range args {
			if arg == want {
				return true
			}
		}
		return false
	}

	for _, format := range []string{".flac", ".mp3", ".m4a"} {
		t.Run("local"+format, func(t *testing.T) {
			config.UseDocker = false
			config.NoPreserveMetadata = false
			audioPath := "/work/track" + format
			imagePath := "/work/Cover.JPG"
			outputPath := "/work/.track" + format + ".embed-123" + format

			commandName, args, err := buildEmbedImageCommand(audioPath, imagePath, outputPath)
			if err != nil {
				t.Fatalf("buildEmbedImageCommand returned error: %v", err)
			}
			if commandName != "ffmpeg" {
				t.Errorf("command = %q, want ffmpeg", commandName)
			}
			for _, want := range []string{
				"-map", "0:a:0", "-map", "1:v:0", "-map_metadata", "0",
				"-c:a", "copy", "-disposition:v:0", "attached_pic", outputPath,
			} {
				if !contains(args, want) {
					t.Errorf("arguments %v do not contain %q", args, want)
				}
			}
			if contains(args, "0:v?") || contains(args, "0:v") {
				t.Errorf("arguments unexpectedly map existing target artwork: %v", args)
			}
			if format == ".m4a" {
				if !contains(args, "mjpeg") {
					t.Errorf("M4A arguments do not include the MJPEG cover-art codec: %v", args)
				}
				if contains(args, "use_metadata_tags") {
					t.Errorf("M4A arguments unexpectedly include use_metadata_tags: %v", args)
				}
			}
		})
	}

	t.Run("metadata disabled", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = true
		_, args, err := buildEmbedImageCommand("/work/track.flac", "/work/front.png", "/work/out.flac")
		if err != nil {
			t.Fatalf("buildEmbedImageCommand returned error: %v", err)
		}
		if !contains(args, "-map_metadata") || !contains(args, "-1") {
			t.Errorf("metadata-disabled arguments = %v, want -map_metadata -1", args)
		}
	})

	t.Run("docker paths", func(t *testing.T) {
		config.UseDocker = true
		config.NoPreserveMetadata = false
		config.SourceDir = "/host/source"
		config.TargetDir = "/host/target"
		commandName, args, err := buildEmbedImageCommand(
			"/host/target/album/track.flac",
			"/host/source/album/folder.PNG",
			"/host/target/album/.track.flac.embed-123.flac",
		)
		if err != nil {
			t.Fatalf("buildEmbedImageCommand returned error: %v", err)
		}
		if commandName != "docker" {
			t.Errorf("command = %q, want docker", commandName)
		}
		for _, want := range []string{
			"run", "--rm", "--entrypoint", "ffmpeg", "/source/album/folder.PNG",
			"/target/album/track.flac", "/target/album/.track.flac.embed-123.flac",
		} {
			if !contains(args, want) {
				t.Errorf("Docker arguments %v do not contain %q", args, want)
			}
		}
	})
}

// TestEmbedImageInAudioSuccess verifies temporary output replacement and
// preservation of the source audio when a hardlink was used.
func TestEmbedImageInAudioSuccess(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")
	imagePath := filepath.Join(tmpDir, "cover.jpg")
	if err := os.WriteFile(sourcePath, []byte("source-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("final-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}

	config = Config{EmbedCopiedImage: true, NoPreserveMetadata: false}
	var commandArgs []string
	runEmbedCommand = func(name string, args ...string) error {
		commandArgs = append([]string(nil), args...)
		outputPath := args[len(args)-1]
		return os.WriteFile(outputPath, []byte("embedded-audio"), 0o644)
	}

	if err := embedImageInAudio(sourcePath, targetPath, imagePath); err != nil {
		t.Fatalf("embedImageInAudio returned error: %v", err)
	}

	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	targetBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBytes) != "source-audio" {
		t.Errorf("source changed to %q", sourceBytes)
	}
	if string(targetBytes) != "embedded-audio" {
		t.Errorf("target = %q, want embedded-audio", targetBytes)
	}
	if len(commandArgs) == 0 || commandArgs[len(commandArgs)-1] == targetPath {
		t.Error("FFmpeg output path was not a temporary target path")
	}
	assertNoEmbedTempFiles(t, tmpDir, "target.flac")
}

// TestEmbedImageInAudioFailurePreservesTarget verifies that a failed FFmpeg
// run removes its temporary output and does not replace valid audio.
func TestEmbedImageInAudioFailurePreservesTarget(t *testing.T) {
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source.mp3")
	targetPath := filepath.Join(tmpDir, "target.mp3")
	imagePath := filepath.Join(tmpDir, "cover.png")
	if err := os.WriteFile(sourcePath, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, []byte("valid-audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(imagePath, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}

	runEmbedCommand = func(name string, args ...string) error {
		return errors.New("synthetic FFmpeg failure")
	}
	if err := embedImageInAudio(sourcePath, targetPath, imagePath); err == nil {
		t.Fatal("embedImageInAudio unexpectedly succeeded")
	}

	targetBytes, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(targetBytes) != "valid-audio" {
		t.Errorf("target changed to %q after failure", targetBytes)
	}
	assertNoEmbedTempFiles(t, tmpDir, "target.mp3")
}

// TestEmbedAudioOutputsContinuesAfterFailure verifies warnings identify the
// failed target and sidecar while later outputs are still processed.
func TestEmbedAudioOutputsContinuesAfterFailure(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	firstSource := filepath.Join(tmpDir, "first", "track.flac")
	secondSource := filepath.Join(tmpDir, "second", "track.flac")
	firstTarget := filepath.Join(tmpDir, "first-target.flac")
	secondTarget := filepath.Join(tmpDir, "second-target.flac")
	firstImage := filepath.Join(tmpDir, "first", "cover.jpg")
	secondImage := filepath.Join(tmpDir, "second", "cover.jpg")
	for _, path := range []string{firstSource, secondSource, firstTarget, secondTarget, firstImage, secondImage} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		firstSource: "first-source", secondSource: "second-source",
		firstTarget: "first-audio", secondTarget: "second-audio",
		firstImage: "first-image", secondImage: "second-image",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	config = Config{EmbedCopiedImage: true}
	calls := 0
	runEmbedCommand = func(name string, args ...string) error {
		calls++
		if calls == 1 {
			return errors.New("synthetic first failure")
		}
		return os.WriteFile(args[len(args)-1], []byte("second-embedded"), 0o644)
	}

	var output string
	var captureErr error
	output, captureErr = captureOutput(func() {
		embedAudioOutputs([]audioOutput{
			{sourcePath: firstSource, targetPath: firstTarget},
			{sourcePath: secondSource, targetPath: secondTarget},
		})
	})
	if captureErr != nil {
		t.Fatalf("captureOutput returned error: %v", captureErr)
	}
	if calls != 2 {
		t.Fatalf("FFmpeg calls = %d, want 2", calls)
	}
	firstBytes, err := os.ReadFile(firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(secondTarget)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != "first-audio" {
		t.Errorf("failed target changed to %q", firstBytes)
	}
	if string(secondBytes) != "second-embedded" {
		t.Errorf("second target = %q, want second-embedded", secondBytes)
	}
	if !strings.Contains(output, firstImage) || !strings.Contains(output, firstTarget) {
		t.Errorf("warning does not identify image and target: %q", output)
	}
}

// TestEmbedAudioOutputsNoSidecarDoesNotInvokeFFmpeg verifies that missing
// sidecars remain a true no-op.
func TestEmbedAudioOutputsNoSidecarDoesNotInvokeFFmpeg(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "track.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")
	for path, content := range map[string]string{sourcePath: "source", targetPath: "audio"} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config = Config{EmbedCopiedImage: true}
	calls := 0
	runEmbedCommand = func(name string, args ...string) error {
		calls++
		return nil
	}
	embedAudioOutputs([]audioOutput{{sourcePath: sourcePath, targetPath: targetPath}})
	if calls != 0 {
		t.Errorf("FFmpeg calls = %d, want 0", calls)
	}
}

// TestEmbedImageInAudioBreaksHardlink verifies the intentional hardlink
// exception for a successfully remuxed audio output.
func TestEmbedImageInAudioBreaksHardlink(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")
	imagePath := filepath.Join(tmpDir, "cover.jpg")
	for path, content := range map[string]string{
		sourcePath: "source-audio", imagePath: "image",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config = Config{PreferHardlinks: true, EmbedCopiedImage: true}
	if err := copyFile(sourcePath, targetPath); err != nil {
		t.Fatal(err)
	}
	beforeSource, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	beforeTarget, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(beforeSource, beforeTarget) {
		t.Fatal("test setup did not create a hardlink")
	}

	runEmbedCommand = func(name string, args ...string) error {
		return os.WriteFile(args[len(args)-1], []byte("embedded-audio"), 0o644)
	}
	if err := embedImageInAudio(sourcePath, targetPath, imagePath); err != nil {
		t.Fatalf("embedImageInAudio returned error: %v", err)
	}

	afterSource, err := os.Stat(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	afterTarget, err := os.Stat(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(afterSource, afterTarget) {
		t.Error("embedded target still shares the source inode")
	}
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceBytes) != "source-audio" {
		t.Errorf("hardlinked source changed to %q", sourceBytes)
	}
}

// TestDefaultConversionTargetPath verifies the default ALAC-to-FLAC and FLAC
// target-path rules used by the output inventory.
func TestDefaultConversionTargetPath(t *testing.T) {
	if got := defaultConversionTargetPath("/target/song.m4a", "alac"); got != "/target/song.flac" {
		t.Errorf("ALAC target = %q, want /target/song.flac", got)
	}
	if got := defaultConversionTargetPath("/target/song.flac", "flac"); got != "/target/song.flac" {
		t.Errorf("FLAC target = %q, want /target/song.flac", got)
	}
}

// TestDefaultFallbackTargetPath verifies that failed default ALAC conversion
// does not place M4A bytes under a FLAC filename.
func TestDefaultFallbackTargetPath(t *testing.T) {
	if got := defaultFallbackTargetPath("/target/song.flac", "/target/song.m4a", "alac"); got != "/target/song.m4a" {
		t.Errorf("ALAC fallback = %q, want /target/song.m4a", got)
	}
	if got := defaultFallbackTargetPath("/target/song.flac", "/target/song.flac", "flac"); got != "/target/song.flac" {
		t.Errorf("FLAC fallback = %q, want /target/song.flac", got)
	}
}

// TestEmbedAudioOutputsDoesNotCopyStandaloneSidecars verifies that enabling
// --copy-images alongside embedding does not couple the two operations.
func TestEmbedAudioOutputsDoesNotCopyStandaloneSidecars(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	originalRunner := runEmbedCommand
	t.Cleanup(func() { runEmbedCommand = originalRunner })

	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(sourceDir, "track.mp3")
	targetPath := filepath.Join(targetDir, "target.mp3")
	imagePath := filepath.Join(sourceDir, "cover.jpg")
	for path, content := range map[string]string{
		sourcePath: "source", targetPath: "audio", imagePath: "image",
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config = Config{CopyImages: true, EmbedCopiedImage: true}
	runEmbedCommand = func(name string, args ...string) error {
		return os.WriteFile(args[len(args)-1], []byte("embedded"), 0o644)
	}
	embedAudioOutputs([]audioOutput{{sourcePath: sourcePath, targetPath: targetPath}})
	if _, err := os.Stat(filepath.Join(targetDir, "cover.jpg")); !os.IsNotExist(err) {
		t.Error("embedding pass copied the standalone sidecar unexpectedly")
	}
}

// TestProcessAudioFilesWithOutputsInventory verifies that the inventory
// contains only current successful outputs and preserves nested source paths.
func TestProcessAudioFilesWithOutputsInventory(t *testing.T) {
	originalConfig := config
	t.Cleanup(func() { config = originalConfig })

	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")
	nestedDir := filepath.Join(sourceDir, "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(sourceDir, "song.flac"):   "dummy flac",
		filepath.Join(sourceDir, "song.mp3"):    "dummy mp3",
		filepath.Join(sourceDir, "song.m4a"):    "dummy m4a",
		filepath.Join(nestedDir, "nested.flac"): "dummy nested",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "stale.flac"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	config = Config{
		SourceDir:          sourceDir,
		TargetDir:          targetDir,
		NoPreserveMetadata: true,
		SoxCommand:         "true",
	}
	outputs, err := processAudioFilesWithOutputs()
	if err != nil {
		t.Fatalf("processAudioFilesWithOutputs returned error: %v", err)
	}
	if len(outputs) != 4 {
		t.Fatalf("inventory length = %d, want 4", len(outputs))
	}
	for _, output := range outputs {
		if output.sourcePath == "" || output.targetPath == "" {
			t.Errorf("incomplete inventory record: %+v", output)
		}
		if output.targetPath == filepath.Join(targetDir, "stale.flac") {
			t.Error("inventory included stale target file")
		}
	}
	seen := map[string]bool{}
	for _, output := range outputs {
		seen[filepath.Base(output.targetPath)] = true
	}
	for _, name := range []string{"song.flac", "song.mp3", "song.m4a", "nested.flac"} {
		if !seen[name] {
			t.Errorf("inventory missing %s: %+v", name, outputs)
		}
	}
}

// TestEnforcedOutputPath verifies actual target-extension selection for all
// enforced formats, including the intentional MP3 exceptions.
func TestEnforcedOutputPath(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		sourceExt string
		enforced  string
		expected  string
	}{
		{name: "flac to flac", target: "/target/song.flac", sourceExt: ".flac", enforced: "flac", expected: "/target/song.flac"},
		{name: "alac to flac", target: "/target/song.m4a", sourceExt: ".m4a", enforced: "flac", expected: "/target/song.flac"},
		{name: "mp3 remains under flac", target: "/target/song.mp3", sourceExt: ".mp3", enforced: "flac", expected: "/target/song.mp3"},
		{name: "uppercase mp3 remains under flac", target: "/target/song.MP3", sourceExt: ".mp3", enforced: "flac", expected: "/target/song.mp3"},
		{name: "flac to mp3", target: "/target/song.flac", sourceExt: ".flac", enforced: "mp3", expected: "/target/song.mp3"},
		{name: "mp3 remains under mp3", target: "/target/song.mp3", sourceExt: ".mp3", enforced: "mp3", expected: "/target/song.mp3"},
		{name: "uppercase mp3 remains under mp3", target: "/target/song.MP3", sourceExt: ".MP3", enforced: "mp3", expected: "/target/song.mp3"},
		{name: "alac to mp3", target: "/target/song.m4a", sourceExt: ".m4a", enforced: "mp3", expected: "/target/song.mp3"},
		{name: "flac to alac", target: "/target/song.flac", sourceExt: ".flac", enforced: "alac", expected: "/target/song.m4a"},
		{name: "mp3 remains under alac", target: "/target/song.mp3", sourceExt: ".mp3", enforced: "alac", expected: "/target/song.mp3"},
		{name: "uppercase mp3 remains under alac", target: "/target/song.MP3", sourceExt: ".MP3", enforced: "alac", expected: "/target/song.mp3"},
		{name: "alac to alac", target: "/target/song.m4a", sourceExt: ".m4a", enforced: "alac", expected: "/target/song.m4a"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := enforcedOutputPath(test.target, test.sourceExt, test.enforced); got != test.expected {
				t.Errorf("enforcedOutputPath() = %q, want %q", got, test.expected)
			}
		})
	}
}

// assertNoEmbedTempFiles verifies that no temporary embedding artifact remains
// for a target basename.
func assertNoEmbedTempFiles(t *testing.T, dir, baseName string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read temporary directory: %v", err)
	}
	prefix := "." + baseName + ".embed-"
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) {
			t.Errorf("temporary embedding artifact remains: %s", entry.Name())
		}
	}
}
