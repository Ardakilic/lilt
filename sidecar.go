package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// sidecarImageNames lists supported conventional artwork names in selection
// priority order.
var sidecarImageNames = [...]string{
	"cover.jpg",
	"cover.png",
	"front.jpg",
	"front.png",
	"folder.jpg",
	"folder.png",
}

// resolveSidecarImage returns the highest-priority supported image beside audioPath.
// It searches only the containing directory, matches supported names without regard
// to case, accepts only regular files, and returns the path of the actual matching file.
func resolveSidecarImage(audioPath string) (string, bool, error) {
	dir := filepath.Dir(audioPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false, fmt.Errorf("read sidecar directory %q: %w", dir, err)
	}

	for _, candidate := range sidecarImageNames {
		for _, entry := range entries {
			if !strings.EqualFold(entry.Name(), candidate) {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				return "", false, fmt.Errorf("inspect sidecar candidate %q: %w", path, err)
			}
			if info.Mode().IsRegular() {
				return path, true, nil
			}
		}
	}

	return "", false, nil
}
