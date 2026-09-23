package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// runEmbedCommand executes an FFmpeg or Docker command. It is a package-level
// seam so tests can exercise replacement and failure behavior without media
// tools installed.
var runEmbedCommand = func(name string, args ...string) error {
	return runExternalCommand(name, args...)
}

// buildEmbedImageCommand constructs the local or Docker FFmpeg command that
// remuxes a final audio output with a selected sidecar image. The audio and
// image inputs are mapped explicitly so existing attached-picture streams in
// the final audio file are not copied.
func buildEmbedImageCommand(audioPath, imagePath, outputPath string) (string, []string, error) {
	imageExt := strings.ToLower(filepath.Ext(imagePath))
	if imageExt != ".jpg" && imageExt != ".png" {
		return "", nil, fmt.Errorf("unsupported sidecar image format: %s", imageExt)
	}

	format := strings.ToLower(filepath.Ext(outputPath))
	if format != ".flac" && format != ".mp3" && format != ".m4a" {
		return "", nil, fmt.Errorf("unsupported audio output format for image embedding: %s", format)
	}

	commandName := "ffmpeg"
	ffmpegOutputPath := outputPath
	var args []string
	if config.UseDocker {
		commandName = "docker"
		args = []string{
			"run", "--rm", "--entrypoint", "ffmpeg",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage,
			"-y",
			"-i", getDockerTargetPath(audioPath),
			"-i", getDockerPath(imagePath),
		}
		ffmpegOutputPath = getDockerTargetPath(outputPath)
	} else {
		args = []string{
			"-y",
			"-i", audioPath,
			"-i", imagePath,
		}
	}

	metadataMap := "0"
	chapterMap := "0"
	if config.NoPreserveMetadata {
		metadataMap = "-1"
		chapterMap = "-1"
	}

	args = append(args,
		"-map", "0:a:0",
		"-map", "1:v:0",
		"-map_metadata", metadataMap,
		"-map_chapters", chapterMap,
		"-c:a", "copy",
	)

	// MP4/M4A cover art is most reliable as an MJPEG attached-picture
	// stream. FLAC and MP3 can carry the original JPEG/PNG stream directly.
	videoCodec := "copy"
	if format == ".m4a" {
		videoCodec = "mjpeg"
	}
	args = append(args,
		"-c:v", videoCodec,
		"-disposition:v:0", "attached_pic",
		"-metadata:s:v:0", "title=Album cover",
		"-metadata:s:v:0", "comment=Cover (Front)",
		ffmpegOutputPath,
	)

	return commandName, args, nil
}

// pathsReferToSamePath reports whether two path strings identify the same
// directory entry. It deliberately does not use os.SameFile, because a
// successful embedding must be allowed to detach a hardlinked target.
func pathsReferToSamePath(path1, path2 string) bool {
	absPath1, err1 := filepath.Abs(path1)
	absPath2, err2 := filepath.Abs(path2)
	if err1 != nil || err2 != nil {
		return filepath.Clean(path1) == filepath.Clean(path2)
	}
	return filepath.Clean(absPath1) == filepath.Clean(absPath2)
}

