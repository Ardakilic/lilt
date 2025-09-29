package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Config holds the application configuration
type Config struct {
	SourceDir           string
	TargetDir           string
	CopyImages          bool
	UseDocker           bool
	DockerImage         string
	SoxCommand          string
	NoPreserveMetadata  bool
	EnforceOutputFormat string // "flac", "mp3", "alac", or empty for default behavior
}

// AudioInfo holds information about an audio file
type AudioInfo struct {
	Bits   int
	Rate   int
	Format string // "flac" or "alac"
}

var (
	config         Config
	version        = "dev" // This will be set during build time
	selfUpdateFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "lilt <source_directory>",
	Short: "Convert Hi-Res FLAC/ALAC files to 16-bit FLAC files",
	Long: `Lilt - FLAC/ALAC Audio Converter

This tool converts Hi-Res FLAC and ALAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
It also copies MP3 files and image files (JPG, PNG) to the target directory.

With the --enforce-output-format flag, you can convert all audio files to a specific format:
- flac: Convert all files to 16-bit FLAC
- mp3: Convert all files to 320kbps MP3
- alac: Convert all files to 16-bit ALAC (M4A)

Copyright (C) 2025 Arda Kilicdagi
Licensed under MIT License`,
	Args:    cobra.MaximumNArgs(1),
	RunE:    runConverter,
	Version: version,
}

func init() {
	rootCmd.Flags().StringVar(&config.TargetDir, "target-dir", "./transcoded", "Specify target directory")
	rootCmd.Flags().BoolVar(&config.CopyImages, "copy-images", false, "Copy JPG and PNG files")
	rootCmd.Flags().BoolVar(&config.UseDocker, "use-docker", false, "Use Docker to run Sox instead of local installation")
	rootCmd.Flags().StringVar(&config.DockerImage, "docker-image", "ardakilic/sox_ng:latest", "Specify Docker image")
	rootCmd.Flags().BoolVar(&config.NoPreserveMetadata, "no-preserve-metadata", false, "Do not preserve ID3 tags and cover art using FFmpeg (metadata is preserved by default)")
	rootCmd.Flags().StringVar(&config.EnforceOutputFormat, "enforce-output-format", "", "Enforce output format for all files: flac, mp3, or alac")
	rootCmd.Flags().BoolVar(&selfUpdateFlag, "self-update", false, "Check for updates and self-update if newer version available")

	// Set default values
	config.SoxCommand = "sox"
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runConverter(cmd *cobra.Command, args []string) error {
	if selfUpdateFlag {
		if len(args) > 0 {
			return fmt.Errorf("--self-update does not take arguments")
		}
		return selfUpdate(http.DefaultClient)
	}

	if len(args) == 0 {
		return fmt.Errorf("source directory required")
	}

	config.SourceDir = args[0]

	// Validate enforce-output-format flag
	if config.EnforceOutputFormat != "" {
		validFormats := []string{"flac", "mp3", "alac"}
		if !slices.Contains(validFormats, config.EnforceOutputFormat) {
			return fmt.Errorf("invalid enforce-output-format: %s. Valid options are: flac, mp3, alac", config.EnforceOutputFormat)
		}
	}

	// Validate source directory
	if _, err := os.Stat(config.SourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", config.SourceDir)
	}

	// Setup Sox command
	if err := setupSoxCommand(); err != nil {
		return err
	}

	// Create target directory
	if err := os.MkdirAll(config.TargetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Process audio files
	if err := processAudioFiles(); err != nil {
		return err
	}

	// Copy image files if requested
	if config.CopyImages {
		if err := copyImageFiles(); err != nil {
			return err
		}
	}

	fmt.Println("Processing complete!")
	return nil
}

func setupSoxCommand() error {
	if config.UseDocker {
		// Check if docker is installed
		if _, err := exec.LookPath("docker"); err != nil {
			return fmt.Errorf("docker is not installed. Please install Docker to use this option")
		}

		// Get absolute paths
		sourceAbs, err := filepath.Abs(config.SourceDir)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for source directory: %w", err)
		}

		targetAbs, err := filepath.Abs(config.TargetDir)
		if err != nil {
			return fmt.Errorf("failed to get absolute path for target directory: %w", err)
		}

		config.SourceDir = sourceAbs
		config.TargetDir = targetAbs
	} else {
		// Check if sox is installed locally
		if _, err := exec.LookPath(config.SoxCommand); err != nil {
			return fmt.Errorf("sox is not installed. Please install sox or use --use-docker option")
		}

		// Check for FFmpeg only when needed
		needsFFmpeg := !config.NoPreserveMetadata

		// Quick check if directory contains ALAC files (if metadata preservation is disabled)
		if !needsFFmpeg {
			hasALAC, _ := hasALACFiles(config.SourceDir)
			needsFFmpeg = hasALAC
		}

		if needsFFmpeg {
			if _, err := exec.LookPath("ffmpeg"); err != nil {
				return fmt.Errorf("ffmpeg is not installed. Please install FFmpeg for ALAC support and metadata preservation, or use --use-docker option")
			}
		}
	}
	return nil
}

func hasALACFiles(dir string) (bool, error) {
	hasALAC := false
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking even if there's an error with a specific file
		}

		if !info.IsDir() && strings.ToLower(filepath.Ext(path)) == ".m4a" {
			hasALAC = true
			return filepath.SkipAll // Found ALAC file, no need to continue
		}
		return nil
	})
	return hasALAC, err
}

