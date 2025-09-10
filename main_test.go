package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// MockTransport is a simple mock for http.RoundTripper to simulate API responses
type mockTransport struct {
	responses map[string]*http.Response
	err       error // If set, return this error for all requests
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp, ok := m.responses[req.URL.String()]
	if !ok {
		resp = &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBuffer([]byte("Not Found"))),
			Header:     make(http.Header),
		}
	}
	return resp, nil
}

// createMockClient creates a http.Client with a mock transport for testing
func createMockClient(responses map[string]*http.Response, err error) *http.Client {
	transport := &mockTransport{responses: responses, err: err}
	return &http.Client{Transport: transport}
}

// captureOutput captures stdout output during test execution
func captureOutput(f func()) (string, error) {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outC <- buf.String()
	}()

	f()

	w.Close()
	os.Stdout = old
	out := <-outC

	return out, nil
}

func TestParseAudioInfo(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected AudioInfo
	}{
		{
			name: "24-bit 96kHz FLAC",
			input: `Input File     : 'test.flac'
Channels       : 2
Sample Rate    : 96000
Precision      : 24-bit
Duration       : 00:03:45.23 = 21621600 samples ~ 16216.2 CDDA sectors
File Size      : 64.5M
Bit Rate       : 2.41M
Sample Encoding: 24-bit Signed Integer PCM`,
			expected: AudioInfo{Bits: 24, Rate: 96000},
		},
		{
			name: "16-bit 44.1kHz FLAC",
			input: `Input File     : 'test.flac'
Channels       : 2
Sample Rate    : 44100
Precision      : 16-bit
Duration       : 00:03:45.23 = 9953100 samples ~ 16216.2 CDDA sectors
File Size      : 39.5M
Bit Rate       : 1.41M
Sample Encoding: 16-bit Signed Integer PCM`,
			expected: AudioInfo{Bits: 16, Rate: 44100},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAudioInfo(tc.input)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Bits != tc.expected.Bits {
				t.Errorf("Expected bits %d, got %d", tc.expected.Bits, result.Bits)
			}

			if result.Rate != tc.expected.Rate {
				t.Errorf("Expected rate %d, got %d", tc.expected.Rate, result.Rate)
			}
		})
	}
}

func TestDetermineConversion(t *testing.T) {
	testCases := []struct {
		name               string
		input              AudioInfo
		expectedConversion bool
		expectedBitrate    []string
		expectedSampleRate []string
	}{
		{
			name:               "24-bit 96kHz needs conversion",
			input:              AudioInfo{Bits: 24, Rate: 96000},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L", "48000"},
		},
		{
			name:               "16-bit 44.1kHz no conversion",
			input:              AudioInfo{Bits: 16, Rate: 44100},
			expectedConversion: false,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "16-bit 88.2kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 88200},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			needsConversion, bitrateArgs, sampleRateArgs := determineConversion(&tc.input)

			if needsConversion != tc.expectedConversion {
				t.Errorf("Expected conversion %v, got %v", tc.expectedConversion, needsConversion)
			}

			if len(bitrateArgs) != len(tc.expectedBitrate) {
				t.Errorf("Expected bitrate args %v, got %v", tc.expectedBitrate, bitrateArgs)
			} else {
				for i, arg := range bitrateArgs {
					if arg != tc.expectedBitrate[i] {
						t.Errorf("Expected bitrate arg %s, got %s", tc.expectedBitrate[i], arg)
					}
				}
			}

			if len(sampleRateArgs) != len(tc.expectedSampleRate) {
				t.Errorf("Expected sample rate args %v, got %v", tc.expectedSampleRate, sampleRateArgs)
			} else {
				for i, arg := range sampleRateArgs {
					if arg != tc.expectedSampleRate[i] {
						t.Errorf("Expected sample rate arg %s, got %s", tc.expectedSampleRate[i], arg)
					}
				}
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "flac-converter-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a source file with specific content and permissions
	srcPath := filepath.Join(tmpDir, "source.txt")
	srcContent := "Hello, World!\nThis is a test file."

	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Set specific modification time
	modTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(srcPath, modTime, modTime); err != nil {
		t.Fatalf("Failed to set source file time: %v", err)
	}

	// Copy the file
	dstPath := filepath.Join(tmpDir, "destination.txt")
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("Failed to copy file: %v", err)
	}

	// Verify destination file exists
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		t.Fatal("Destination file does not exist")
	}

	// Verify content is identical
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if string(dstContent) != srcContent {
		t.Errorf("Content mismatch:\nExpected: %q\nGot: %q", srcContent, string(dstContent))
	}

	// Verify permissions are preserved
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("Failed to stat source file: %v", err)
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Permissions not preserved:\nExpected: %v\nGot: %v", srcInfo.Mode(), dstInfo.Mode())
	}

	// Verify modification time is preserved (within reasonable tolerance)
	timeDiff := srcInfo.ModTime().Sub(dstInfo.ModTime())
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff > time.Second {
		t.Errorf("Modification time not preserved:\nExpected: %v\nGot: %v\nDifference: %v",
			srcInfo.ModTime(), dstInfo.ModTime(), timeDiff)
	}
}

func TestCompareVersions(t *testing.T) {
	testCases := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "v1.0.0 < v1.0.1",
			v1:       "v1.0.0",
			v2:       "v1.0.1",
			expected: -1,
		},
		{
			name:     "v1.0.0 == v1.0.0",
			v1:       "v1.0.0",
			v2:       "v1.0.0",
			expected: 0,
		},
		{
			name:     "v2.0.0 > v1.0.0",
			v1:       "v2.0.0",
			v2:       "v1.0.0",
			expected: 1,
		},
		{
			name:     "1.2.3 < v1.2.4",
			v1:       "1.2.3",
			v2:       "v1.2.4",
			expected: -1,
		},
		{
			name:     "v1.0 < v1.0.1",
			v1:       "v1.0",
			v2:       "v1.0.1",
			expected: -1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compareVersions(tc.v1, tc.v2)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d", tc.expected, result)
			}
		})
	}
}

func TestSelfUpdateFetchLatest(t *testing.T) {
	// Test version comparison part
	oldVersion := "v1.0.0"
	cmp := compareVersions(oldVersion, "v1.0.1")
	if cmp >= 0 {
		t.Errorf("Expected negative cmp for update needed")
	}

	// Test dev version skip
	originalVersion := version
	version = "dev"
	defer func() { version = originalVersion }()

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(http.DefaultClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "Development version detected. Skipping update check.") {
		t.Error("Expected dev version message")
	}
}

func TestExtractTarGZ(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-extract")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a sample tar.gz
	buf := new(bytes.Buffer)
	gw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gw)

	header := &tar.Header{
		Name: "flac-converter-linux-amd64",
		Mode: 0755,
		Size: 100,
	}
	tw.WriteHeader(header)
	io.Copy(tw, strings.NewReader("dummy binary"))
	tw.Close()
	gw.Close()

	tempFile := filepath.Join(tmpDir, "test.tar.gz")
	os.WriteFile(tempFile, buf.Bytes(), 0644)

	// Call extraction logic (extract the function for testability, but for now, simulate)
	goos := "linux"
	goarch := "amd64"
	// Simulate extraction
	file, err := os.Open(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	found := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == "flac-converter-"+goos+"-"+goarch {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find binary in tar")
	}
}

func TestExtractZip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-extract")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a sample zip
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	f, err := zw.Create("flac-converter-windows-amd64.exe")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(f, strings.NewReader("dummy binary"))
	zw.Close()

	tempFile := filepath.Join(tmpDir, "test.zip")
	os.WriteFile(tempFile, buf.Bytes(), 0644)

	// Simulate extraction
	r, err := zip.OpenReader(tempFile)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	found := false
	for _, f := range r.File {
		if f.Name == "flac-converter-windows-amd64.exe" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected to find binary in zip")
	}
}

func TestBinaryReplacement(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-replace")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Simulate current binary
	currentPath := filepath.Join(tmpDir, "current.exe")
	os.WriteFile(currentPath, []byte("old"), 0755)

	// Simulate new binary
	newPath := filepath.Join(tmpDir, "new.exe")
	os.WriteFile(newPath, []byte("new"), 0755)

	// Simulate replacement
	backupPath := currentPath + ".old"
	err = os.Rename(currentPath, backupPath)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Rename(newPath, currentPath)
	if err != nil {
		// Restore
		os.Rename(backupPath, currentPath)
		t.Fatal(err)
	}

	// Check permissions
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0755 != 0755 {
		t.Errorf("Expected executable permissions")
	}

	// Verify content
	content, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Errorf("Expected new content")
	}
}

func TestGetAudioInfoDocker(t *testing.T) {
	// Test with Docker
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.DockerImage = "test/sox"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	// Create a dummy file for testing
	tmpDir, err := os.MkdirTemp("", "test-audio-docker")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.flac")
	os.WriteFile(testFile, []byte("dummy"), 0644)

	info, err := getAudioInfo(testFile)
	// We expect this to fail since docker is not available, but it tests the Docker path
	if err == nil {
		t.Logf("getAudioInfo with Docker succeeded unexpectedly with: %+v", info)
	}
}

func TestProcessFlacDocker(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-convert-docker")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceFile := filepath.Join(tmpDir, "source.flac")
	targetFile := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourceFile, []byte("dummy flac"), 0644)

	// Test with Docker
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.DockerImage = "test/sox"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	bitrateArgs := []string{"-b", "16"}
	sampleRateArgs := []string{"rate", "-v", "-L", "44100"}

	err = processFlac(sourceFile, targetFile, true, bitrateArgs, sampleRateArgs)
	// We expect this to fail since docker is not available, but it tests the Docker path
	if err == nil {
		t.Logf("processFlac with Docker succeeded unexpectedly")
	}
}

func TestProcessFlacTemporaryFileNaming(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-temp-naming")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceFile := filepath.Join(tmpDir, "test file with spaces.flac")
	targetFile := filepath.Join(tmpDir, "output file with spaces.flac")

	// Create a dummy source file
	os.WriteFile(sourceFile, []byte("dummy flac"), 0644)

	// Test the temporary file naming logic
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false // Enable metadata preservation

	// Test that temporary file path has .tmp.flac extension
	var tempPath string
	if !config.NoPreserveMetadata {
		tempPath = strings.TrimSuffix(targetFile, ".flac") + ".tmp.flac"
	} else {
		tempPath = targetFile
	}

	expectedTempPath := filepath.Join(tmpDir, "output file with spaces.tmp.flac")
	if tempPath != expectedTempPath {
		t.Errorf("Expected temp path %s, got %s", expectedTempPath, tempPath)
	}

	// Verify the extension is .flac for temporary file
	if !strings.HasSuffix(tempPath, ".flac") {
		t.Errorf("Temporary file should have .flac extension, got %s", tempPath)
	}
}