// embedImageInAudio safely embeds imagePath into the final audio output at
// audioPath. The source audio path is accepted separately so a target that is
// the same file as the source is never rewritten in place. The output is
// written to a temporary file and replaces audioPath only after success.
func embedImageInAudio(sourcePath, audioPath, imagePath string) error {
	if pathsReferToSamePath(sourcePath, audioPath) {
		return fmt.Errorf("source and target refer to the same path: %s", audioPath)
	}

	audioInfo, err := os.Stat(audioPath)
	if err != nil {
		return fmt.Errorf("stat final audio output %s: %w", audioPath, err)
	}
	if !audioInfo.Mode().IsRegular() {
		return fmt.Errorf("final audio output is not a regular file: %s", audioPath)
	}

	imageInfo, err := os.Stat(imagePath)
	if err != nil {
		return fmt.Errorf("stat sidecar image %s: %w", imagePath, err)
	}
	if !imageInfo.Mode().IsRegular() {
		return fmt.Errorf("sidecar image is not a regular file: %s", imagePath)
	}

	tempPath, err := createEmbedTempPath(audioPath)
	if err != nil {
		return err
	}
	defer os.Remove(tempPath)

	commandName, args, err := buildEmbedImageCommand(audioPath, imagePath, tempPath)
	if err != nil {
		return err
	}
	if err := runEmbedCommand(commandName, args...); err != nil {
		return fmt.Errorf("FFmpeg sidecar embedding failed: %w", err)
	}

	// Keep the final target's permissions when possible. FFmpeg may have
	// created the temporary file with a narrower mode.
	if err := os.Chmod(tempPath, audioInfo.Mode().Perm()); err != nil {
		return fmt.Errorf("preserve final audio permissions: %w", err)
	}
	if err := replaceFileSafely(tempPath, audioPath); err != nil {
		return fmt.Errorf("replace final audio output: %w", err)
	}
	return nil
}

// embedAudioOutputs embeds the selected sidecar beside each source into the
// corresponding final target. Embedding is a no-op for files without a
// sidecar, and a failure is reported as a warning without stopping later
// outputs from being processed.
func embedAudioOutputs(outputs []audioOutput) {
	if !config.EmbedCopiedImage {
		return
	}

	for _, output := range outputs {
		imagePath, found, err := resolveSidecarImage(output.sourcePath)
		if err != nil {
			fmt.Printf("Warning: Could not find sidecar image for %s: %v\n", output.sourcePath, err)
			continue
		}
		if !found {
			continue
		}

		if err := embedImageInAudio(output.sourcePath, output.targetPath, imagePath); err != nil {
			fmt.Printf("Warning: Could not embed %s in %s: %v\n", imagePath, output.targetPath, err)
		}
	}
}

// createEmbedTempPath creates a uniquely named temporary output beside the
// final target while preserving the target extension for FFmpeg muxer
// selection.
func createEmbedTempPath(targetPath string) (string, error) {
	targetDir := filepath.Dir(targetPath)
	tempFile, err := os.CreateTemp(targetDir, "."+filepath.Base(targetPath)+".embed-*"+filepath.Ext(targetPath))
	if err != nil {
		return "", fmt.Errorf("create temporary embedding output beside %s: %w", targetPath, err)
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("close temporary embedding output %s: %w", tempPath, err)
	}
	if err := os.Chmod(tempPath, 0o666); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("make temporary embedding output writable: %w", err)
	}
	return tempPath, nil
}

// replaceFileSafely replaces target with temp, using a rollback-capable backup
// when the platform refuses to overwrite an existing destination directly.
func replaceFileSafely(tempPath, targetPath string) error {
	if err := os.Rename(tempPath, targetPath); err == nil {
		return nil
	}

	backupFile, err := os.CreateTemp(filepath.Dir(targetPath), "."+filepath.Base(targetPath)+".backup-*")
	if err != nil {
		return fmt.Errorf("create replacement backup: %w", err)
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("close replacement backup: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return fmt.Errorf("prepare replacement backup: %w", err)
	}

	if err := os.Rename(targetPath, backupPath); err != nil {
		return fmt.Errorf("move existing target aside: %w", err)
	}
	if err := os.Rename(tempPath, targetPath); err != nil {
		if restoreErr := os.Rename(backupPath, targetPath); restoreErr != nil {
			return fmt.Errorf("replace target: %v; restore target: %w", err, restoreErr)
		}
		return fmt.Errorf("replace target: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		fmt.Printf("Warning: Failed to remove replacement backup %s: %v\n", backupPath, err)
	}
	return nil
}

// runExternalCommand executes an external command with argument slices intact.
func runExternalCommand(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
