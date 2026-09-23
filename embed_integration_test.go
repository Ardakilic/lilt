package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// integrationProbe is the subset of ffprobe output needed to verify that an
// output contains one audio stream and one attached-picture stream.
type integrationProbe struct {
	Streams []struct {
		CodecType   string         `json:"codec_type"`
		Disposition map[string]int `json:"disposition"`
	} `json:"streams"`
}

// TestIntegrationEmbedSidecarFormats generates short audio and image fixtures
// and verifies real FFmpeg attachment for FLAC, MP3, and M4A outputs. The test
// is skipped when the host lacks FFmpeg, ffprobe, or a required encoder.
func TestIntegrationEmbedSidecarFormats(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skipf("ffmpeg is not installed: %v", err)
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skipf("ffprobe is not installed: %v", err)
	}

	originalConfig := config
	t.Cleanup(func() { config = originalConfig })
	config = Config{EmbedCopiedImage: true, NoPreserveMetadata: true}

	for _, format := range []string{".flac", ".mp3", ".m4a"} {
		t.Run(format[1:], func(t *testing.T) {
			tmpDir := t.TempDir()
			sourcePath := filepath.Join(tmpDir, "source"+format)
			targetPath := filepath.Join(tmpDir, "target"+format)

			encoder := map[string]string{
				".flac": "flac",
				".mp3":  "libmp3lame",
				".m4a":  "alac",
			}[format]
			if !hasFFmpegEncoder(t, ffmpeg, encoder) {
				t.Skipf("FFmpeg encoder %q is not available", encoder)
			}
			runMediaCommand(t, ffmpeg, "-f", "lavfi", "-i", "sine=frequency=1000:duration=0.2", "-c:a", encoder, sourcePath)
			if err := copyFile(sourcePath, targetPath); err != nil {
				t.Fatalf("copy generated media: %v", err)
			}

			for _, imageExt := range []string{".jpg", ".png"} {
				imagePath := filepath.Join(tmpDir, "cover"+imageExt)
				runMediaCommand(t, ffmpeg, "-f", "lavfi", "-i", "color=c=red:size=16x16", "-frames:v", "1", imagePath)
				if err := embedImageInAudio(sourcePath, targetPath, imagePath); err != nil {
					t.Fatalf("embed %s: %v", imageExt, err)
				}

				probe := readIntegrationProbe(t, ffprobe, targetPath)
				if got := countIntegrationStreams(probe, "audio"); got != 1 {
					t.Errorf("audio streams = %d, want 1", got)
				}
				if got := countIntegrationStreams(probe, "video"); got != 1 {
					t.Errorf("video streams = %d, want 1", got)
				}
				if !hasAttachedPicture(probe) {
					t.Errorf("output has no attached_pic stream: %+v", probe.Streams)
				}
			}
		})
	}
}

// hasFFmpegEncoder reports whether the selected FFmpeg build exposes an
// encoder. It is used so optional tests can skip unsupported MP3 builds rather
// than fail for an environment capability unrelated to Lilt.
func hasFFmpegEncoder(t *testing.T, ffmpeg, encoder string) bool {
	t.Helper()
	output, err := exec.Command(ffmpeg, "-hide_banner", "-encoders").Output()
	if err != nil {
		return false
	}
	return containsIntegrationEncoder(string(output), encoder)
}

// containsIntegrationEncoder checks the plain-text output of ffmpeg -encoders.
func containsIntegrationEncoder(output, encoder string) bool {
	for _, line := range []string{output} {
		if len(line) > 0 && containsIntegrationSubstring(line, encoder) {
			return true
		}
	}
	return false
}

// containsIntegrationSubstring is a small allocation-free substring helper
// for the ffmpeg encoder listing.
func containsIntegrationSubstring(text, substring string) bool {
	if substring == "" {
		return true
	}
	for index := 0; index+len(substring) <= len(text); index++ {
		if text[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}

// runMediaCommand executes a generated-media setup command and fails the test
// when the command cannot create its fixture.
func runMediaCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("media command %s failed: %v\n%s", name, err, output)
	}
}

// readIntegrationProbe runs ffprobe and decodes its stream-level JSON output.
func readIntegrationProbe(t *testing.T, ffprobe, path string) integrationProbe {
	t.Helper()
	output, err := exec.Command(ffprobe, "-v", "error", "-show_streams", "-of", "json", path).Output()
	if err != nil {
		t.Fatalf("ffprobe %s failed: %v", path, err)
	}
	var probe integrationProbe
	if err := json.Unmarshal(output, &probe); err != nil {
		t.Fatalf("decode ffprobe output for %s: %v", path, err)
	}
	return probe
}

// countIntegrationStreams counts streams of a particular codec type.
func countIntegrationStreams(probe integrationProbe, codecType string) int {
	count := 0
	for _, stream := range probe.Streams {
		if stream.CodecType == codecType {
			count++
		}
	}
	return count
}

// hasAttachedPicture reports whether any probed stream has the attached_pic
// disposition enabled.
func hasAttachedPicture(probe integrationProbe) bool {
	for _, stream := range probe.Streams {
		if stream.Disposition["attached_pic"] == 1 {
			return true
		}
	}
	return false
}