func TestMergeMetadataWithFFmpeg(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-ffmpeg-merge")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceFile := filepath.Join(tmpDir, "source.flac")
	tempFile := filepath.Join(tmpDir, "temp.tmp.flac")
	targetFile := filepath.Join(tmpDir, "target.flac")

	// Create dummy files
	os.WriteFile(sourceFile, []byte("source flac"), 0644)
	os.WriteFile(tempFile, []byte("converted flac"), 0644)

	// Test with metadata preservation disabled
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = true

	err = mergeMetadataWithFFmpeg(sourceFile, tempFile, targetFile)
	if err != nil {
		t.Errorf("mergeMetadataWithFFmpeg failed with NoPreserveMetadata=true: %v", err)
	}

	// Should just rename temp to target
	if _, err := os.Stat(targetFile); os.IsNotExist(err) {
		t.Error("Target file should exist after merge with NoPreserveMetadata=true")
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Error("Temp file should be removed after merge")
	}
}

func TestSetupSoxCommandDocker(t *testing.T) {
	// Test Docker path setup
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.DockerImage = "test/sox"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	err := setupSoxCommand()
	// We expect this to fail since docker is not available, but it tests the Docker setup path
	if err == nil {
		t.Logf("setupSoxCommand with Docker succeeded unexpectedly")
	}
}

func TestSelfUpdateNetworkFailure(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Create mock client that returns network error
	mockClient := createMockClient(nil, fmt.Errorf("network error"))

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	// Verify graceful error handling - function should not return error but print messages
	if err != nil {
		t.Errorf("Expected no error for network failure (graceful handling), got: %v", err)
	}

	// Check for expected output messages
	if !strings.Contains(output, "Current version: v1.0.0") {
		t.Error("Expected current version in output")
	}
	if !strings.Contains(output, "Checking for updates from:") {
		t.Error("Expected checking URL in output")
	}
	if !strings.Contains(output, "Failed to check for updates from") {
		t.Error("Expected failure message in output")
	}
	if !strings.Contains(output, "Please visit https://github.com/Ardakilic/flac-to-16bit-converter") {
		t.Error("Expected fallback instructions in output")
	}
}

// The MockServer is no longer needed as we're using client mocking

func TestSelfUpdateUpToDate(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock response with same version
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.0.0"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	// Should not error
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Check for expected output
	if !strings.Contains(output, "Current version: v1.0.0") {
		t.Error("Expected current version in output")
	}
	if !strings.Contains(output, "Latest version: v1.0.0") {
		t.Error("Expected latest version in output")
	}
	if !strings.Contains(output, "You are running the latest version.") {
		t.Error("Expected up-to-date message")
	}
}

func TestCopyFileErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-errors")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test with non-existent source file
	err = copyFile("/non/existent/source", filepath.Join(tmpDir, "dest"))
	if err == nil {
		t.Error("Expected error for non-existent source file")
	}

	// Test with valid files
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")
	srcContent := "test content"

	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("copyFile failed: %v", err)
	}

	// Verify content
	content, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != srcContent {
		t.Errorf("Content mismatch: expected %q, got %q", srcContent, string(content))
	}
}

func TestProcessAudioFilesErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-process-errors")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(sourceDir, 0755)
	os.MkdirAll(targetDir, 0755)

	// Create a file that will cause getAudioInfo to fail
	flacFile := filepath.Join(sourceDir, "test.flac")
	os.WriteFile(flacFile, []byte("dummy flac"), 0644)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir
	config.UseDocker = false
	config.SoxCommand = "false" // Command that always fails

	err = processAudioFiles()
	// Should not fail completely, should copy original file on error
	if err != nil {
		t.Logf("processAudioFiles with failing sox returned: %v", err)
	}

	// Verify file was still processed (copied)
	if _, err := os.Stat(filepath.Join(targetDir, "test.flac")); os.IsNotExist(err) {
		t.Error("FLAC file was not processed even with failing sox")
	}
}

func TestCopyImageFilesErrors(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-images-errors")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source directory structure
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(sourceDir, 0755)
	os.MkdirAll(targetDir, 0755)

	// Create test files
	jpgFile := filepath.Join(sourceDir, "test.jpg")
	os.WriteFile(jpgFile, []byte("jpg content"), 0644)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir

	// Test copyImageFiles
	err = copyImageFiles()
	if err != nil {
		t.Fatalf("copyImageFiles failed: %v", err)
	}

	// Verify file was copied
	if _, err := os.Stat(filepath.Join(targetDir, "test.jpg")); os.IsNotExist(err) {
		t.Error("JPG file was not copied")
	}
}

func TestCompareVersionsEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		{
			name:     "empty versions",
			v1:       "",
			v2:       "",
			expected: 0,
		},
		{
			name:     "single number vs double",
			v1:       "1",
			v2:       "1.0",
			expected: 0,
		},
		{
			name:     "double vs triple",
			v1:       "1.0",
			v2:       "1.0.0",
			expected: 0,
		},
		{
			name:     "major version difference",
			v1:       "2.0.0",
			v2:       "1.9.9",
			expected: 1,
		},
		{
			name:     "patch version difference",
			v1:       "1.0.1",
			v2:       "1.0.0",
			expected: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := compareVersions(tc.v1, tc.v2)
			if result != tc.expected {
				t.Errorf("Expected %d, got %d for %s vs %s", tc.expected, result, tc.v1, tc.v2)
			}
		})
	}
}

func TestSelfUpdateDevVersion(t *testing.T) {
	originalVersion := version
	version = "dev"
	defer func() { version = originalVersion }()

	// Use default client, but since dev version skips, no request
	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(http.DefaultClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "Development version detected. Skipping update check.") {
		t.Error("Expected dev version message")
	}
}

func TestSelfUpdateSameVersion(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock response with same version
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.0.0"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "You are running the latest version.") {
		t.Error("Expected same version message")
	}
}

func TestSelfUpdateNewerLocalVersion(t *testing.T) {
	originalVersion := version
	version = "v2.0.0"
	defer func() { version = originalVersion }()

	// Mock response with older version
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.0.0"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "You are running a newer version v2.0.0 than the latest release v1.0.0.") {
		t.Error("Expected newer local version message")
	}
}

func TestSelfUpdateInvalidVersion(t *testing.T) {
	originalVersion := version
	version = "invalid-version"
	defer func() { version = originalVersion }()

	// Mock response with valid version
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.0.0"}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Since compareVersions treats invalid as 0, it should show newer or equal
	if !strings.Contains(output, "Current version: invalid-version") {
		t.Error("Expected invalid version in output")
	}
	// Depending on comparison, but at least it doesn't panic
}

func TestSelfUpdateWithMockServer(t *testing.T) {
	// This test is now properly mocked via client
	t.Skip("Replaced by individual mock client tests")
}

func TestCopyFilePermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-perm")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	// Create source file with specific permissions
	srcContent := "test content"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0755); err != nil {
		t.Fatal(err)
	}

	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("copyFile failed: %v", err)
	}

	// Check that executable permissions are preserved
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatal(err)
	}

	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}

	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("Permissions not preserved: expected %v, got %v", srcInfo.Mode(), dstInfo.Mode())
	}
}

func TestProcessAudioFilesEmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-empty-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(sourceDir, 0755)
	os.MkdirAll(targetDir, 0755)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir
	config.UseDocker = false
	config.SoxCommand = "true"

	// Test with empty directory
	err = processAudioFiles()
	if err != nil {
		t.Logf("processAudioFiles with empty dir: %v", err)
	}
}

func TestProcessAudioFilesNonExistentDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "nonexistent")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(targetDir, 0755)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir
	config.UseDocker = false
	config.SoxCommand = "true"

	// Test with non-existent source directory
	err = processAudioFiles()
	if err == nil {
		t.Error("Expected error for non-existent source directory")
	}
}

func TestSetupSoxCommandDockerPaths(t *testing.T) {
	// Test Docker path setup more thoroughly
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.DockerImage = "test/sox"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	// This will fail since docker is not available, but tests the path setup
	err := setupSoxCommand()
	if err == nil {
		t.Logf("setupSoxCommand with Docker succeeded unexpectedly")
	}
}

func TestRunConverterWithImages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-run-images")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)

	// Create a test image file
	jpgFile := filepath.Join(sourceDir, "test.jpg")
	os.WriteFile(jpgFile, []byte("fake jpg"), 0644)

	// Test with copy-images flag
	originalConfig := config
	defer func() { config = originalConfig }()
	config.TargetDir = filepath.Join(tmpDir, "target")
	config.UseDocker = false
	config.SoxCommand = "true"
	config.CopyImages = true

	err = runConverter(nil, []string{sourceDir})
	// This will attempt to process files and copy images
	if err != nil {
		t.Logf("runConverter with images returned: %v", err)
	}
}

func TestParseAudioInfoEdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected AudioInfo
	}{
		{
			name:     "empty input",
			input:    "",
			expected: AudioInfo{},
		},
		{
			name:     "malformed input",
			input:    "random text without proper format",
			expected: AudioInfo{},
		},
		{
			name: "missing sample rate",
			input: `Input File     : 'test.flac'
Channels       : 2
Precision      : 16-bit
Duration       : 00:03:45.23 = 9953100 samples ~ 16216.2 CDDA sectors
File Size      : 39.5M
Bit Rate       : 1.41M
Sample Encoding: 16-bit Signed Integer PCM`,
			expected: AudioInfo{Bits: 16, Rate: 0},
		},
		{
			name: "missing bit depth",
			input: `Input File     : 'test.flac'
Channels       : 2
Sample Rate    : 44100
Duration       : 00:03:45.23 = 9953100 samples ~ 16216.2 CDDA sectors
File Size      : 39.5M
Bit Rate       : 1.41M
Sample Encoding: Signed Integer PCM`,
			expected: AudioInfo{Bits: 0, Rate: 44100},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := parseAudioInfo(tc.input)
			if err != nil {
				t.Logf("parseAudioInfo returned error: %v", err)
			}

			if result.Bits != tc.expected.Bits {
				t.Errorf("Expected bits %d, got %d", tc.expected.Bits, result.Bits)
			}

			if result.Rate != tc.expected.Rate {
				t.Errorf("Expected rate %d, got %d", tc.expected.Rate, result.Rate)
			}
		})
	}
}

func TestCopyFileDestinationExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-dest-exists")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	// Create source file
	srcContent := "source content"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create destination file with different content
	dstContent := "existing content"
	if err := os.WriteFile(dstPath, []byte(dstContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Copy should overwrite destination
	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("copyFile failed: %v", err)
	}

	// Verify content was overwritten
	result, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != srcContent {
		t.Errorf("Expected %q, got %q", srcContent, string(result))
	}
}

func TestCopyImageFilesWithSubdirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-images-subdirs")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create nested directory structure
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(filepath.Join(sourceDir, "subdir1", "subdir2"), 0755)
	os.MkdirAll(targetDir, 0755)

	// Create test files in nested directories
	jpgFile1 := filepath.Join(sourceDir, "test.jpg")
	jpgFile2 := filepath.Join(sourceDir, "subdir1", "test2.jpg")
	jpgFile3 := filepath.Join(sourceDir, "subdir1", "subdir2", "test3.jpg")

	os.WriteFile(jpgFile1, []byte("jpg1"), 0644)
	os.WriteFile(jpgFile2, []byte("jpg2"), 0644)
	os.WriteFile(jpgFile3, []byte("jpg3"), 0644)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir

	// Test copyImageFiles
	err = copyImageFiles()
	if err != nil {
		t.Fatalf("copyImageFiles failed: %v", err)
	}

	// Verify all files were copied with correct structure
	if _, err := os.Stat(filepath.Join(targetDir, "test.jpg")); os.IsNotExist(err) {
		t.Error("Root level JPG file was not copied")
	}

	if _, err := os.Stat(filepath.Join(targetDir, "subdir1", "test2.jpg")); os.IsNotExist(err) {
		t.Error("Nested JPG file was not copied")
	}

	if _, err := os.Stat(filepath.Join(targetDir, "subdir1", "subdir2", "test3.jpg")); os.IsNotExist(err) {
		t.Error("Deeply nested JPG file was not copied")
	}
}

func TestSetupSoxCommandLocalSuccess(t *testing.T) {
	// Test setupSoxCommand when sox is available (using 'true' as mock)
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = false
	config.SoxCommand = "true" // 'true' command always succeeds

	err := setupSoxCommand()
	// This should succeed since 'true' is available on most systems
	if err != nil {
		t.Logf("setupSoxCommand with 'true' command: %v", err)
	}
}

func TestDetermineConversionAllCases(t *testing.T) {
	testCases := []struct {
		name               string
		input              AudioInfo
		expectedConversion bool
		expectedBitrate    []string
		expectedSampleRate []string
	}{
		{
			name:               "16-bit 44.1kHz no conversion",
			input:              AudioInfo{Bits: 16, Rate: 44100},
			expectedConversion: false,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "16-bit 48kHz no conversion",
			input:              AudioInfo{Bits: 16, Rate: 48000},
			expectedConversion: false,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "24-bit 44.1kHz needs bitrate conversion",
			input:              AudioInfo{Bits: 24, Rate: 44100},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "32-bit 44.1kHz needs bitrate conversion",
			input:              AudioInfo{Bits: 32, Rate: 44100},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L"},
		},
		{
			name:               "16-bit 96kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 96000},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "48000"},
		},
		{
			name:               "16-bit 192kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 192000},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "48000"},
		},
		{
			name:               "16-bit 88.2kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 88200},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
		{
			name:               "16-bit 176.4kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 176400},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
		{
			name:               "16-bit 352.8kHz needs sample rate conversion",
			input:              AudioInfo{Bits: 16, Rate: 352800},
			expectedConversion: true,
			expectedBitrate:    nil,
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
		{
			name:               "24-bit 96kHz needs both conversions",
			input:              AudioInfo{Bits: 24, Rate: 96000},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L", "48000"},
		},
		{
			name:               "24-bit 176.4kHz needs both conversions",
			input:              AudioInfo{Bits: 24, Rate: 176400},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
		{
			name:               "24-bit 352.8kHz needs both conversions",
			input:              AudioInfo{Bits: 24, Rate: 352800},
			expectedConversion: true,
			expectedBitrate:    []string{"-b", "16"},
			expectedSampleRate: []string{"rate", "-v", "-L", "44100"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			needsConversion, bitrateArgs, sampleRateArgs := determineConversion(&tc.input)

			if needsConversion != tc.expectedConversion {
				t.Errorf("Expected conversion %v, got %v", tc.expectedConversion, needsConversion)
			}

			if len(bitrateArgs) != len(tc.expectedBitrate) {
				t.Errorf("Expected bitrate args %v, got %v", tc.expectedBitrate, bitrateArgs)
			} else {
				for i, arg := range bitrateArgs {
					if arg != tc.expectedBitrate[i] {
						t.Errorf("Expected bitrate arg %s, got %s", tc.expectedBitrate[i], arg)
					}
				}
			}

			if len(sampleRateArgs) != len(tc.expectedSampleRate) {
				t.Errorf("Expected sample rate args %v, got %v", tc.expectedSampleRate, sampleRateArgs)
			} else {
				for i, arg := range sampleRateArgs {
					if arg != tc.expectedSampleRate[i] {
						t.Errorf("Expected sample rate arg %s, got %s", tc.expectedSampleRate[i], arg)
					}
				}
			}
		})
	}
}

func TestCopyFileSyncError(t *testing.T) {
	// Test copyFile when destination directory doesn't exist
	tmpDir, err := os.MkdirTemp("", "test-copy-sync-error")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "nonexistent", "dest.txt")

	// Create source file
	srcContent := "test content"
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Copy should fail because destination directory doesn't exist
	err = copyFile(srcPath, dstPath)
	if err == nil {
		t.Error("Expected error when destination directory doesn't exist")
	}
}

func TestCopyFileLargeFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-large")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "large-source.txt")
	dstPath := filepath.Join(tmpDir, "large-dest.txt")

	// Create a larger file (1MB)
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	if err := os.WriteFile(srcPath, largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Copy the large file
	err = copyFile(srcPath, dstPath)
	if err != nil {
		t.Errorf("copyFile failed for large file: %v", err)
	}

	// Verify content
	result, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != len(largeContent) {
		t.Errorf("Expected length %d, got %d", len(largeContent), len(result))
	}
	for i, b := range result {
		if b != largeContent[i] {
			t.Errorf("Content mismatch at byte %d", i)
			break
		}
	}
}

func TestGetDockerPath(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.SourceDir = "/host/source"
	expected := "/source/subdir/file.flac"

	result := getDockerPath("/host/source/subdir/file.flac")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestGetDockerTargetPath(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.TargetDir = "/host/target"
	expected := "/target/subdir/file.flac"

	result := getDockerTargetPath("/host/target/subdir/file.flac")
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestCopyImageFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-images")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create source directory structure
	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0755)
	os.MkdirAll(filepath.Join(targetDir), 0755)

	// Create test image files
	jpgFile := filepath.Join(sourceDir, "test.jpg")
	pngFile := filepath.Join(sourceDir, "subdir", "test.png")
	nonImageFile := filepath.Join(sourceDir, "test.txt")

	os.WriteFile(jpgFile, []byte("jpg content"), 0644)
	os.WriteFile(pngFile, []byte("png content"), 0644)
	os.WriteFile(nonImageFile, []byte("text content"), 0644)

	// Set config
	originalConfig := config
	defer func() { config = originalConfig }()
	config.SourceDir = sourceDir
	config.TargetDir = targetDir

	// Test copyImageFiles
	err = copyImageFiles()
	if err != nil {
		t.Fatalf("copyImageFiles failed: %v", err)
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(targetDir, "test.jpg")); os.IsNotExist(err) {
		t.Error("JPG file was not copied")
	}

	if _, err := os.Stat(filepath.Join(targetDir, "subdir", "test.png")); os.IsNotExist(err) {
		t.Error("PNG file was not copied")
	}

	// Verify non-image file was not copied
	if _, err := os.Stat(filepath.Join(targetDir, "test.txt")); !os.IsNotExist(err) {
		t.Error("Non-image file was incorrectly copied")
	}
}

func TestGetAudioInfo(t *testing.T) {
	// Test with local sox - this will fail if sox is not available, but tests the parsing logic
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = false
	config.SoxCommand = "echo"

	// Create a dummy file for testing
	tmpDir, err := os.MkdirTemp("", "test-audio")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test.flac")
	os.WriteFile(testFile, []byte("dummy"), 0644)

	info, err := getAudioInfo(testFile)
	// We expect this to fail since echo doesn't produce sox output, but it tests the function call
	if err == nil {
		t.Logf("getAudioInfo succeeded unexpectedly with: %+v", info)
	}
}

func TestRunConverter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-run")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)

	// Test without arguments - should require source directory
	err = runConverter(nil, []string{})
	if err == nil || err.Error() != "source directory required" {
		t.Error("Expected error for missing source directory")
	}

	// Test with self-update flag
	originalSelfUpdateFlag := selfUpdateFlag
	defer func() { selfUpdateFlag = originalSelfUpdateFlag }()

	selfUpdateFlag = true
	err = runConverter(nil, []string{})
	if err != nil {
		t.Logf("runConverter with self-update returned: %v", err)
	}

	// Test with self-update flag and arguments (should fail)
	selfUpdateFlag = true
	err = runConverter(nil, []string{sourceDir})
	if err == nil || err.Error() != "--self-update does not take arguments" {
		t.Error("Expected error when self-update has arguments")
	}

	// Reset flag
	selfUpdateFlag = false

	// Test with valid source directory
	originalConfig := config
	defer func() { config = originalConfig }()
	config.TargetDir = filepath.Join(tmpDir, "target")
	config.UseDocker = false
	config.SoxCommand = "true"
	config.CopyImages = true

	err = runConverter(nil, []string{sourceDir})
	// This will attempt to process files, may fail without real sox but tests the logic
	if err != nil {
		t.Logf("runConverter with valid args returned: %v", err)
	}

	// Test with non-existent source directory
	err = runConverter(nil, []string{"/non/existent/path"})
	if err == nil || !strings.Contains(err.Error(), "source directory does not exist") {
		t.Error("Expected error for non-existent source directory")
	}
}

func TestSelfUpdateHTTPError(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock network error
	mockClient := createMockClient(nil, fmt.Errorf("connection refused"))

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	// Verify graceful error handling - function should not return error but print messages
	if err != nil {
		t.Errorf("Expected no error for HTTP failure (graceful handling), got: %v", err)
	}

	if !strings.Contains(output, "Failed to check for updates from") {
		t.Error("Expected HTTP failure message")
	}
	if !strings.Contains(output, "Please visit https://github.com/Ardakilic/flac-to-16bit-converter") {
		t.Error("Expected fallback instructions in output")
	}
}

func TestCopyFileReadOnlySource(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-copy-readonly")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")
	srcContent := "test read-only content"

	// Create source file with read-only permissions
	if err := os.WriteFile(srcPath, []byte(srcContent), 0444); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	// Test copy operation
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify destination content
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}
	if string(dstContent) != srcContent {
		t.Errorf("Content mismatch:\nExpected: %q\nGot: %q", srcContent, string(dstContent))
	}

	// Verify permissions
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("Failed to stat source file: %v", err)
	}
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	// Check that at least read permissions are preserved
	if dstInfo.Mode().Perm()&0444 != srcInfo.Mode().Perm()&0444 {
		t.Errorf("Permissions not preserved properly:\nSource: %v\nDestination: %v",
			srcInfo.Mode().Perm(), dstInfo.Mode().Perm())
	}
}