func processAudioFiles() error {
	return filepath.Walk(config.SourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".flac" && ext != ".mp3" && ext != ".m4a" {
			return nil
		}

		fmt.Printf("Processing: %s\n", path)

		// Create target directory structure
		relPath, err := filepath.Rel(config.SourceDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(config.TargetDir, relPath)
		targetDir := filepath.Dir(targetPath)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}

		// Handle enforce-output-format mode
		if config.EnforceOutputFormat != "" {
			return processAudioFileWithEnforcedFormat(path, targetPath, ext)
		}

		// Original processing logic when no format enforcement
		// Handle MP3 files - just copy them
		if ext == ".mp3" {
			fmt.Printf("Copying MP3 file: %s\n", path)
			return copyFile(path, targetPath)
		}

		// Process FLAC and ALAC files
		audioInfo, err := getAudioInfo(path)
		if err != nil {
			fmt.Printf("Warning: Could not get audio info for %s, copying original\n", path)
			return copyFile(path, targetPath)
		}

		fmt.Printf("Detected: %d bits, %d Hz, %s format\n", audioInfo.Bits, audioInfo.Rate, audioInfo.Format)

		needsConversion, bitrateArgs, sampleRateArgs := determineConversion(audioInfo)

		if needsConversion || audioInfo.Format == "alac" {
			// Determine target sample rate for display based on source rate
			var targetRate string
			switch audioInfo.Rate {
			case 48000, 96000, 192000, 384000:
				targetRate = "48000 Hz"
			case 44100, 88200, 176400, 352800:
				targetRate = "44100 Hz"
			default:
				targetRate = "same rate"
			}

			if audioInfo.Format == "alac" {
				if needsConversion {
					fmt.Printf("Converting ALAC to FLAC: %s (%d-bit %d Hz → 16-bit %s)\n", path, audioInfo.Bits, audioInfo.Rate, targetRate)
				} else {
					fmt.Printf("Converting ALAC to FLAC: %s (maintaining %d-bit %d Hz)\n", path, audioInfo.Bits, audioInfo.Rate)
				}
				// Always convert ALAC to FLAC, even if bit depth and sample rate are acceptable
				targetPath = changeExtensionToFlac(targetPath)
			} else {
				fmt.Printf("Converting FLAC: %s (%d-bit %d Hz → 16-bit %s)\n", path, audioInfo.Bits, audioInfo.Rate, targetRate)
			}

			if err := processAudioFile(path, targetPath, audioInfo, needsConversion, bitrateArgs, sampleRateArgs); err != nil {
				fmt.Printf("Error: Audio conversion failed. Copying original file instead. Error: %v\n", err)
				return copyFile(path, targetPath)
			}
		} else {
			fmt.Printf("Copying FLAC: %s\n", path)
			return copyFile(path, targetPath)
		}

		return nil
	})
}

func processAudioFileWithEnforcedFormat(sourcePath, targetPath, sourceExt string) error {
	// Get audio info for source file
	var audioInfo *AudioInfo
	var err error

	// Skip MP3 files if they don't need processing
	if sourceExt == ".mp3" && config.EnforceOutputFormat == "mp3" {
		fmt.Printf("Copying MP3 file: %s (already in target format)\n", sourcePath)
		return copyFile(sourcePath, targetPath)
	}

	// Get audio info for FLAC and ALAC files
	if sourceExt == ".flac" || sourceExt == ".m4a" {
		audioInfo, err = getAudioInfo(sourcePath)
		if err != nil {
			fmt.Printf("Warning: Could not get audio info for %s, copying original\n", sourcePath)
			return copyFile(sourcePath, targetPath)
		}
		fmt.Printf("Detected: %d bits, %d Hz, %s format\n", audioInfo.Bits, audioInfo.Rate, audioInfo.Format)
	}

	// Determine target file extension and process accordingly
	switch config.EnforceOutputFormat {
	case "flac":
		return processToFLAC(sourcePath, targetPath, sourceExt, audioInfo)
	case "mp3":
		return processToMP3(sourcePath, targetPath, sourceExt, audioInfo)
	case "alac":
		return processToALAC(sourcePath, targetPath, sourceExt, audioInfo)
	default:
		return fmt.Errorf("unsupported enforce-output-format: %s", config.EnforceOutputFormat)
	}
}

func processToFLAC(sourcePath, targetPath, sourceExt string, audioInfo *AudioInfo) error {
	// Change target extension to .flac
	targetPath = changeExtensionToFlac(targetPath)

	if sourceExt == ".mp3" {
		// Never convert MP3 to FLAC - just copy the original MP3
		fmt.Printf("Copying MP3: %s (MP3 files are not converted to lossless formats)\n", sourcePath)
		// Keep original extension for MP3
		originalTargetPath := strings.TrimSuffix(targetPath, ".flac") + ".mp3"
		return copyFile(sourcePath, originalTargetPath)
	}

	if sourceExt == ".flac" && audioInfo != nil {
		// Check if FLAC needs conversion or can be copied
		needsConversion, bitrateArgs, sampleRateArgs := determineConversion(audioInfo)
		if !needsConversion {
			fmt.Printf("Copying FLAC: %s (already 16-bit)\n", sourcePath)
			return copyFile(sourcePath, targetPath)
		} else {
			fmt.Printf("Converting FLAC: %s (reducing quality to 16-bit)\n", sourcePath)
			return processAudioFile(sourcePath, targetPath, audioInfo, needsConversion, bitrateArgs, sampleRateArgs)
		}
	}

	if sourceExt == ".m4a" && audioInfo != nil {
		// Convert ALAC to FLAC
		needsConversion, bitrateArgs, sampleRateArgs := determineConversion(audioInfo)
		if needsConversion {
			fmt.Printf("Converting ALAC to FLAC: %s (reducing quality to 16-bit)\n", sourcePath)
		} else {
			fmt.Printf("Converting ALAC to FLAC: %s (maintaining quality)\n", sourcePath)
		}
		return processAudioFile(sourcePath, targetPath, audioInfo, needsConversion, bitrateArgs, sampleRateArgs)
	}

	return fmt.Errorf("unsupported source format for FLAC conversion: %s", sourceExt)
}

