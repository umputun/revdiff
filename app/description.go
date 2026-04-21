package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// descriptionMaxBytes caps the size of --description-file to prevent loading
// pathologically large files into the TUI. 1 MiB is far more than any
// hand-written or agent-generated description will ever need; crossing it is
// almost certainly a user error (wrong flag, pipe of the whole repo).
const descriptionMaxBytes = 1 << 20

// resolveDescription returns the final description string from opts, reading
// --description-file when set. Returns "" when neither flag is present.
// Surfaces clear errors for missing, oversized, or unreadable files.
func resolveDescription(opts options) (string, error) {
	if opts.Description != "" {
		return opts.Description, nil
	}
	if opts.DescriptionFile == "" {
		return "", nil
	}

	path, err := filepath.Abs(opts.DescriptionFile)
	if err != nil {
		return "", fmt.Errorf("resolve --description-file path: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("read --description-file: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("--description-file %q is a directory", path)
	}
	if info.Size() > descriptionMaxBytes {
		return "", fmt.Errorf("--description-file %q is %d bytes (max %d)", path, info.Size(), descriptionMaxBytes)
	}

	data, err := os.ReadFile(path) //nolint:gosec // path is user-provided via --description-file
	if err != nil {
		return "", fmt.Errorf("read --description-file: %w", err)
	}
	return string(data), nil
}