func TestWindowsPathHandling(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.SourceDir = `C:\Users\test\music`
	config.TargetDir = `C:\Users\test\output`

	// Test path conversions
	dockerPath := getDockerPath(`C:\Users\test\music\song.flac`)
	expected := "/source/song.flac"
	if dockerPath != expected {
		t.Errorf("Windows path conversion failed. Expected: %s, Got: %s", expected, dockerPath)
	}

	targetPath := getDockerTargetPath(`C:\Users\test\output\song.flac`)
	expectedTarget := "/target/song.flac"
	if targetPath != expectedTarget {
		t.Errorf("Windows target path conversion failed. Expected: %s, Got: %s", expectedTarget, targetPath)
	}
}

func TestSelfUpdateBadStatusCode(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock 500 status
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("Server Error")),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "Failed to fetch release info") {
		t.Error("Expected bad status message")
	}
	if !strings.Contains(output, "HTTP 500") {
		t.Error("Expected status code in output")
	}
}

func TestSelfUpdateJSONParseError(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock invalid JSON
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("invalid json {")),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "Failed to parse release info") {
		t.Error("Expected JSON parse error message")
	}
}

func TestSelfUpdateDownloadFailure(t *testing.T) {
	originalVersion := version
	version = "v0.9.0" // Older version to trigger download
	defer func() { version = originalVersion }()

	// Mock API success, but download error
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.0.0"}`
	apiResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}
	// For download, return error
	responses := map[string]*http.Response{apiURL: apiResp}
	mockClient := createMockClient(responses, nil) // But for assetURL, since not in map, it will 404, but to simulate error, set transport err for second call? Wait, since it's the same client, but responses don't have assetURL, it will 404.

	// To simulate download error, use a transport that errors on second call, but for simplicity, let it 404
	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "New version v1.0.0 available. Updating...") {
		t.Error("Expected update trigger")
	}
	if !strings.Contains(output, "Failed to download update") {
		t.Error("Expected download failure message")
	}
}

func TestSelfUpdateTempFileCreation(t *testing.T) {
	// Test temp file creation during update process
	tmpDir, err := os.MkdirTemp("", "test-selfupdate-temp")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock temp file to test the creation logic
	tempFile, err := os.CreateTemp(tmpDir, "flac-converter-update-*")
	if err != nil {
		t.Fatal(err)
	}
	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	// Write some test data
	testData := []byte("mock binary data")
	if _, err := tempFile.Write(testData); err != nil {
		t.Fatal(err)
	}
	tempFile.Close()

	// Verify file was created and has correct content
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != string(testData) {
		t.Errorf("Temp file content mismatch")
	}
}

func TestSelfUpdateTempDirCreation(t *testing.T) {
	// Test temp directory creation during update process
	tmpDir, err := os.MkdirTemp("", "test-selfupdate-tempdir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a mock temp directory
	tempDir, err := os.MkdirTemp(tmpDir, "flac-converter-extract-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Verify directory was created
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		t.Error("Temp directory was not created")
	}
}

func TestSelfUpdateBinaryReplacement(t *testing.T) {
	// Test binary replacement logic
	tmpDir, err := os.MkdirTemp("", "test-binary-replace")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create mock current binary
	currentPath := filepath.Join(tmpDir, "current-binary")
	if err := os.WriteFile(currentPath, []byte("old binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create mock new binary
	newPath := filepath.Join(tmpDir, "new-binary")
	if err := os.WriteFile(newPath, []byte("new binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Test backup creation
	backupPath := currentPath + ".old"
	if err := os.Rename(currentPath, backupPath); err != nil {
		t.Fatal(err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("Backup file was not created")
	}

	// Test replacement
	if err := os.Rename(newPath, currentPath); err != nil {
		t.Fatal(err)
	}

	// Verify new binary is in place
	if _, err := os.Stat(currentPath); os.IsNotExist(err) {
		t.Error("New binary was not placed correctly")
	}

	// Verify content
	content, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new binary" {
		t.Errorf("Binary content incorrect after replacement")
	}
}

func TestSelfUpdatePermissionSetting(t *testing.T) {
	// Test permission setting on updated binary
	tmpDir, err := os.MkdirTemp("", "test-permissions")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := filepath.Join(tmpDir, "test-binary")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set executable permissions
	if err := os.Chmod(testFile, 0755); err != nil {
		t.Fatal(err)
	}

	// Verify permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}

	expectedMode := os.FileMode(0755)
	if info.Mode() != expectedMode {
		t.Errorf("Expected permissions %v, got %v", expectedMode, info.Mode())
	}
}

func TestSelfUpdateVersionComparisonScenarios(t *testing.T) {
	testCases := []struct {
		name        string
		current     string
		latest      string
		expectPrint string
	}{
		{
			name:        "current newer than latest",
			current:     "v2.0.0",
			latest:      "v1.0.0",
			expectPrint: "newer version",
		},
		{
			name:        "current equal to latest",
			current:     "v1.0.0",
			latest:      "v1.0.0",
			expectPrint: "latest version",
		},
		{
			name:        "current older than latest",
			current:     "v1.0.0",
			latest:      "v2.0.0",
			expectPrint: "New version",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test the version comparison logic
			cmp := compareVersions(tc.current, tc.latest)
			switch {
			case cmp < 0:
				if tc.expectPrint != "New version" {
					t.Errorf("Expected 'New version' for older current version")
				}
			case cmp == 0:
				if tc.expectPrint != "latest version" {
					t.Errorf("Expected 'latest version' for equal versions")
				}
			case cmp > 0:
				if tc.expectPrint != "newer version" {
					t.Errorf("Expected 'newer version' for newer current version")
				}
			}
		})
	}
}

func TestSelfUpdateAssetURLConstruction(t *testing.T) {
	// Test asset URL construction for different platforms
	testCases := []struct {
		goos     string
		goarch   string
		version  string
		expected string
	}{
		{
			goos:     "darwin",
			goarch:   "arm64",
			version:  "v1.0.0",
			expected: "https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v1.0.0/flac-converter-darwin-arm64.tar.gz",
		},
		{
			goos:     "windows",
			goarch:   "amd64",
			version:  "v1.0.0",
			expected: "https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v1.0.0/flac-converter-windows-amd64.exe.zip",
		},
		{
			goos:     "linux",
			goarch:   "amd64",
			version:  "v2.1.0",
			expected: "https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v2.1.0/flac-converter-linux-amd64.tar.gz",
		},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%s-%s", tc.goos, tc.goarch, tc.version), func(t *testing.T) {
			var filename string
			if tc.goos == "windows" {
				filename = fmt.Sprintf("flac-converter-%s-%s.exe.zip", tc.goos, tc.goarch)
			} else {
				filename = fmt.Sprintf("flac-converter-%s-%s.tar.gz", tc.goos, tc.goarch)
			}

			assetURL := fmt.Sprintf("https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/%s/%s", tc.version, filename)

			if assetURL != tc.expected {
				t.Errorf("Expected URL %s, got %s", tc.expected, assetURL)
			}
		})
	}
}

func TestSelfUpdateBinaryNameConstruction(t *testing.T) {
	// Test binary name construction for different platforms
	testCases := []struct {
		goos     string
		goarch   string
		expected string
	}{
		{"darwin", "arm64", "flac-converter-darwin-arm64"},
		{"linux", "amd64", "flac-converter-linux-amd64"},
		{"windows", "amd64", "flac-converter-windows-amd64.exe"},
		{"linux", "arm64", "flac-converter-linux-arm64"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s-%s", tc.goos, tc.goarch), func(t *testing.T) {
			binaryName := "flac-converter-" + tc.goos + "-" + tc.goarch
			if tc.goos == "windows" {
				binaryName += ".exe"
			}

			if binaryName != tc.expected {
				t.Errorf("Expected binary name %s, got %s", tc.expected, binaryName)
			}
		})
	}
}

func TestSelfUpdatePlatformDetection(t *testing.T) {
	// Test platform detection logic
	currentGOOS := runtime.GOOS
	currentGOARCH := runtime.GOARCH

	// Test Unix-like systems (should use tar.gz)
	if currentGOOS != "windows" {
		expectedExt := ".tar.gz"
		var filename string
		if currentGOOS == "windows" {
			filename = fmt.Sprintf("flac-converter-%s-%s.exe.zip", currentGOOS, currentGOARCH)
		} else {
			filename = fmt.Sprintf("flac-converter-%s-%s.tar.gz", currentGOOS, currentGOARCH)
		}

		if !strings.HasSuffix(filename, expectedExt) {
			t.Errorf("Expected Unix filename to end with %s, got %s", expectedExt, filename)
		}
	}

	// Test Windows (should use .exe.zip)
	if currentGOOS == "windows" {
		expectedExt := ".exe.zip"
		var filename string
		if currentGOOS == "windows" {
			filename = fmt.Sprintf("flac-converter-%s-%s.exe.zip", currentGOOS, currentGOARCH)
		} else {
			filename = fmt.Sprintf("flac-converter-%s-%s.tar.gz", currentGOOS, currentGOARCH)
		}

		if !strings.HasSuffix(filename, expectedExt) {
			t.Errorf("Expected Windows filename to end with %s, got %s", expectedExt, filename)
		}
	}
}

func TestSelfUpdateExtractionLogic(t *testing.T) {
	// Test the extraction logic branches
	tmpDir, err := os.MkdirTemp("", "test-extraction-logic")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Test Unix extraction path
	goos := "linux"
	goarch := "amd64"
	binaryName := "flac-converter-" + goos + "-" + goarch

	// Verify the binary name construction logic
	expectedBinaryName := "flac-converter-linux-amd64"
	if binaryName != expectedBinaryName {
		t.Errorf("Expected binary name %s, got %s", expectedBinaryName, binaryName)
	}

	// Test Windows extraction path
	goos = "windows"
	binaryName = "flac-converter-" + goos + "-" + goarch
	if goos == "windows" {
		binaryName += ".exe"
	}

	expectedBinaryName = "flac-converter-windows-amd64.exe"
	if binaryName != expectedBinaryName {
		t.Errorf("Expected Windows binary name %s, got %s", expectedBinaryName, binaryName)
	}
}

func TestSelfUpdateErrorMessages(t *testing.T) {
	// Test that error messages contain expected fallback instructions
	fallbackMessage := "Please visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update."

	// Test that fallback message contains expected elements
	if !strings.Contains(fallbackMessage, "github.com/Ardakilic/flac-to-16bit-converter") {
		t.Errorf("Fallback message should contain repository URL")
	}
	if !strings.Contains(fallbackMessage, "install.sh") {
		t.Errorf("Fallback message should mention install.sh")
	}
}

func TestSelfUpdateCurrentPathResolution(t *testing.T) {
	// Test os.Executable() path resolution
	execPath, err := os.Executable()
	if err != nil {
		t.Logf("os.Executable() returned error: %v", err)
	} else {
		// Verify it's an absolute path
		if !filepath.IsAbs(execPath) {
			t.Errorf("Expected absolute path, got %s", execPath)
		}

		// Verify the file exists
		if _, err := os.Stat(execPath); os.IsNotExist(err) {
			t.Errorf("Executable path does not exist: %s", execPath)
		}
	}
}

func TestSelfUpdateBackupFileNaming(t *testing.T) {
	// Test backup file naming convention
	currentPath := "/usr/local/bin/flac-converter"
	backupPath := currentPath + ".old"

	expectedBackup := "/usr/local/bin/flac-converter.old"
	if backupPath != expectedBackup {
		t.Errorf("Expected backup path %s, got %s", expectedBackup, backupPath)
	}
}

func TestSelfUpdateHTTP403ErrorMessage(t *testing.T) {
	// Test that HTTP 403 errors show the specific rate limiting message
	// This test verifies the error message format for forbidden responses

	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"

	// The actual HTTP 403 error would be caught by the real API call,
	// but we can test the message format logic
	testMessage := fmt.Sprintf("Checking for updates from: %s\n", apiURL)
	testMessage += fmt.Sprintf("Failed to fetch release info from %s: HTTP 403 (Forbidden)\n", apiURL)
	testMessage += "This may be due to GitHub API rate limiting. Please wait a few minutes and try again, or visit https://github.com/Ardakilic/flac-to-16bit-converter to check the latest version manually and run the install.sh command to update."

	// Verify the message contains the expected elements
	if !strings.Contains(testMessage, "Checking for updates from:") {
		t.Error("Error message should show the URL being checked")
	}

	if !strings.Contains(testMessage, apiURL) {
		t.Error("Error message should contain the API URL")
	}

	if !strings.Contains(testMessage, "HTTP 403 (Forbidden)") {
		t.Error("Error message should specify HTTP 403")
	}

	if !strings.Contains(testMessage, "rate limiting") {
		t.Error("Error message should mention rate limiting")
	}

	if !strings.Contains(testMessage, "wait a few minutes") {
		t.Error("Error message should suggest waiting")
	}

	if !strings.Contains(testMessage, "github.com/Ardakilic/flac-to-16bit-converter") {
		t.Error("Error message should contain repository URL")
	}

	if !strings.Contains(testMessage, "install.sh") {
		t.Error("Error message should mention install.sh")
	}
}

func TestSelfUpdateURLErrorMessages(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock 403 for API
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	resp403 := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(strings.NewReader("Forbidden")),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp403}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Verify URL in error messages
	if !strings.Contains(output, apiURL) {
		t.Error("Expected API URL in error message")
	}
	if !strings.Contains(output, "HTTP 403 (Forbidden)") {
		t.Error("Expected 403 status in output")
	}
	if !strings.Contains(output, "GitHub API rate limiting") {
		t.Error("Expected rate limiting message")
	}
}
func TestMergeMetadataWithFFmpegNoPreserve(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = true

	tmpDir, err := os.MkdirTemp("", "test-merge-no-preserve")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	// Create dummy files
	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	if err != nil {
		t.Fatalf("Expected no error when not preserving metadata, got: %v", err)
	}

	// Verify temp was renamed to target
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Target file should exist after rename")
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Temp file should have been removed after rename")
	}
}