func processToMP3(sourcePath, targetPath, sourceExt string, audioInfo *AudioInfo) error {
	// Change target extension to .mp3
	targetPath = changeExtensionToMP3(targetPath)

	if sourceExt == ".mp3" {
		fmt.Printf("Copying MP3: %s (already in target format)\n", sourcePath)
		return copyFile(sourcePath, targetPath)
	}

	// Convert FLAC or ALAC to MP3 at 320kbps
	fmt.Printf("Converting %s to MP3: %s (320kbps)\n", strings.ToUpper(strings.TrimPrefix(sourceExt, ".")), sourcePath)
	return convertToMP3(sourcePath, targetPath, audioInfo)
}

func processToALAC(sourcePath, targetPath, sourceExt string, audioInfo *AudioInfo) error {
	// Change target extension to .m4a
	targetPath = changeExtensionToM4A(targetPath)

	if sourceExt == ".m4a" && audioInfo != nil {
		// Check if ALAC needs conversion or can be copied
		if audioInfo.Bits == 16 && (audioInfo.Rate == 44100 || audioInfo.Rate == 48000) {
			fmt.Printf("Copying ALAC: %s (already 16-bit)\n", sourcePath)
			return copyFile(sourcePath, targetPath)
		} else {
			fmt.Printf("Converting ALAC: %s (reducing quality to 16-bit)\n", sourcePath)
			return convertToALAC(sourcePath, targetPath, audioInfo)
		}
	}

	if sourceExt == ".flac" {
		// Convert FLAC to ALAC
		fmt.Printf("Converting FLAC to ALAC: %s\n", sourcePath)
		return convertToALAC(sourcePath, targetPath, audioInfo)
	}

	if sourceExt == ".mp3" {
		// Never convert MP3 to ALAC - just copy the original MP3
		fmt.Printf("Copying MP3: %s (MP3 files are not converted to lossless formats)\n", sourcePath)
		// Keep original extension for MP3
		originalTargetPath := strings.TrimSuffix(targetPath, ".m4a") + ".mp3"
		return copyFile(sourcePath, originalTargetPath)
	}

	return fmt.Errorf("unsupported source format for ALAC conversion: %s", sourceExt)
}

func getAudioInfo(filePath string) (*AudioInfo, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	if ext == ".m4a" {
		return getALACInfo(filePath)
	} else {
		return getFLACInfo(filePath)
	}
}

func getFLACInfo(filePath string) (*AudioInfo, error) {
	var cmd *exec.Cmd

	if config.UseDocker {
		dockerPath := getDockerPath(filePath)
		args := []string{"run", "--rm",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage, "--i", dockerPath}
		cmd = exec.Command("docker", args...)
	} else {
		cmd = exec.Command(config.SoxCommand, "--i", filePath)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	audioInfo, err := parseAudioInfo(string(output))
	if err != nil {
		return nil, err
	}

	audioInfo.Format = "flac"
	return audioInfo, nil
}

func getALACInfo(filePath string) (*AudioInfo, error) {
	var cmd *exec.Cmd

	if config.UseDocker {
		dockerPath := getDockerPath(filePath)
		args := []string{"run", "--rm", "--entrypoint", "ffprobe",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage,
			"-v", "quiet", "-show_entries", "stream=sample_rate,bits_per_raw_sample", "-of", "csv=p=0", dockerPath}
		cmd = exec.Command("docker", args...)
	} else {
		// Check if ffprobe is available
		if _, err := exec.LookPath("ffprobe"); err != nil {
			return nil, fmt.Errorf("ffprobe is not installed. Please install FFmpeg for ALAC support or use --use-docker option")
		}
		cmd = exec.Command("ffprobe", "-v", "quiet", "-show_entries", "stream=sample_rate,bits_per_raw_sample", "-of", "csv=p=0", filePath)
	}

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseALACInfo(string(output))
}

func parseALACInfo(info string) (*AudioInfo, error) {
	lines := strings.Split(strings.TrimSpace(info), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no audio stream information found")
	}

	// Look for the first line that contains valid audio stream data (both rate and bits)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue // Skip lines that don't have both values
		}

		rate, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue // Skip lines with invalid sample rate
		}

		bits, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue // Skip lines with invalid bit depth
		}

		// Skip streams that don't look like audio (rate should be reasonable)
		if rate < 8000 || rate > 500000 {
			continue
		}

		return &AudioInfo{
			Bits:   bits,
			Rate:   rate,
			Format: "alac",
		}, nil
	}

	return nil, fmt.Errorf("no valid audio stream information found")
}

func changeExtensionToFlac(filePath string) string {
	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + ".flac"
}

