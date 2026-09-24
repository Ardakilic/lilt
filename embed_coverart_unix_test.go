//go:build darwin || linux

package main

// Unix-only tests for --embed-cover-art review feedback. These rely on POSIX
// shell-script stubs on PATH, fifo files, and POSIX UIDs, so they are
// excluded on Windows (CI runs linux/darwin/windows).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// writeStubBinary writes an executable shell stub into dir and returns its path.
func writeStubBinary(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0755); err != nil {
		t.Fatalf("failed to write stub %s: %v", name, err)
	}
	return p
}

func TestSetupSoxCommandEmbedCoverArtNamesFlag(t *testing.T) {
	// A stub sox on an otherwise empty PATH: sox resolves, ffmpeg does not.
	bindir := t.TempDir()
	writeStubBinary(t, bindir, "sox", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", bindir)

	newConfig := func() Config {
		return Config{
			UseDocker:  false,
			SourceDir:  t.TempDir(),
			TargetDir:  t.TempDir(),
			SoxCommand: "sox",
		}
	}
	originalConfig := config
	defer func() { config = originalConfig }()

	t.Run("EmbedCoverArtNamesFlag", func(t *testing.T) {
		config = newConfig()
		config.EmbedCoverArt = true
		config.NoPreserveMetadata = true
		err := setupSoxCommand()
		if err == nil {
			t.Fatal("expected ffmpeg-missing error, got nil")
		}
		if !strings.Contains(err.Error(), "--embed-cover-art") {
			t.Errorf("expected error to name --embed-cover-art, got: %v", err)
		}
	})

	t.Run("GenericErrorPreserved", func(t *testing.T) {
		config = newConfig()
		config.EmbedCoverArt = false
		config.NoPreserveMetadata = false
		err := setupSoxCommand()
		if err == nil {
			t.Fatal("expected ffmpeg-missing error, got nil")
		}
		if strings.Contains(err.Error(), "--embed-cover-art") {
			t.Errorf("generic error must not name --embed-cover-art, got: %v", err)
		}
		if !strings.Contains(err.Error(), "ffmpeg is not installed") {
			t.Errorf("expected generic ffmpeg error, got: %v", err)
		}
	})

	t.Run("NoFFmpegNeeded", func(t *testing.T) {
		config = newConfig()
		config.EmbedCoverArt = false
		config.NoPreserveMetadata = true
		if err := setupSoxCommand(); err != nil {
			t.Errorf("expected success without ALAC/metadata/embed, got: %v", err)
		}
	})
}

func TestEmbedImageIntoAudioDockerUsesHostUser(t *testing.T) {
	// A stub docker records its args and exits 0 without creating the temp
	// output, so embedding deterministically fails at the mode-restore step
	// after the docker invocation has been observed.
	bindir := t.TempDir()
	argsLog := filepath.Join(bindir, "docker.args")
	writeStubBinary(t, bindir, "docker", "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$LILT_TEST_DOCKER_ARGS\"\nexit 0\n")
	t.Setenv("PATH", bindir)
	t.Setenv("LILT_TEST_DOCKER_ARGS", argsLog)

	originalConfig := config
	defer func() { config = originalConfig }()

	root := t.TempDir()
	srcDir := filepath.Join(root, "src")
	dstDir := filepath.Join(root, "dst")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		t.Fatal(err)
	}
	audio := filepath.Join(dstDir, "song.flac")
	if err := os.WriteFile(audio, []byte("fake flac bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	image := filepath.Join(srcDir, "cover.jpg")
	if err := os.WriteFile(image, []byte("fake jpg bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	config = Config{
		UseDocker:   true,
		SourceDir:   srcDir,
		TargetDir:   dstDir,
		DockerImage: "test/image",
	}

	err := embedImageIntoAudio(audio, image)
	if err == nil || !strings.Contains(err.Error(), "failed to restore mode") {
		t.Fatalf("expected restore-mode failure (stub creates no tmp), got: %v", err)
	}
	raw, readErr := os.ReadFile(argsLog)
	if readErr != nil {
		t.Fatalf("docker stub logged no args: %v", readErr)
	}
	log := string(raw)
	for _, want := range []string{
		"--entrypoint\nffmpeg",
		fmt.Sprintf("--user\n%d:%d", os.Getuid(), os.Getgid()),
		"/source/cover.jpg",
		"/target/song.flac",
		"/target/song.embed.tmp.flac",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("docker args missing %q, got:\n%s", want, log)
		}
	}
}

func TestFindCoverImageSkipsNonRegularFiles(t *testing.T) {
	// A fifo named cover.jpg must not shadow the regular front.jpg, and a
	// fifo-only directory must resolve to ("", false).
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "cover.jpg"), 0644); err != nil {
		t.Fatalf("mkfifo failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "front.jpg"), []byte("front"), 0644); err != nil {
		t.Fatal(err)
	}
	got, ok := findCoverImage(dir)
	if !ok {
		t.Fatal("expected front.jpg, got none")
	}
	if filepath.Base(got) != "front.jpg" {
		t.Errorf("expected fifo cover.jpg to be skipped in favor of front.jpg, got %q", got)
	}

	only := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(only, "cover.jpg"), 0644); err != nil {
		t.Fatalf("mkfifo failed: %v", err)
	}
	if got, ok := findCoverImage(only); ok || got != "" {
		t.Errorf("expected (\"\", false) for fifo-only dir, got (%q, %v)", got, ok)
	}
}