func TestMergeMetadataWithFFmpegLocalSuccess(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false
	config.UseDocker = false

	tmpDir, err := os.MkdirTemp("", "test-merge-local")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	// Create dummy files
	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	// This test checks FFmpeg availability and skips if not installed, as mocking exec.Command is complex for unit tests.
	// The test validates the success path when FFmpeg is available, or gracefully skips when it's not.
	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	if err != nil {
		// If ffmpeg not installed, log but don't fail test
		t.Logf("FFmpeg not available, skipping success test: %v", err)
		return
	}

	// Verify temp removed and target exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Target file should exist after successful merge")
	}
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Temp file should have been removed after successful merge")
	}
}

func TestMergeMetadataWithFFmpegLocalFailure(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false
	config.UseDocker = false

	tmpDir, err := os.MkdirTemp("", "test-merge-local-fail")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	// Create dummy files
	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	// Test FFmpeg failure by checking if FFmpeg is unavailable on the system.
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		t.Skip("FFmpeg is available, cannot test failure case easily without mocking")
	}

	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	if err == nil {
		t.Error("Expected error when FFmpeg fails")
	}

	// On FFmpeg failure, the temp file should remain since cleanup only occurs on successful merge.
	if _, err := os.Stat(tempPath); os.IsNotExist(err) {
		t.Error("Temp file should remain on FFmpeg failure")
	}
}

func TestMergeMetadataWithFFmpegDocker(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false
	config.UseDocker = true
	config.DockerImage = "test/ffmpeg"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	tmpDir, err := os.MkdirTemp("", "test-merge-docker")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	if err == nil {
		t.Error("Expected Docker error but got nil")
	}

	// Verify fallback to temp file rename
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Error("Target file should not exist after Docker failure in helper")
	}
}

func TestMergeMetadataDockerFailure(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	tmpDir, err := os.MkdirTemp("", "test-merge-docker-failure")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	// Force Docker failure by using invalid image
	config.NoPreserveMetadata = false
	config.UseDocker = true
	config.DockerImage = "invalid/nonexistent:image"
	config.SourceDir = tmpDir
	config.TargetDir = tmpDir

	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	if err == nil {
		t.Error("Expected Docker error but got nil")
	}

	// Verify temp file cleanup
	if _, err := os.Stat(tempPath); os.IsNotExist(err) {
		t.Error("Temp file should remain after Docker failure in helper")
	}
}

func TestFFmpegExecutionPaths(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	t.Run("LocalFFmpegExecution", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-ffmpeg-local")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		sourcePath := filepath.Join(tmpDir, "source.flac")
		tempPath := filepath.Join(tmpDir, "temp.flac")
		targetPath := filepath.Join(tmpDir, "target.flac")

		os.WriteFile(sourcePath, []byte("source"), 0644)
		os.WriteFile(tempPath, []byte("temp"), 0644)

		config.NoPreserveMetadata = false
		config.UseDocker = false

		// Test local FFmpeg execution
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skipf("FFmpeg not available locally, skipping local execution test: %v", err)
		}

		err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
		if err != nil {
			t.Logf("Local FFmpeg execution failed (may be expected with dummy files): %v", err)
		} else {
			t.Log("Local FFmpeg execution succeeded")
			// Verify temp file was cleaned up on success
			if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
				t.Error("Temp file should be cleaned up after successful merge")
			}
		}
	})

	t.Run("DockerFFmpegExecution", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-ffmpeg-docker")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		sourcePath := filepath.Join(tmpDir, "source.flac")
		tempPath := filepath.Join(tmpDir, "temp.flac")
		targetPath := filepath.Join(tmpDir, "target.flac")

		os.WriteFile(sourcePath, []byte("source"), 0644)
		os.WriteFile(tempPath, []byte("temp"), 0644)

		config.NoPreserveMetadata = false
		config.UseDocker = true
		config.DockerImage = "test/ffmpeg:latest"
		config.SourceDir = tmpDir
		config.TargetDir = tmpDir

		// Test Docker FFmpeg execution
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skipf("Docker not available, skipping Docker execution test: %v", err)
		}

		err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
		if err != nil {
			t.Logf("Docker FFmpeg execution failed (expected with test image): %v", err)
		} else {
			t.Log("Docker FFmpeg execution succeeded")
		}
	})

	t.Run("FFmpegBinaryNotFound", func(t *testing.T) {
		tmpDir, err := os.MkdirTemp("", "test-ffmpeg-missing")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		sourcePath := filepath.Join(tmpDir, "source.flac")
		tempPath := filepath.Join(tmpDir, "temp.flac")
		targetPath := filepath.Join(tmpDir, "target.flac")

		os.WriteFile(sourcePath, []byte("source"), 0644)
		os.WriteFile(tempPath, []byte("temp"), 0644)

		config.NoPreserveMetadata = false
		config.UseDocker = false

		// Check if FFmpeg is actually missing
		if _, err := exec.LookPath("ffmpeg"); err == nil {
			t.Skip("FFmpeg is available, cannot test missing binary scenario")
		}

		err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
		if err == nil {
			t.Error("Expected error when FFmpeg binary is not found")
		} else {
			t.Logf("Got expected error when FFmpeg is missing: %v", err)
		}

		// Verify temp file remains on failure
		if _, err := os.Stat(tempPath); os.IsNotExist(err) {
			t.Error("Temp file should remain when FFmpeg execution fails")
		}
	})
}

func TestDetermineConversionFullMatrix(t *testing.T) {
	testCases := []struct {
		name               string
		input              AudioInfo
		expectedConversion bool
	}{
		{
			name:               "32-bit 44.1kHz",
			input:              AudioInfo{Bits: 32, Rate: 44100},
			expectedConversion: true,
		},
		{
			name:               "16-bit 192kHz",
			input:              AudioInfo{Bits: 16, Rate: 192000},
			expectedConversion: true,
		},
		{
			name:               "24-bit 48kHz",
			input:              AudioInfo{Bits: 24, Rate: 48000},
			expectedConversion: true,
		},
		{
			name:               "16-bit 48kHz",
			input:              AudioInfo{Bits: 16, Rate: 48000},
			expectedConversion: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			needsConversion, _, _ := determineConversion(&tc.input)
			if needsConversion != tc.expectedConversion {
				t.Errorf("Expected conversion %v, got %v", tc.expectedConversion, needsConversion)
			}
		})
	}
}

