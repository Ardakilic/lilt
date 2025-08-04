package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Config holds the application configuration
type Config struct {
	SourceDir   string
	TargetDir   string
	CopyImages  bool
	UseDocker   bool
	DockerImage string
	SoxCommand  string
}

// AudioInfo holds information about an audio file
type AudioInfo struct {
	Bits int
	Rate int
}

var (
	config  Config
	version = "dev" // This will be set during build time
)

var rootCmd = &cobra.Command{
	Use:   "flac-converter <source_directory>",
	Short: "Convert Hi-Res FLAC files to 16-bit FLAC files",
	Long: `FLAC to 16-bit Converter

This tool converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
It also copies MP3 files and image files (JPG, PNG) to the target directory.

Copyright (C) 2025 Arda Kilicdagi
Licensed under MIT License`,
	Args:    cobra.ExactArgs(1),
	RunE:    runConverter,
	Version: version,
}

func init() {
	rootCmd.Flags().StringVar(&config.TargetDir, "target-dir", "./transcoded", "Specify target directory")
	rootCmd.Flags().BoolVar(&config.CopyImages, "copy-images", false, "Copy JPG and PNG files")
	rootCmd.Flags().BoolVar(&config.UseDocker, "use-docker", false, "Use Docker to run Sox instead of local installation")
	rootCmd.Flags().StringVar(&config.DockerImage, "docker-image", "ardakilic/sox_ng:latest", "Specify Docker image")

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
	config.SourceDir = args[0]

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
	}
	return nil
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
		if ext != ".flac" && ext != ".mp3" {
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

		// Handle MP3 files - just copy them
		if ext == ".mp3" {
			fmt.Printf("Copying MP3 file: %s\n", path)
			return copyFile(path, targetPath)
		}

		// Process FLAC files
		audioInfo, err := getAudioInfo(path)
		if err != nil {
			fmt.Printf("Warning: Could not get audio info for %s, copying original\n", path)
			return copyFile(path, targetPath)
		}

		fmt.Printf("Detected: %d bits, %d Hz\n", audioInfo.Bits, audioInfo.Rate)

		needsConversion, bitrateArgs, sampleRateArgs := determineConversion(audioInfo)

		if needsConversion {
			fmt.Printf("Converting FLAC: %s\n", path)
			if err := convertFlac(path, targetPath, bitrateArgs, sampleRateArgs); err != nil {
				fmt.Printf("Error: Sox conversion failed. Copying original file instead. Error: %v\n", err)
				return copyFile(path, targetPath)
			}
		} else {
			fmt.Printf("Copying FLAC: %s\n", path)
			return copyFile(path, targetPath)
		}

		return nil
	})
}

func getAudioInfo(filePath string) (*AudioInfo, error) {
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

	return parseAudioInfo(string(output))
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
	case 88200:
		needsConversion = true
		sampleRateArgs = append(sampleRateArgs, "44100")
	}

	return needsConversion, bitrateArgs, sampleRateArgs
}

func convertFlac(sourcePath, targetPath string, bitrateArgs, sampleRateArgs []string) error {
	var cmd *exec.Cmd

	if config.UseDocker {
		dockerSource := getDockerPath(sourcePath)
		dockerTarget := getDockerTargetPath(targetPath)

		args := []string{"run", "--rm",
			"-v", fmt.Sprintf("%s:/source", config.SourceDir),
			"-v", fmt.Sprintf("%s:/target", config.TargetDir),
			config.DockerImage, "--multi-threaded", "-G", dockerSource}

		args = append(args, bitrateArgs...)
		args = append(args, dockerTarget)
		args = append(args, sampleRateArgs...)
		args = append(args, "dither")

		cmd = exec.Command("docker", args...)
	} else {
		args := []string{"--multi-threaded", "-G", sourcePath}
		args = append(args, bitrateArgs...)
		args = append(args, targetPath)
		args = append(args, sampleRateArgs...)
		args = append(args, "dither")

		cmd = exec.Command(config.SoxCommand, args...)
	}

	return cmd.Run()
}

func getDockerPath(hostPath string) string {
	relPath, _ := filepath.Rel(config.SourceDir, hostPath)
	return filepath.ToSlash(filepath.Join("/source", relPath))
}

func getDockerTargetPath(hostPath string) string {
	relPath, _ := filepath.Rel(config.TargetDir, hostPath)
	return filepath.ToSlash(filepath.Join("/target", relPath))
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

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}
