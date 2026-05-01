package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// validateCompareFlag checks that --compare-old/--compare-new are well-formed
// and not in conflict with other flags. Returns the absolute paths for both
// sides; callers stash them on opts so resolution runs once.
//
// Conflict checks run before stat checks so a misuse error is not masked by a
// missing-file error.
func validateCompareFlag(opts options) (oldPath, newPath string, err error) {
	if opts.CompareOld == "" && opts.CompareNew == "" {
		return "", "", nil
	}
	if opts.CompareOld == "" || opts.CompareNew == "" {
		return "", "", errors.New("--compare-old and --compare-new must be used together")
	}
	if opts.Refs.Base != "" || opts.Refs.Against != "" {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with refs")
	}
	if opts.Staged {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --staged")
	}
	if len(opts.Only) > 0 {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --only")
	}
	if opts.AllFiles {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --all-files")
	}
	if opts.Stdin {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --stdin")
	}
	if len(opts.Include) > 0 {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --include")
	}
	if len(opts.Exclude) > 0 {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --exclude")
	}
	if opts.Annotations != "" {
		return "", "", errors.New("--compare-old/--compare-new cannot be used with --annotations")
	}
	oldInfo, err := os.Stat(opts.CompareOld)
	if err != nil {
		return "", "", fmt.Errorf("--compare-old: %w", err)
	}
	if !oldInfo.Mode().IsRegular() {
		return "", "", errors.New("--compare-old must be a regular file")
	}
	newInfo, err := os.Stat(opts.CompareNew)
	if err != nil {
		return "", "", fmt.Errorf("--compare-new: %w", err)
	}
	if !newInfo.Mode().IsRegular() {
		return "", "", errors.New("--compare-new must be a regular file")
	}
	absOld, err := filepath.Abs(opts.CompareOld)
	if err != nil {
		return "", "", fmt.Errorf("resolve --compare-old: %w", err)
	}
	absNew, err := filepath.Abs(opts.CompareNew)
	if err != nil {
		return "", "", fmt.Errorf("resolve --compare-new: %w", err)
	}
	return absOld, absNew, nil
}