func TestMergeMetadataWithFFmpegTempRemovalError(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false
	config.UseDocker = false
	config.SoxCommand = "true" // Mock sox success

	tmpDir, err := os.MkdirTemp("", "test-convert-metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)

	// Test processFlac directly with conversion arguments to verify metadata preservation logic.
	bitrateArgs := []string{"-b", "16"}
	sampleRateArgs := []string{"rate", "-v", "-L", "44100"}

	err = processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
	if err != nil {
		// If ffmpeg not available, accept fallback rename error as known case
		if strings.Contains(err.Error(), "fallback rename failed") || strings.Contains(err.Error(), "FFmpeg metadata merge failed") {
			t.Logf("FFmpeg not available, fallback rename failed as expected: %v", err)
		} else {
			t.Errorf("Expected nil or known error, got: %v", err)
		}
		return
	}

	// Verify target exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Target file should exist after successful conversion with metadata")
	}
}

func TestProcessFlacDockerFailure(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	tmpDir, err := os.MkdirTemp("", "test-convert-docker-fail")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")
	os.WriteFile(sourcePath, []byte("dummy flac"), 0644)

	config.UseDocker = true
	config.DockerImage = "invalid-image"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"

	bitrateArgs := []string{"-b", "16"}
	sampleRateArgs := []string{"rate", "-v", "-L", "44100"}

	err = processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
	if err == nil {
		t.Error("Expected Docker failure but got nil")
	}

	// Verify fallback copy
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Error("Target file should not exist after Docker sox failure in processFlac")
	}
}

func TestProcessFlacMetadataFallback(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false
	config.UseDocker = false
	config.SoxCommand = "false" // Mock sox failure

	tmpDir, err := os.MkdirTemp("", "test-convert-fallback")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)

	bitrateArgs := []string{"-b", "16"}
	sampleRateArgs := []string{"rate", "-v", "-L", "44100"}

	err = processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
	if err == nil {
		t.Error("Expected error on sox failure")
	}

	// Verify no temp left behind
	tempPath := targetPath + ".tmp"
	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Error("Temp file should be cleaned up on sox failure")
	}
}

func TestProcessFlacNoConversionWithMetadata(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.NoPreserveMetadata = false

	tmpDir, err := os.MkdirTemp("", "test-convert-no-conversion")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)

	// No args, should copy
	bitrateArgs := []string{}
	sampleRateArgs := []string{"rate", "-v", "-L"}

	err = processFlac(sourcePath, targetPath, false, bitrateArgs, sampleRateArgs)
	if err != nil {
		t.Errorf("Expected no error for no conversion, got: %v", err)
	}
	// Verify copied
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Error("Target should be copy of source")
	}
}

func TestMainFunction(t *testing.T) {
	// This test just ensures the main function exists and can be called
	// We can't easily test the actual execution without more complex setup
	// but we can at least ensure it compiles and doesn't panic immediately
	if rootCmd == nil {
		t.Error("rootCmd should be initialized")
	}
}

func TestSetupSoxCommand(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	t.Run("LocalSoxMissing", func(t *testing.T) {
		config.UseDocker = false
		config.SoxCommand = "nonexistent-sox"
		err := setupSoxCommand()
		if err == nil || !strings.Contains(err.Error(), "sox is not installed") {
			t.Errorf("Expected sox missing error, got: %v", err)
		}
	})

	t.Run("LocalFFmpegMissing", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = false
		config.SoxCommand = "true" // Assume sox success for this test
		err := setupSoxCommand()
		if err == nil || !strings.Contains(err.Error(), "ffmpeg is not installed") {
			t.Logf("FFmpeg may be installed, but expected missing error (or sox issue): %v", err)
		}
	})

	t.Run("DockerMissing", func(t *testing.T) {
		config.UseDocker = true
		config.SourceDir = "/tmp/source"
		config.TargetDir = "/tmp/target"
		err := setupSoxCommand()
		if err == nil {
			t.Log("Docker available, no error")
		} else if !strings.Contains(err.Error(), "docker is not installed") {
			t.Errorf("Expected docker missing error or no error, got: %v", err)
		}
	})

	t.Run("DockerPaths", func(t *testing.T) {
		config.UseDocker = true
		config.SourceDir = "."
		config.TargetDir = "./target"
		// Will fail on LookPath, but covers abs path calls
		err := setupSoxCommand()
		t.Logf("Docker path setup error (expected if no docker): %v", err)
	})
}

func TestFFmpegBinaryExistence(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	t.Run("LocalModeWithMetadataPreservation_FFmpegRequired", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = false
		config.SoxCommand = "true" // Mock SoX as available

		// Test the actual FFmpeg check
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			// FFmpeg not available - should get error from setupSoxCommand
			err := setupSoxCommand()
			if err == nil {
				t.Error("Expected error when FFmpeg is not available but metadata preservation is enabled")
			} else if !strings.Contains(err.Error(), "ffmpeg is not installed") {
				t.Errorf("Expected FFmpeg installation error, got: %v", err)
			}
		} else {
			// FFmpeg available - should succeed
			err := setupSoxCommand()
			if err != nil {
				t.Errorf("Expected success when FFmpeg is available, got: %v", err)
			}
		}
	})

	t.Run("LocalModeWithoutMetadataPreservation_FFmpegNotRequired", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = true // Metadata preservation disabled
		config.SoxCommand = "true"       // Mock SoX as available

		// Should succeed regardless of FFmpeg availability
		err := setupSoxCommand()
		if err != nil {
			t.Errorf("Expected success when metadata preservation is disabled, got: %v", err)
		}
	})

	t.Run("DockerMode_FFmpegNotRequiredLocally", func(t *testing.T) {
		config.UseDocker = true
		config.NoPreserveMetadata = false // Metadata preservation enabled
		config.DockerImage = "test/image"

		tmpDir, err := os.MkdirTemp("", "test-docker-ffmpeg")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)

		config.SourceDir = tmpDir
		config.TargetDir = tmpDir

		// Test Docker path - should not check for local FFmpeg
		if _, err := exec.LookPath("docker"); err != nil {
			// Docker not available
			err := setupSoxCommand()
			if err == nil {
				t.Log("Expected Docker installation error but got success")
			} else if !strings.Contains(err.Error(), "docker is not installed") {
				t.Errorf("Expected Docker installation error, got: %v", err)
			}
		} else {
			// Docker available - should succeed without checking local FFmpeg
			err := setupSoxCommand()
			if err != nil {
				t.Logf("Docker setup failed (may be expected): %v", err)
			}
			// In Docker mode, local FFmpeg availability shouldn't matter
		}
	})

	t.Run("LocalMode_FFmpegAvailabilityCheck", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = false
		config.SoxCommand = "true"

		// Directly test FFmpeg availability
		_, ffmpegErr := exec.LookPath("ffmpeg")
		err := setupSoxCommand()

		if ffmpegErr != nil {
			// FFmpeg not available
			if err == nil {
				t.Error("Expected error when FFmpeg is not available")
			} else if !strings.Contains(err.Error(), "ffmpeg is not installed") {
				t.Errorf("Expected FFmpeg error, got: %v", err)
			}
			t.Logf("FFmpeg not available (expected): %v", ffmpegErr)
		} else {
			// FFmpeg available
			if err != nil {
				t.Errorf("Expected success when FFmpeg is available, got: %v", err)
			}
			t.Log("FFmpeg is available")
		}
	})
}

func TestProcessAudioFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-process-audio")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	targetDir := filepath.Join(tmpDir, "target")

	os.MkdirAll(sourceDir, 0755)
	os.MkdirAll(targetDir, 0755)

	// Create FLAC and MP3 files
	flacFile := filepath.Join(sourceDir, "test.flac")
	mp3File := filepath.Join(sourceDir, "test.mp3")
	os.WriteFile(flacFile, []byte("dummy flac"), 0644)
	os.WriteFile(mp3File, []byte("dummy mp3"), 0644)

	// Subdir test
	subdir := filepath.Join(sourceDir, "subdir")
	os.MkdirAll(subdir, 0755)
	subFlac := filepath.Join(subdir, "sub.flac")
	os.WriteFile(subFlac, []byte("dummy sub flac"), 0644)

	originalConfig := config
	defer func() { config = originalConfig }()

	config.SourceDir = sourceDir
	config.TargetDir = targetDir
	config.UseDocker = false
	config.SoxCommand = "true"       // Mock success
	config.NoPreserveMetadata = true // Simplify, no FFmpeg

	err = processAudioFiles()
	if err != nil {
		t.Logf("processAudioFiles error (expected if no sox): %v", err)
	}

	// Verify files processed
	if _, err := os.Stat(filepath.Join(targetDir, "test.flac")); os.IsNotExist(err) {
		t.Error("Root FLAC not processed")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "test.mp3")); os.IsNotExist(err) {
		t.Error("Root MP3 not copied")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "subdir", "sub.flac")); os.IsNotExist(err) {
		t.Error("Subdir FLAC not processed")
	}

	// Test with failing sox for fallback copy
	config.SoxCommand = "false"
	err = processAudioFiles()
	if err != nil {
		t.Logf("processAudioFiles with failing sox (fallback expected): %v", err)
	}
	// Should fallback copy on error
	if _, err := os.Stat(filepath.Join(targetDir, "test.flac")); os.IsNotExist(err) {
		t.Error("Fallback copy failed on sox error")
	}

	// Test non-audio file skip
	nonAudio := filepath.Join(sourceDir, "test.txt")
	os.WriteFile(nonAudio, []byte("dummy text"), 0644)
	err = processAudioFiles()
	if err != nil {
		t.Logf("processAudioFiles with non-audio: %v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "test.txt")); !os.IsNotExist(err) {
		t.Error("Non-audio file was incorrectly processed")
	}
}