func changeExtensionToMP3(filePath string) string {
	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + ".mp3"
}

func changeExtensionToM4A(filePath string) string {
	ext := filepath.Ext(filePath)
	return strings.TrimSuffix(filePath, ext) + ".m4a"
}

func convertToMP3(sourcePath, targetPath string, audioInfo *AudioInfo) error {
	// MP3 conversion: Use SoX to convert audio, then FFmpeg to preserve metadata
	var tempPath string

	if !config.NoPreserveMetadata {
		// Create temporary path for conversion output with proper extension
		ext := filepath.Ext(targetPath)
		tempPath = strings.TrimSuffix(targetPath, ext) + ".tmp" + ext
	} else {
		tempPath = targetPath
	}

	// Determine appropriate sample rate for MP3
	targetSampleRate := "44100"
	if audioInfo != nil {
		switch audioInfo.Rate {
		case 48000, 96000, 192000, 384000:
			targetSampleRate = "48000"
		default:
			targetSampleRate = "44100"
		}
	}

	var cmd *exec.Cmd

	if config.UseDocker {
		dockerSourcePath := getDockerPath(sourcePath)
		dockerTempPath := getDockerTargetPath(tempPath)
		args := []string{"run", "--rm",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage, dockerSourcePath, "-t", "mp3", "-C", "320", "-r", targetSampleRate, dockerTempPath}
		cmd = exec.Command("docker", args...)
	} else {
		cmd = exec.Command(config.SoxCommand, sourcePath, "-t", "mp3", "-C", "320", "-r", targetSampleRate, tempPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("conversion to MP3 failed: %w", err)
	}

	if !config.NoPreserveMetadata {
		// Merge metadata using FFmpeg
		if mergeErr := mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath); mergeErr != nil {
			fmt.Printf("Warning: Metadata preservation failed for %s, keeping converted audio without tags: %v\n", targetPath, mergeErr)
			// Fallback: rename temp to target
			if renameErr := os.Rename(tempPath, targetPath); renameErr != nil {
				return fmt.Errorf("fallback rename failed after metadata merge error: %w", renameErr)
			}
			return nil
		}
		// If merge succeeded, temp is already removed in merge function
	} else {
		// If not preserving metadata, tempPath == targetPath, no action needed
	}

	return nil
}

func convertToALAC(sourcePath, targetPath string, audioInfo *AudioInfo) error {
	// ALAC conversion:
	// To preserve the best quality and metadata:
	// First Use SoX to process and downsample audio to a temp FLAC, since sox can do this better
	// Then since SoX can't encode to ALAC, use FFmpeg to convert to ALAC and preserve metadata

	var tempPath string

	if !config.NoPreserveMetadata {
		// Create temporary path for conversion output with proper extension
		ext := filepath.Ext(targetPath)
		tempPath = strings.TrimSuffix(targetPath, ext) + ".tmp" + ext
	} else {
		tempPath = targetPath
	}

	// Step 1: Use SoX to convert source to intermediate FLAC with proper bit depth/sample rate
	tempFlacPath := strings.TrimSuffix(tempPath, ".m4a") + ".temp.flac"

	// Determine if we need SoX processing for bit depth/sample rate conversion
	needsConversion := false
	var bitrateArgs []string
	sampleRateArgs := []string{"rate", "-v", "-L"}

	if audioInfo != nil {
		// Check bit depth
		if audioInfo.Bits > 16 {
			needsConversion = true
			bitrateArgs = []string{"-b", "16"}
		}

		// Check sample rate
		switch audioInfo.Rate {
		case 96000, 192000, 384000:
			needsConversion = true
			sampleRateArgs = append(sampleRateArgs, "48000")
		case 88200, 176400, 352800:
			needsConversion = true
			sampleRateArgs = append(sampleRateArgs, "44100")
		}
	}

	var cmd *exec.Cmd

	if needsConversion {
		// Use SoX for quality conversion to FLAC first
		if config.UseDocker {
			dockerSource := getDockerPath(sourcePath)
			dockerTempFlac := getDockerTargetPath(tempFlacPath)

			args := []string{"run", "--rm",
				"-v", fmt.Sprintf("%s:/source", config.SourceDir),
				"-v", fmt.Sprintf("%s:/target", config.TargetDir),
				config.DockerImage, "--multi-threaded", "-G", dockerSource}

			args = append(args, bitrateArgs...)
			args = append(args, dockerTempFlac)
			args = append(args, sampleRateArgs...)
			args = append(args, "dither")

			cmd = exec.Command("docker", args...)
		} else {
			args := []string{"--multi-threaded", "-G", sourcePath}
			args = append(args, bitrateArgs...)
			args = append(args, tempFlacPath)
			args = append(args, sampleRateArgs...)
			args = append(args, "dither")

			cmd = exec.Command(config.SoxCommand, args...)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("SoX conversion to FLAC failed: %w", err)
		}
	} else {
		// Direct conversion to FLAC without quality changes
		if config.UseDocker {
			dockerSource := getDockerPath(sourcePath)
			dockerTempFlac := getDockerTargetPath(tempFlacPath)

			args := []string{"run", "--rm",
				"-v", fmt.Sprintf("%s:/source", config.SourceDir),
				"-v", fmt.Sprintf("%s:/target", config.TargetDir),
				config.DockerImage, dockerSource, dockerTempFlac}

			cmd = exec.Command("docker", args...)
		} else {
			cmd = exec.Command(config.SoxCommand, sourcePath, tempFlacPath)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("SoX conversion to FLAC failed: %w", err)
		}
	}

	// Step 2: Convert FLAC to ALAC using FFmpeg
	if config.UseDocker {
		dockerTempFlac := getDockerTargetPath(tempFlacPath)
		dockerTemp := getDockerTargetPath(tempPath)

		args := []string{"run", "--rm", "--entrypoint", "ffmpeg",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage,
			"-y", "-i", dockerTempFlac, "-c:a", "alac", "-sample_fmt", "s16p", dockerTemp}

		cmd = exec.Command("docker", args...)
	} else {
		cmd = exec.Command("ffmpeg", "-y", "-i", tempFlacPath, "-c:a", "alac", "-sample_fmt", "s16p", tempPath)
	}

	if err := cmd.Run(); err != nil {
		os.Remove(tempFlacPath) // Clean up temp FLAC file
		return fmt.Errorf("FFmpeg FLAC to ALAC conversion failed: %w", err)
	}

	// Clean up temp FLAC file
	os.Remove(tempFlacPath)

	if !config.NoPreserveMetadata {
		// Merge metadata using FFmpeg
		if mergeErr := mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath); mergeErr != nil {
			fmt.Printf("Warning: Metadata preservation failed for %s, keeping converted audio without tags: %v\n", targetPath, mergeErr)
			// Fallback: rename temp to target
			if renameErr := os.Rename(tempPath, targetPath); renameErr != nil {
				return fmt.Errorf("fallback rename failed after metadata merge error: %w", renameErr)
			}
			return nil
		}
		// If merge succeeded, temp is already removed in merge function
	} else {
		// If not preserving metadata, tempPath == targetPath, no action needed
	}

	return nil
}

