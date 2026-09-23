package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveSidecarImage verifies deterministic same-directory sidecar selection.
func TestResolveSidecarImage(t *testing.T) {
	testCases := []struct {
		name             string
		audioDir         string
		audioFiles       []string
		images           []string
		nonRegularImages []string
		parentImages     []string
		expected         []string
		wantErr          bool
	}{
		{
			name:       "cover jpg has highest precedence",
			audioFiles: []string{"track.flac"},
			images:     []string{"folder.jpg", "cover.png", "front.jpg", "cover.jpg", "front.png", "folder.png"},
			expected:   []string{"cover.jpg"},
		},
		{
			name:       "cover png precedes front",
			audioFiles: []string{"track.flac"},
			images:     []string{"cover.png", "front.jpg", "front.png", "folder.jpg", "folder.png"},
			expected:   []string{"cover.png"},
		},
		{
			name:       "front jpg precedes front png and folder",
			audioFiles: []string{"track.flac"},
			images:     []string{"front.jpg", "front.png", "folder.jpg", "folder.png"},
			expected:   []string{"front.jpg"},
		},
		{
			name:       "front png precedes folder",
			audioFiles: []string{"track.flac"},
			images:     []string{"front.png", "folder.jpg", "folder.png"},
			expected:   []string{"front.png"},
		},
		{
			name:       "folder jpg precedes folder png",
			audioFiles: []string{"track.flac"},
			images:     []string{"folder.jpg", "folder.png"},
			expected:   []string{"folder.jpg"},
		},
		{
			name:       "folder png is final candidate",
			audioFiles: []string{"track.flac"},
			images:     []string{"folder.png"},
			expected:   []string{"folder.png"},
		},
		{
			name:       "cover jpg is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"CoVeR.JpG"},
			expected:   []string{"CoVeR.JpG"},
		},
		{
			name:       "cover png is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"COVER.PNG"},
			expected:   []string{"COVER.PNG"},
		},
		{
			name:       "front jpg is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"FrOnT.JpG"},
			expected:   []string{"FrOnT.JpG"},
		},
		{
			name:       "front png is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"FRONT.PnG"},
			expected:   []string{"FRONT.PnG"},
		},
		{
			name:       "folder jpg is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"FoLdEr.JpG"},
			expected:   []string{"FoLdEr.JpG"},
		},
		{
			name:       "folder png is case insensitive",
			audioFiles: []string{"track.flac"},
			images:     []string{"FOLDER.pNg"},
			expected:   []string{"FOLDER.pNg"},
		},
		{
			name:         "parent directory is not searched",
			audioDir:     "album",
			audioFiles:   []string{"track.flac"},
			parentImages: []string{"cover.jpg", "cover.png", "front.jpg", "front.png", "folder.jpg", "folder.png"},
			expected:     []string{""},
		},
		{
			name:       "unsupported extensions are ignored",
			audioFiles: []string{"track.flac"},
			images:     []string{"cover.jpeg", "cover.webp", "front.gif", "folder.bmp", "album.jpg"},
			expected:   []string{""},
		},
		{
			name:             "non-regular higher-priority candidate is ignored",
			audioFiles:       []string{"track.flac"},
			nonRegularImages: []string{"cover.jpg"},
			images:           []string{"front.png"},
			expected:         []string{"front.png"},
		},
		{
			name:             "non-regular only candidate is not selected",
			audioFiles:       []string{"track.flac"},
			nonRegularImages: []string{"cover.jpg", "folder.png"},
			expected:         []string{""},
		},
		{
			name:       "same sidecar is reused by every audio file",
			audioFiles: []string{"track01.flac", "track02.mp3", "track03.m4a"},
			images:     []string{"folder.jpg"},
			expected:   []string{"folder.jpg", "folder.jpg", "folder.jpg"},
		},
		{
			name:       "missing containing directory returns error",
			audioDir:   "missing",
			audioFiles: []string{"track.flac"},
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			audioDir := root
			if tc.audioDir != "" {
				audioDir = filepath.Join(root, tc.audioDir)
			}
			if !tc.wantErr {
				if err := os.MkdirAll(audioDir, 0o755); err != nil {
					t.Fatalf("create audio directory: %v", err)
				}
			}

			for _, name := range tc.parentImages {
				if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o644); err != nil {
					t.Fatalf("create parent image %q: %v", name, err)
				}
			}
			for _, name := range tc.images {
				if err := os.WriteFile(filepath.Join(audioDir, name), []byte(name), 0o644); err != nil {
					t.Fatalf("create image %q: %v", name, err)
				}
			}
			for _, name := range tc.nonRegularImages {
				if err := os.MkdirAll(filepath.Join(audioDir, name), 0o755); err != nil {
					t.Fatalf("create non-regular candidate %q: %v", name, err)
				}
			}

			for i, audioFile := range tc.audioFiles {
				audioPath := filepath.Join(audioDir, audioFile)
				if !tc.wantErr {
					if err := os.WriteFile(audioPath, []byte("audio"), 0o644); err != nil {
						t.Fatalf("create audio file %q: %v", audioFile, err)
					}
				}

				gotPath, gotOK, err := resolveSidecarImage(audioPath)
				if tc.wantErr {
					if err == nil {
						t.Fatalf("resolveSidecarImage(%q) returned no error", audioPath)
					}
					continue
				}
				if err != nil {
					t.Fatalf("resolveSidecarImage(%q) returned error: %v", audioPath, err)
				}

				wantOK := tc.expected[i] != ""
				if gotOK != wantOK {
					t.Errorf("resolveSidecarImage(%q) found = %v, want %v", audioPath, gotOK, wantOK)
				}
				wantPath := tc.expected[i]
				if wantOK {
					wantPath = filepath.Join(audioDir, wantPath)
				}
				if gotPath != wantPath {
					t.Errorf("resolveSidecarImage(%q) path = %q, want %q", audioPath, gotPath, wantPath)
				}
			}
		})
	}
}