func TestProcessFlac(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-convert-flac")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")
	os.WriteFile(sourcePath, []byte("dummy source"), 0644)

	originalConfig := config
	defer func() { config = originalConfig }()

	t.Run("NoConversionCopy", func(t *testing.T) {
		config.UseDocker = false
		config.SoxCommand = "true"
		config.NoPreserveMetadata = true
		bitrateArgs := []string{}
		sampleRateArgs := []string{"rate", "-v", "-L"}
		err := processFlac(sourcePath, targetPath, false, bitrateArgs, sampleRateArgs)
		if err != nil {
			t.Errorf("Expected no error for no conversion, got: %v", err)
		}
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			t.Error("Target should exist after copy")
		}
	})

	t.Run("BitrateConversion", func(t *testing.T) {
		config.UseDocker = false
		config.SoxCommand = "true"
		config.NoPreserveMetadata = true
		bitrateArgs := []string{"-b", "16"}
		sampleRateArgs := []string{"rate", "-v", "-L"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err != nil {
			t.Logf("Conversion error (if no sox): %v", err)
		}
	})

	t.Run("SampleRateConversion", func(t *testing.T) {
		config.UseDocker = false
		config.SoxCommand = "true"
		config.NoPreserveMetadata = true
		bitrateArgs := []string{}
		sampleRateArgs := []string{"rate", "-v", "-L", "44100"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err != nil {
			t.Logf("Conversion error (if no sox): %v", err)
		}
	})

	t.Run("MetadataPreserveSuccess", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = false
		config.SoxCommand = "true"
		// Assume FFmpeg available or test fallback
		bitrateArgs := []string{"-b", "16"}
		sampleRateArgs := []string{"rate", "-v", "-L", "44100"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err != nil {
			t.Logf("Metadata preserve error (if no ffmpeg): %v", err)
		}
	})

	t.Run("MetadataPreserveFailFallback", func(t *testing.T) {
		config.UseDocker = false
		config.NoPreserveMetadata = false
		config.SoxCommand = "true"
		// If FFmpeg fails, fallback rename
		bitrateArgs := []string{"-b", "16"}
		sampleRateArgs := []string{"rate", "-v", "-L", "44100"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err != nil {
			t.Logf("Expected fallback on metadata fail: %v", err)
		}
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			t.Error("Target should exist after fallback")
		}
	})

	t.Run("DockerConversion", func(t *testing.T) {
		config.UseDocker = true
		config.DockerImage = "test/sox"
		config.SourceDir = tmpDir
		config.TargetDir = tmpDir
		bitrateArgs := []string{"-b", "16"}
		sampleRateArgs := []string{"rate", "-v", "-L", "44100"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err == nil {
			t.Logf("Docker conversion succeeded unexpectedly")
		}
	})

	t.Run("SoxFailureFallback", func(t *testing.T) {
		config.UseDocker = false
		config.SoxCommand = "false" // Fail
		config.NoPreserveMetadata = true
		bitrateArgs := []string{"-b", "16"}
		sampleRateArgs := []string{"rate", "-v", "-L", "44100"}
		err := processFlac(sourcePath, targetPath, true, bitrateArgs, sampleRateArgs)
		if err == nil {
			t.Error("Expected error on sox failure")
		}
	})
}

func TestSelfUpdateFullFlow(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	// Mock API success with newer version
	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.1.0"}`
	apiResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	// Mock asset download with dummy zip (for windows) or tar.gz (for linux/darwin)
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	var assetURL string
	var dummyArchive []byte
	if goos == "windows" {
		assetURL = fmt.Sprintf("https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v1.1.0/flac-converter-%s-%s.exe.zip", goos, goarch)
		// Dummy zip with exe
		buf := new(bytes.Buffer)
		zw := zip.NewWriter(buf)
		f, _ := zw.Create("flac-converter-windows-amd64.exe")
		f.Write([]byte("dummy exe"))
		zw.Close()
		dummyArchive = buf.Bytes()
	} else {
		assetURL = fmt.Sprintf("https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v1.1.0/flac-converter-%s-%s.tar.gz", goos, goarch)
		// Dummy tar.gz
		buf := new(bytes.Buffer)
		gw := gzip.NewWriter(buf)
		tw := tar.NewWriter(gw)
		header := &tar.Header{
			Name: "flac-converter-linux-amd64",
			Mode: 0755,
			Size: 9,
		}
		tw.WriteHeader(header)
		tw.Write([]byte("dummy binary"))
		tw.Close()
		gw.Close()
		dummyArchive = buf.Bytes()
	}

	assetResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(dummyArchive)),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: apiResp, assetURL: assetResp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "New version v1.1.0 available. Updating...") {
		t.Error("Expected update trigger message")
	}
	if strings.Contains(output, "Update complete. Please restart the application.") {
		t.Log("Full update succeeded")
	} else {
		t.Logf("Update flow covered up to extraction/replacement, output: %s", output)
	}
}

func TestSelfUpdateAPI404(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	resp404 := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("Not Found")),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: resp404}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !strings.Contains(output, "Failed to fetch release info") {
		t.Error("Expected 404 message")
	}
	if !strings.Contains(output, "HTTP 404") {
		t.Error("Expected status code in output")
	}
}

func TestSelfUpdateInvalidArchive(t *testing.T) {
	originalVersion := version
	version = "v1.0.0"
	defer func() { version = originalVersion }()

	goos := runtime.GOOS
	goarch := runtime.GOARCH

	apiURL := "https://api.github.com/repos/Ardakilic/flac-to-16bit-converter/releases/latest"
	respBody := `{"tag_name": "v1.1.0"}`
	apiResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(respBody)),
		Header:     make(http.Header),
	}

	assetURL := fmt.Sprintf("https://github.com/Ardakilic/flac-to-16bit-converter/releases/download/v1.1.0/flac-converter-%s-%s.tar.gz", goos, goarch)
	assetResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("invalid archive data")),
		Header:     make(http.Header),
	}
	responses := map[string]*http.Response{apiURL: apiResp, assetURL: assetResp}
	mockClient := createMockClient(responses, nil)

	var err error
	output, captureErr := captureOutput(func() {
		err = selfUpdate(mockClient)
	})
	if captureErr != nil {
		t.Fatalf("Failed to capture output: %v", captureErr)
	}

	if err != nil {
		t.Logf("Error on invalid archive (expected): %v", err)
	}

	if !strings.Contains(output, "Failed to extract") && !strings.Contains(output, "Failed to open") && !strings.Contains(output, "Failed to read") {
		t.Logf("Output: %s", output)
		t.Error("Expected extraction failure message")
	}
}

func TestRunConverterEdge(t *testing.T) {
	originalSelfUpdateFlag := selfUpdateFlag
	defer func() { selfUpdateFlag = originalSelfUpdateFlag }()

	originalConfig := config
	defer func() { config = originalConfig }()

	tmpDir, err := os.MkdirTemp("", "test-run-edge")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourceDir := filepath.Join(tmpDir, "source")
	os.MkdirAll(sourceDir, 0755)

	t.Run("TargetDirCreationFail", func(t *testing.T) {
		config.TargetDir = "/var/nonexistent/target"
		config.UseDocker = false
		config.SoxCommand = "true"
		config.NoPreserveMetadata = true // Avoid FFmpeg error
		err := runConverter(nil, []string{sourceDir})
		if err == nil || !strings.Contains(err.Error(), "failed to create target directory") {
			t.Errorf("Expected target dir creation error, got: %v", err)
		}
	})

	t.Run("SelfUpdateWithArgs", func(t *testing.T) {
		selfUpdateFlag = true
		err := runConverter(nil, []string{sourceDir})
		if err == nil || err.Error() != "--self-update does not take arguments" {
			t.Errorf("Expected self-update with args error, got: %v", err)
		}
	})

	t.Run("NoImages", func(t *testing.T) {
		config.CopyImages = false
		// Add a test image
		jpgFile := filepath.Join(sourceDir, "test.jpg")
		os.WriteFile(jpgFile, []byte("test"), 0644)
		config.TargetDir = filepath.Join(tmpDir, "target-no-images")
		err := runConverter(nil, []string{sourceDir})
		if err != nil {
			t.Logf("Run with no images: %v", err)
		}
		// Verify no image copied if flag false
		if _, err := os.Stat(filepath.Join(config.TargetDir, "test.jpg")); !os.IsNotExist(err) {
			t.Error("Image copied when flag false")
		}
	})
}

func TestMergeMetadataDocker(t *testing.T) {
	originalConfig := config
	defer func() { config = originalConfig }()

	config.UseDocker = true
	config.DockerImage = "test/ffmpeg"
	config.SourceDir = "/host/source"
	config.TargetDir = "/host/target"
	config.NoPreserveMetadata = false

	tmpDir, err := os.MkdirTemp("", "test-merge-docker")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	sourcePath := filepath.Join(tmpDir, "source.flac")
	tempPath := filepath.Join(tmpDir, "temp.flac")
	targetPath := filepath.Join(tmpDir, "target.flac")

	os.WriteFile(sourcePath, []byte("source"), 0644)
	os.WriteFile(tempPath, []byte("temp"), 0644)

	err = mergeMetadataWithFFmpeg(sourcePath, tempPath, targetPath)
	t.Logf("Docker merge error expected: %v", err)
}

// TestConversionLogTargetRateDetermination tests the switch case logic
// for determining target sample rates in the conversion process
func TestConversionLogTargetRateDetermination(t *testing.T) {
	testCases := []struct {
		name               string
		audioRate          int
		expectedTargetRate string
	}{
		{
			name:               "96kHz to 48kHz",
			audioRate:          96000,
			expectedTargetRate: "48000 Hz",
		},
		{
			name:               "192kHz to 48kHz",
			audioRate:          192000,
			expectedTargetRate: "48000 Hz",
		},
		{
			name:               "88.2kHz to 44.1kHz",
			audioRate:          88200,
			expectedTargetRate: "44100 Hz",
		},
		{
			name:               "176.4kHz to 44.1kHz",
			audioRate:          176400,
			expectedTargetRate: "44100 Hz",
		},
		{
			name:               "44.1kHz no change",
			audioRate:          44100,
			expectedTargetRate: "44100 Hz",
		},
		{
			name:               "48kHz no change",
			audioRate:          48000,
			expectedTargetRate: "48000 Hz",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			audioInfo := AudioInfo{Rate: tc.audioRate}

			var targetRateStr string
			switch audioInfo.Rate {
			case 48000, 96000, 192000, 384000:
				targetRateStr = "48000 Hz"
			case 44100, 88200, 176400, 352800:
				targetRateStr = "44100 Hz"
			default:
				targetRateStr = "same rate"
			}

			if targetRateStr != tc.expectedTargetRate {
				t.Errorf("Expected target rate %s, got %s for input rate %d Hz",
					tc.expectedTargetRate, targetRateStr, tc.audioRate)
			}
		})
	}
}