func processAudioFile(sourcePath, targetPath string, audioInfo *AudioInfo, needsConversion bool, bitrateArgs, sampleRateArgs []string) error {
	if audioInfo.Format == "alac" {
		return processALAC(sourcePath, targetPath, needsConversion, bitrateArgs, sampleRateArgs)
	} else {
		return processFlac(sourcePath, targetPath, needsConversion, bitrateArgs, sampleRateArgs)
	}
}

func processALAC(sourcePath, targetPath string, needsConversion bool, bitrateArgs, sampleRateArgs []string) error {
	var tempPath string

	if !config.NoPreserveMetadata {
		// Create temporary path for conversion output with proper extension
		ext := filepath.Ext(targetPath)
		tempPath = strings.TrimSuffix(targetPath, ext) + ".tmp" + ext
	} else {
		tempPath = targetPath
	}

	// Convert ALAC to FLAC using FFmpeg, with optional quality adjustments via SoX
	var cmd *exec.Cmd

	if needsConversion {
		// Two-step process: ALAC -> temp FLAC via FFmpeg, then temp FLAC -> final FLAC via SoX
		tempAlacFlac := strings.TrimSuffix(tempPath, ".flac") + ".alac_temp.flac"

		// Step 1: Convert ALAC to FLAC using FFmpeg
		if config.UseDocker {
			dockerSource := getDockerPath(sourcePath)
			dockerTempAlac := getDockerTargetPath(tempAlacFlac)

			args := []string{"run", "--rm", "--entrypoint", "ffmpeg",
				"-v", fmt.Sprintf("%s:/source", config.SourceDir),
				"-v", fmt.Sprintf("%s:/target", config.TargetDir),
				config.DockerImage,
				"-i", dockerSource,
				"-c:a", "flac",
				dockerTempAlac}

			cmd = exec.Command("docker", args...)
		} else {
			// Check if ffmpeg is available
			if _, err := exec.LookPath("ffmpeg"); err != nil {
				return fmt.Errorf("ffmpeg is not installed. Please install FFmpeg for ALAC support or use --use-docker option")
			}
			cmd = exec.Command("ffmpeg", "-i", sourcePath, "-c:a", "flac", tempAlacFlac)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("FFmpeg ALAC to FLAC conversion failed: %w", err)
		}
		defer os.Remove(tempAlacFlac) // Clean up intermediate file

		// Step 2: Process with SoX for quality adjustment
		if config.UseDocker {
			dockerTempAlac := getDockerTargetPath(tempAlacFlac)
			dockerTemp := getDockerTargetPath(tempPath)

			args := []string{"run", "--rm",
				"-v", fmt.Sprintf("%s:/source", config.SourceDir),
				"-v", fmt.Sprintf("%s:/target", config.TargetDir),
				config.DockerImage, "--multi-threaded", "-G", dockerTempAlac}

			args = append(args, bitrateArgs...)
			args = append(args, dockerTemp)
			args = append(args, sampleRateArgs...)
			args = append(args, "dither")

			cmd = exec.Command("docker", args...)
		} else {
			args := []string{"--multi-threaded", "-G", tempAlacFlac}
			args = append(args, bitrateArgs...)
			args = append(args, tempPath)
			args = append(args, sampleRateArgs...)
			args = append(args, "dither")

			cmd = exec.Command(config.SoxCommand, args...)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("SoX quality adjustment failed: %w", err)
		}
	} else {
		// Direct conversion ALAC to FLAC without quality changes
		if config.UseDocker {
			dockerSource := getDockerPath(sourcePath)
			dockerTemp := getDockerTargetPath(tempPath)

			args := []string{"run", "--rm", "--entrypoint", "ffmpeg",
				"-v", fmt.Sprintf("%s:/source", config.SourceDir),
				"-v", fmt.Sprintf("%s:/target", config.TargetDir),
				config.DockerImage,
				"-i", dockerSource,
				"-c:a", "flac",
				dockerTemp}

			cmd = exec.Command("docker", args...)
		} else {
			// Check if ffmpeg is available
			if _, err := exec.LookPath("ffmpeg"); err != nil {
				return fmt.Errorf("ffmpeg is not installed. Please install FFmpeg for ALAC support or use --use-docker option")
			}
			cmd = exec.Command("ffmpeg", "-i", sourcePath, "-c:a", "flac", tempPath)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("FFmpeg ALAC to FLAC conversion failed: %w", err)
		}
	}

	if !config.NoPreserveMetadata {
		// Merge metadata using FFmpeg
		if mergeErr := mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath); mergeErr != nil {
			fmt.Printf("Warning: Metadata preservation failed for %s, keeping converted audio without tags: %v\n", targetPath, mergeErr)
			// Fallback: rename temp to target
			if renameErr := os.Rename(tempPath, targetPath); renameErr != nil {
				return fmt.Errorf("fallback rename failed after metadata merge error: %w", renameErr)
			}
			return nil
		}
		// If merge succeeded, temp is already removed in merge function
	} else {
		// If not preserving metadata, tempPath == targetPath, no action needed
	}

	return nil
}

