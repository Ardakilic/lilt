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
	config         Config
	version        = "dev" // This will be set during build time
	selfUpdateFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "flac-converter <source_directory>",
	Short: "Convert Hi-Res FLAC files to 16-bit FLAC files",
	Long: `FLAC to 16-bit Converter

This tool converts Hi-Res FLAC files to 16-bit FLAC files with a sample rate of 44.1kHz or 48kHz.
It also copies MP3 files and image files (JPG, PNG) to the target directory.

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
		return selfUpdate()
	}

	if len(args) == 0 {
		return fmt.Errorf("source directory required")
	}

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

func selfUpdate() error {
	currentVersion := version
	if currentVersion == "dev" {
		fmt.Println("Development version detected. Skipping update check.")
		return nil
	}

	fmt.Printf("Current version: %s\n", currentVersion)

	// Fetch latest release from GitHub API
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	fmt.Printf("Checking for updates from: %s\n", apiURL)

	resp, err := http.Get(apiURL)
	if err != nil {
		fmt.Printf("Failed to check for updates from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusForbidden {
			fmt.Printf("Failed to fetch release info from %s: HTTP %d (Forbidden)\n", apiURL, resp.StatusCode)
			fmt.Println("This may be due to GitHub API rate limiting. Please wait a few minutes and try again, or visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
		} else {
			fmt.Printf("Failed to fetch release info from %s: HTTP %d\n", apiURL, resp.StatusCode)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
		}
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Failed to read response from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
		return nil
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		fmt.Printf("Failed to parse release info from %s: %v\n", apiURL, err)
		fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
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
			filename = fmt.Sprintf("flac-converter-%s-%s.exe.zip", goos, goarch)
		} else {
			filename = fmt.Sprintf("flac-converter-%s-%s.tar.gz", goos, goarch)
		}

		assetURL := fmt.Sprintf("https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/%s/%s", latestVersion, filename)
		fmt.Printf("Downloading from: %s\n", assetURL)

		// Download the asset
		fmt.Printf("Downloading update from: %s\n", assetURL)
		downloadResp, err := http.Get(assetURL)
		if err != nil {
			fmt.Printf("Failed to download update from %s: %v\n", assetURL, err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer downloadResp.Body.Close()

		if downloadResp.StatusCode != http.StatusOK {
			fmt.Printf("Failed to download update from %s: HTTP %d\n", assetURL, downloadResp.StatusCode)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		// Create temp file for download
		tempFile, err := os.CreateTemp("", "flac-converter-update-*")
		if err != nil {
			fmt.Printf("Failed to create temp file: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer os.Remove(tempFile.Name()) // Clean up if error

		_, err = io.Copy(tempFile, downloadResp.Body)
		if err != nil {
			fmt.Printf("Failed to download update: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		tempFile.Close()

		// Create temp dir for extraction
		tempDir, err := os.MkdirTemp("", "flac-converter-extract-*")
		if err != nil {
			fmt.Printf("Failed to create temp dir: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}
		defer os.RemoveAll(tempDir) // Clean up if error

		// Extract
		if goos == "windows" {
			// Extract zip
			r, err := zip.OpenReader(tempFile.Name())
			if err != nil {
				fmt.Printf("Failed to open zip: %v\n", err)
				fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
				return nil
			}
			defer r.Close()

			for _, f := range r.File {
				if f.Name == filename[:len(filename)-4] { // Remove .zip
					rc, err := f.Open()
					if err != nil {
						continue
					}
					outFile, err := os.Create(filepath.Join(tempDir, f.Name))
					if err != nil {
						rc.Close()
						continue
					}
					_, err = io.Copy(outFile, rc)
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
				fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
				return nil
			}
			defer file.Close()

			gzr, err := gzip.NewReader(file)
			if err != nil {
				fmt.Printf("Failed to read gzip: %v\n", err)
				fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
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
					fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
					return nil
				}
				if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "flac-converter-"+goos+"-"+goarch {
					outFile, err := os.Create(filepath.Join(tempDir, header.Name))
					if err != nil {
						continue
					}
					_, err = io.Copy(outFile, tr)
					outFile.Close()
					break
				}
			}
		}

		// Find the extracted binary
		binaryName := "flac-converter-" + goos + "-" + goarch
		if goos == "windows" {
			binaryName += ".exe"
		}
		newBinaryPath := filepath.Join(tempDir, binaryName)
		if _, err := os.Stat(newBinaryPath); os.IsNotExist(err) {
			fmt.Printf("Failed to extract binary: %s not found\n", binaryName)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		// Replacement
		currentPath, err := os.Executable()
		if err != nil {
			fmt.Printf("Failed to get current executable path: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		backupPath := currentPath + ".old"
		if err := os.Rename(currentPath, backupPath); err != nil {
			fmt.Printf("Failed to backup current binary: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
			return nil
		}

		if err := os.Rename(newBinaryPath, currentPath); err != nil {
			// Restore backup
			os.Rename(backupPath, currentPath)
			fmt.Printf("Failed to replace binary: %v\n", err)
			fmt.Println("Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update.")
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