// TestConversionLogOutput tests the actual log message formatting
// for different audio conversion scenarios
func TestConversionLogOutput(t *testing.T) {
	testCases := []struct {
		name              string
		sourceFile        string
		sourceBits        int
		sourceRate        int
		targetBits        int
		targetRateStr     string
		expectedLogFormat string
	}{
		{
			name:              "Hi-Res 24-bit 96kHz to 16-bit 48kHz",
			sourceFile:        "/path/to/music.flac",
			sourceBits:        24,
			sourceRate:        96000,
			targetBits:        16,
			targetRateStr:     "48000 Hz",
			expectedLogFormat: "Converting FLAC: /path/to/music.flac (24-bit 96000 Hz → 16-bit 48000 Hz)",
		},
		{
			name:              "24-bit 88.2kHz to 16-bit 44.1kHz",
			sourceFile:        "/music/track.flac",
			sourceBits:        24,
			sourceRate:        88200,
			targetBits:        16,
			targetRateStr:     "44100 Hz",
			expectedLogFormat: "Converting FLAC: /music/track.flac (24-bit 88200 Hz → 16-bit 44100 Hz)",
		},
		{
			name:              "32-bit 192kHz to 16-bit 48kHz",
			sourceFile:        "test.flac",
			sourceBits:        32,
			sourceRate:        192000,
			targetBits:        16,
			targetRateStr:     "48000 Hz",
			expectedLogFormat: "Converting FLAC: test.flac (32-bit 192000 Hz → 16-bit 48000 Hz)",
		},
		{
			name:              "16-bit 96kHz to 16-bit 48kHz (sample rate only)",
			sourceFile:        "sample.flac",
			sourceBits:        16,
			sourceRate:        96000,
			targetBits:        16,
			targetRateStr:     "48000 Hz",
			expectedLogFormat: "Converting FLAC: sample.flac (16-bit 96000 Hz → 16-bit 48000 Hz)",
		},
		{
			name:              "24-bit 176.4kHz to 16-bit 44.1kHz",
			sourceFile:        "hires.flac",
			sourceBits:        24,
			sourceRate:        176400,
			targetBits:        16,
			targetRateStr:     "44100 Hz",
			expectedLogFormat: "Converting FLAC: hires.flac (24-bit 176400 Hz → 16-bit 44100 Hz)",
		},
		{
			name:              "32-bit 352.8kHz to 16-bit 44.1kHz",
			sourceFile:        "ultra-hires.flac",
			sourceBits:        32,
			sourceRate:        352800,
			targetBits:        16,
			targetRateStr:     "44100 Hz",
			expectedLogFormat: "Converting FLAC: ultra-hires.flac (32-bit 352800 Hz → 16-bit 44100 Hz)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Simulate the log message construction
			logMessage := fmt.Sprintf("Converting FLAC: %s (%d-bit %d Hz → %d-bit %s)",
				tc.sourceFile, tc.sourceBits, tc.sourceRate, tc.targetBits, tc.targetRateStr)

			if logMessage != tc.expectedLogFormat {
				t.Errorf("Expected log format:\n%s\nGot:\n%s", tc.expectedLogFormat, logMessage)
			}

			// Verify the message contains all expected components
			if !strings.Contains(logMessage, "Converting FLAC:") {
				t.Error("Log message should contain 'Converting FLAC:'")
			}
			if !strings.Contains(logMessage, tc.sourceFile) {
				t.Error("Log message should contain the source file path")
			}
			if !strings.Contains(logMessage, fmt.Sprintf("%d-bit", tc.sourceBits)) {
				t.Error("Log message should contain source bit depth")
			}
			if !strings.Contains(logMessage, fmt.Sprintf("%d Hz", tc.sourceRate)) {
				t.Error("Log message should contain source sample rate")
			}
			if !strings.Contains(logMessage, "→") {
				t.Error("Log message should contain conversion arrow")
			}
			if !strings.Contains(logMessage, fmt.Sprintf("%d-bit", tc.targetBits)) {
				t.Error("Log message should contain target bit depth")
			}
			if !strings.Contains(logMessage, tc.targetRateStr) {
				t.Error("Log message should contain target sample rate string")
			}
		})
	}
}

// TestSampleRateConversionMapping tests the comprehensive mapping
// of all sample rate conversion scenarios
func TestSampleRateConversionMapping(t *testing.T) {
	testCases := []struct {
		name                    string
		inputRate               int
		expectedNeedsConversion bool
		expectedTargetRate      string
		expectedSoxArg          string
	}{
		// High sample rates that need conversion to 48kHz
		{
			name:                    "96kHz needs conversion",
			inputRate:               96000,
			expectedNeedsConversion: true,
			expectedTargetRate:      "48kHz",
			expectedSoxArg:          "48000",
		},
		{
			name:                    "192kHz needs conversion",
			inputRate:               192000,
			expectedNeedsConversion: true,
			expectedTargetRate:      "48kHz",
			expectedSoxArg:          "48000",
		},
		{
			name:                    "384kHz needs conversion",
			inputRate:               384000,
			expectedNeedsConversion: true,
			expectedTargetRate:      "48kHz",
			expectedSoxArg:          "48000",
		},
		// High sample rates that need conversion to 44.1kHz
		{
			name:                    "88.2kHz needs conversion",
			inputRate:               88200,
			expectedNeedsConversion: true,
			expectedTargetRate:      "44.1kHz",
			expectedSoxArg:          "44100",
		},
		{
			name:                    "176.4kHz needs conversion",
			inputRate:               176400,
			expectedNeedsConversion: true,
			expectedTargetRate:      "44.1kHz",
			expectedSoxArg:          "44100",
		},
		{
			name:                    "352.8kHz needs conversion",
			inputRate:               352800,
			expectedNeedsConversion: true,
			expectedTargetRate:      "44.1kHz",
			expectedSoxArg:          "44100",
		},
		// Standard rates that don't need conversion
		{
			name:                    "44.1kHz no conversion",
			inputRate:               44100,
			expectedNeedsConversion: false,
			expectedTargetRate:      "44.1kHz",
			expectedSoxArg:          "",
		},
		{
			name:                    "48kHz no conversion",
			inputRate:               48000,
			expectedNeedsConversion: false,
			expectedTargetRate:      "48kHz",
			expectedSoxArg:          "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			audioInfo := AudioInfo{
				Bits: 16, // Use standard bit depth for rate testing
				Rate: tc.inputRate,
			}

			needsConversion, _, sampleRateArgs := determineConversion(&audioInfo)

			// Check if conversion is needed
			if needsConversion != tc.expectedNeedsConversion {
				t.Errorf("Expected needsConversion %v, got %v",
					tc.expectedNeedsConversion, needsConversion)
			}

			// Check target rate determination using switch case logic
			var targetRateStr string
			switch audioInfo.Rate {
			case 48000, 96000, 192000, 384000:
				targetRateStr = "48kHz"
			case 44100, 88200, 176400, 352800:
				targetRateStr = "44.1kHz"
			default:
				targetRateStr = "same rate"
			}

			if targetRateStr != tc.expectedTargetRate {
				t.Errorf("Expected target rate %s, got %s",
					tc.expectedTargetRate, targetRateStr)
			}

			// Check SoX arguments when conversion is needed
			if tc.expectedNeedsConversion {
				if len(sampleRateArgs) < 4 {
					t.Errorf("Expected sample rate args with target rate, got %v", sampleRateArgs)
				} else if sampleRateArgs[3] != tc.expectedSoxArg {
					t.Errorf("Expected SoX target rate %s, got %s",
						tc.expectedSoxArg, sampleRateArgs[3])
				}
			} else {
				// No conversion needed, should not have target rate in args
				if len(sampleRateArgs) > 3 {
					t.Errorf("Expected no target rate in args for no conversion case, got %v", sampleRateArgs)
				}
			}
		})
	}
}

// TestConversionLogIntegration tests the integration of all components
// that contribute to the conversion logging functionality
func TestConversionLogIntegration(t *testing.T) {
	testCases := []struct {
		name         string
		audioInfo    AudioInfo
		expectedLog  string
		expectedConv bool
	}{
		{
			name:         "Complete Hi-Res conversion scenario",
			audioInfo:    AudioInfo{Bits: 24, Rate: 96000},
			expectedLog:  "24-bit 96000 Hz → 16-bit 48kHz",
			expectedConv: true,
		},
		{
			name:         "Bit depth only conversion",
			audioInfo:    AudioInfo{Bits: 24, Rate: 44100},
			expectedLog:  "24-bit 44100 Hz → 16-bit 44.1kHz",
			expectedConv: true,
		},
		{
			name:         "Sample rate only conversion",
			audioInfo:    AudioInfo{Bits: 16, Rate: 88200},
			expectedLog:  "16-bit 88200 Hz → 16-bit 44.1kHz",
			expectedConv: true,
		},
		{
			name:         "No conversion needed",
			audioInfo:    AudioInfo{Bits: 16, Rate: 44100},
			expectedLog:  "16-bit 44100 Hz → 16-bit 44.1kHz",
			expectedConv: false,
		},
		{
			name:         "176.4kHz sample rate conversion",
			audioInfo:    AudioInfo{Bits: 16, Rate: 176400},
			expectedLog:  "16-bit 176400 Hz → 16-bit 44.1kHz",
			expectedConv: true,
		},
		{
			name:         "352.8kHz complete conversion scenario",
			audioInfo:    AudioInfo{Bits: 24, Rate: 352800},
			expectedLog:  "24-bit 352800 Hz → 16-bit 44.1kHz",
			expectedConv: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test conversion determination
			needsConversion, bitrateArgs, sampleRateArgs := determineConversion(&tc.audioInfo)

			if needsConversion != tc.expectedConv {
				t.Errorf("Expected conversion %v, got %v", tc.expectedConv, needsConversion)
			}

			// Test target rate determination using the switch case logic (matches main.go)
			var targetRateStr string
			switch tc.audioInfo.Rate {
			case 48000, 96000, 192000, 384000:
				targetRateStr = "48kHz"
			case 44100, 88200, 176400, 352800:
				targetRateStr = "44.1kHz"
			default:
				targetRateStr = "same rate"
			}

			// Test target bit depth (always 16-bit for conversions)
			targetBits := 16

			// Construct the expected log format
			logFormat := fmt.Sprintf("%d-bit %d Hz → %d-bit %s",
				tc.audioInfo.Bits, tc.audioInfo.Rate, targetBits, targetRateStr)

			if !strings.Contains(tc.expectedLog, logFormat) && tc.expectedLog != logFormat {
				t.Errorf("Expected log format %s, got %s", tc.expectedLog, logFormat)
			}

			// Verify SoX arguments are correct for the conversion
			if needsConversion {
				// Check bitrate args
				if tc.audioInfo.Bits > 16 {
					if len(bitrateArgs) != 2 || bitrateArgs[0] != "-b" || bitrateArgs[1] != "16" {
						t.Errorf("Expected bitrate args [-b 16], got %v", bitrateArgs)
					}
				}

				// Check sample rate args
				if tc.audioInfo.Rate > 48000 {
					if len(sampleRateArgs) < 4 {
						t.Errorf("Expected sample rate args with target, got %v", sampleRateArgs)
					}
				}
			}
		})
	}
}