func parseAudioInfo(info string) (*AudioInfo, error) {
	audioInfo := &AudioInfo{}
	scanner := bufio.NewScanner(strings.NewReader(info))

	bitsRegex := regexp.MustCompile(`Sample Encoding.*?(\d+)-bit`)
	rateRegex := regexp.MustCompile(`Sample Rate\s*:\s*(\d+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := bitsRegex.FindStringSubmatch(line); len(matches) > 1 {
			if bits, err := strconv.Atoi(matches[1]); err == nil {
				audioInfo.Bits = bits
			}
		}

		if matches := rateRegex.FindStringSubmatch(line); len(matches) > 1 {
			if rate, err := strconv.Atoi(matches[1]); err == nil {
				audioInfo.Rate = rate
			}
		}
	}

	return audioInfo, nil
}

func determineConversion(info *AudioInfo) (bool, []string, []string) {
	needsConversion := false
	var bitrateArgs []string
	sampleRateArgs := []string{"rate", "-v", "-L"}

	// Check bit depth
	if info.Bits > 16 {
		needsConversion = true
		bitrateArgs = []string{"-b", "16"}
	}

	// Check sample rate
	switch info.Rate {
	case 96000, 192000, 384000:
		needsConversion = true
		sampleRateArgs = append(sampleRateArgs, "48000")
	case 88200, 176400, 352800:
		needsConversion = true
		sampleRateArgs = append(sampleRateArgs, "44100")
	}

	return needsConversion, bitrateArgs, sampleRateArgs
}

func processFlac(sourcePath, targetPath string, needsConversion bool, bitrateArgs, sampleRateArgs []string) error {
	if !needsConversion {
		return copyFile(sourcePath, targetPath)
	}

	var tempPath string

	if !config.NoPreserveMetadata {
		// Create temporary path for SoX output with proper extension
		ext := filepath.Ext(targetPath)
		tempPath = strings.TrimSuffix(targetPath, ext) + ".tmp" + ext
	} else {
		tempPath = targetPath
	}

	// Run SoX conversion to tempPath or targetPath
	var cmd *exec.Cmd

	if config.UseDocker {
		dockerSource := getDockerPath(sourcePath)
		dockerTemp := getDockerTargetPath(tempPath)

		args := []string{"run", "--rm",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage, "--multi-threaded", "-G", dockerSource}

		args = append(args, bitrateArgs...)
		args = append(args, dockerTemp)
		args = append(args, sampleRateArgs...)
		args = append(args, "dither")

		cmd = exec.Command("docker", args...)
	} else {
		args := []string{"--multi-threaded", "-G", sourcePath}
		args = append(args, bitrateArgs...)
		args = append(args, tempPath)
		args = append(args, sampleRateArgs...)
		args = append(args, "dither")

		cmd = exec.Command(config.SoxCommand, args...)
	}

	if err := cmd.Run(); err != nil {
		if !config.NoPreserveMetadata && tempPath != "" {
			defer os.Remove(tempPath) // Clean up temp on error
		}
		return fmt.Errorf("SoX conversion failed: %w", err)
	}

	if !config.NoPreserveMetadata {
		// Merge metadata using FFmpeg
		if mergeErr := mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath); mergeErr != nil {
			fmt.Printf("Warning: Metadata preservation failed for %s, keeping converted audio without tags: %v\n", targetPath, mergeErr)
			// Fallback: rename temp to target
			if renameErr := os.Rename(tempPath, targetPath); renameErr != nil {
				return fmt.Errorf("fallback rename failed after metadata merge error: %w", renameErr)
			}
			return nil
		}
		// If merge succeeded, temp is already removed in merge function
	} else {
		// If not preserving metadata, tempPath == targetPath, no action needed
	}

	return nil
}

func getDockerPath(hostPath string) string {
	relPath := normalizeForDocker(config.SourceDir, hostPath)
	return "/source/" + relPath
}

func getDockerTargetPath(hostPath string) string {
	relPath := normalizeForDocker(config.TargetDir, hostPath)
	return "/target/" + relPath
}

func normalizeForDocker(base, path string) string {
	// Convert backslashes to forward slashes first
	base = strings.ReplaceAll(base, "\\", "/")
	path = strings.ReplaceAll(path, "\\", "/")

	// Try to use filepath.VolumeName
	volBase := filepath.VolumeName(base)
	baseStripped := base
	if volBase != "" {
		baseStripped = base[len(volBase):]
	} else {
		// Manual check for Windows drive letter (e.g., C:/ or c:/)
		if len(base) >= 3 && base[1] == ':' && base[2] == '/' {
			baseStripped = base[3:]
		}
	}

	volPath := filepath.VolumeName(path)
	pathStripped := path
	if volPath != "" {
		pathStripped = path[len(volPath):]
	} else {
		if len(path) >= 3 && path[1] == ':' && path[2] == '/' {
			pathStripped = path[3:]
		}
	}

	rel, err := filepath.Rel(baseStripped, pathStripped)
	if err != nil {
		return filepath.ToSlash(pathStripped)
	}
	return filepath.ToSlash(rel)
}
func mergeMetadataWithFFmpeg(sourcePath, tempConvertedPath, targetPath string) error {
	if config.NoPreserveMetadata {
		// If not preserving metadata, just rename temp to target
		return os.Rename(tempConvertedPath, targetPath)
	}

	var cmd *exec.Cmd

	if config.UseDocker {
		dockerSource := getDockerPath(sourcePath)
		dockerTemp := getDockerTargetPath(tempConvertedPath)
		dockerTarget := getDockerTargetPath(targetPath)

		args := []string{"run", "--rm", "--entrypoint", "ffmpeg",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage,
			"-i", dockerSource,
			"-i", dockerTemp,
			"-map", "1", // Map audio stream from the converted file (input 1)
			"-map", "0:v?", // Map video streams (cover art) from source file (input 0), ? makes it optional
			"-map_metadata", "0", // Map metadata from source file (input 0)
			"-c", "copy", // Copy streams without re-encoding
			dockerTarget}

		cmd = exec.Command("docker", args...)
	} else {
		// Local FFmpeg
		cmd = exec.Command("ffmpeg",
			"-i", sourcePath,
			"-i", tempConvertedPath,
			"-map", "1", // Map audio stream from the converted file (input 1)
			"-map", "0:v?", // Map video streams (cover art) from source file (input 0), ? makes it optional
			"-map_metadata", "0", // Map metadata from source file (input 0)
			"-c", "copy", // Copy streams without re-encoding
			targetPath)
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("FFmpeg metadata merge failed: %w", err)
	}

	// Remove temp file after successful merge
	if err := os.Remove(tempConvertedPath); err != nil {
		fmt.Printf("Warning: Failed to remove temp file %s: %v\n", tempConvertedPath, err)
	}

	return nil
}

func copyImageFiles() error {
	fmt.Println("Copying image files...")

	return filepath.Walk(config.SourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".jpg" && ext != ".png" {
			return nil
		}

		// Create target directory structure
		relPath, err := filepath.Rel(config.SourceDir, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(config.TargetDir, relPath)
		targetDir := filepath.Dir(targetPath)

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create target directory: %w", err)
		}

		return copyFile(path, targetPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	// Get source file info to preserve permissions and timestamps
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// Copy file content
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	// Ensure all writes are flushed to disk
	if err := destFile.Sync(); err != nil {
		return err
	}

	// Preserve file permissions
	if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
		return err
	}

	// Preserve file timestamps (access time and modification time)
	if err := os.Chtimes(dst, sourceInfo.ModTime(), sourceInfo.ModTime()); err != nil {
		return err
	}

	return nil
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
}

// compareVersions compares two semantic versions (v1 and v2) and returns:
// -1 if v1 < v2
// 0 if v1 == v2
// 1 if v1 > v2
// Assumes versions are like "v1.2.3" or "1.2.3", ignores 'v' prefix
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	// Pad to 3 parts for major.minor.patch
	for len(parts1) < 3 {
		parts1 = append(parts1, "0")
	}
	for len(parts2) < 3 {
		parts2 = append(parts2, "0")
	}

	for i := 0; i < 3; i++ {
		p1, _ := strconv.Atoi(parts1[i])
		p2, _ := strconv.Atoi(parts2[i])
		if p1 < p2 {
			return -1
		} else if p1 > p2 {
			return 1
		}
	}
	return 0
}

func selfUpdate(client *http.Client) error {
	currentVersion := version
	if currentVersion == "dev" {
		fmt.Println("Development version detected. Skipping update check.")
		return nil
	}

	fmt.Printf("Current version: %s\n", currentVersion)

	// Fetch latest release from GitHub API
	apiURL := "https://api.github.com/repos/Ardakilic/lilt/releases/latest"
	fmt.Printf("Checking for updates from: %s\n", apiURL)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		fmt.Printf("Failed to create request for %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Failed to check for updates from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			fmt.Printf("Failed to fetch release info from %s: HTTP %d (Forbidden)\n", apiURL, resp.StatusCode)
			fmt.Println("This may be due to GitHub API rate limiting. Please wait a few minutes and try again, or visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		} else {
			fmt.Printf("Failed to fetch release info from %s: HTTP %d\n", apiURL, resp.StatusCode)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		}
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		return nil
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		fmt.Printf("Failed to parse release info from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
		return nil
	}

	latestVersion := release.TagName
	fmt.Printf("Latest version: %s\n", latestVersion)

	cmp := compareVersions(currentVersion, latestVersion)
	if cmp < 0 {
		fmt.Printf("New version %s available. Updating...\n", latestVersion)

		// Platform detection
		goos := runtime.GOOS
		goarch := runtime.GOARCH

		// Construct asset filename
		var filename string
		if goos == "windows" {
			filename = fmt.Sprintf("lilt-%s-%s.exe.zip", goos, goarch)
		} else {
			filename = fmt.Sprintf("lilt-%s-%s.tar.gz", goos, goarch)
		}

		assetURL := fmt.Sprintf("https://github.com/Ardakilic/lilt/releases/download/%s/%s", latestVersion, filename)
		fmt.Printf("Downloading from: %s\n", assetURL)

		// Download the asset
		fmt.Printf("Downloading update from: %s\n", assetURL)
		downloadReq, err := http.NewRequest("GET", assetURL, nil)
		if err != nil {
			fmt.Printf("Failed to create download request for %s: %v\n", assetURL, err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		downloadResp, err := client.Do(downloadReq)
		if err != nil {
			fmt.Printf("Failed to download update from %s: %v\n", assetURL, err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer downloadResp.Body.Close()

		if downloadResp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to download update from %s: HTTP %d\n", assetURL, downloadResp.StatusCode)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		// Create temp file for download
		tempFile, err := os.CreateTemp("", "lilt-update-*")
		if err != nil {
			fmt.Printf("Failed to create temp file: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer os.Remove(tempFile.Name()) // Clean up if error

		_, err = io.Copy(tempFile, downloadResp.Body)
		if err != nil {
			fmt.Printf("Failed to download update: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		tempFile.Close()

		// Create temp dir for extraction
		tempDir, err := os.MkdirTemp("", "lilt-extract-*")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer os.RemoveAll(tempDir) // Clean up if error

		// Extract
		if goos == "windows" {
			// Extract zip
			r, err := zip.OpenReader(tempFile.Name())
			if err != nil {
				fmt.Printf("Failed to open zip: %v\n", err)
				fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
				return nil
			}
			defer r.Close()

			for _, f := range r.File {
				if f.Name == strings.TrimSuffix(filename, ".zip") { // Remove .zip
					rc, err := f.Open()
					if err != nil {
						fmt.Printf("Warning: Failed to open file %s in zip: %v\n", f.Name, err)
						continue
					}
					outFile, err := os.Create(filepath.Join(tempDir, f.Name))
					if err != nil {
						fmt.Printf("Warning: Failed to create output file %s: %v\n", f.Name, err)
						rc.Close()
						continue
					}
					if _, err = io.Copy(outFile, rc); err != nil {
						fmt.Printf("Warning: Failed to copy file %s: %v\n", f.Name, err)
						outFile.Close()
						rc.Close()
						continue
					}
					outFile.Close()
					rc.Close()
					break
				}
			}
		} else {
			// Extract tar.gz
			file, err := os.Open(tempFile.Name())
			if err != nil {
				fmt.Printf("Failed to open tar.gz: %v\n", err)
				fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
				return nil
			}
			defer file.Close()

			gzr, err := gzip.NewReader(file)
			if err != nil {
				fmt.Printf("Failed to read gzip: %v\n", err)
				fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
				return nil
			}
			defer gzr.Close()

			tr := tar.NewReader(gzr)
			for {
				header, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					fmt.Printf("Failed to extract tar: %v\n", err)
					fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
					return nil
				}
				if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "lilt-"+goos+"-"+goarch {
					outFile, err := os.Create(filepath.Join(tempDir, header.Name))
					if err != nil {
						fmt.Printf("Warning: Failed to create output file %s: %v\n", header.Name, err)
						continue
					}
					if _, err = io.Copy(outFile, tr); err != nil {
						fmt.Printf("Warning: Failed to copy file %s: %v\n", header.Name, err)
						outFile.Close()
						continue
					}
					outFile.Close()
					break
				}
			}
		}

		// Find the extracted binary
		binaryName := "lilt-" + goos + "-" + goarch
		if goos == "windows" {
			binaryName += ".exe"
		}
		newBinaryPath := filepath.Join(tempDir, binaryName)
		if _, err := os.Stat(newBinaryPath); os.IsNotExist(err) {
			fmt.Printf("Failed to extract binary: %s not found\n", binaryName)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		// Replacement
		currentPath, err := os.Executable()
		if err != nil {
			fmt.Printf("Failed to get current executable path: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		backupPath := currentPath + ".old"
		if err := os.Rename(currentPath, backupPath); err != nil {
			fmt.Printf("Failed to backup current binary: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		if err := os.Rename(newBinaryPath, currentPath); err != nil {
			// Restore backup
			os.Rename(backupPath, currentPath)
			fmt.Printf("Failed to replace binary: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/lilt to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		// Make executable
		if err := os.Chmod(currentPath, 0755); err != nil {
			fmt.Printf("Warning: Failed to set permissions on new binary: %v\n", err)
		}

		fmt.Println("Update complete. Please restart the application.")
		return nil
	} else if cmp == 0 {
		fmt.Println("You are running the latest version.")
	} else {
		fmt.Printf("You are running a newer version %s than the latest release %s.\n", currentVersion, latestVersion)
	}

	return nil
}
