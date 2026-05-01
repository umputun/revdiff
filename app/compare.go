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
	conflicts := []struct {
		bad  bool
		flag string
	}{
		{opts.Refs.Base != "" || opts.Refs.Against != "", "refs"},
		{opts.Staged, "--staged"},
		{len(opts.Only) > 0, "--only"},
		{opts.AllFiles, "--all-files"},
		{opts.Stdin, "--stdin"},
		{len(opts.Include) > 0, "--include"},
		{len(opts.Exclude) > 0, "--exclude"},
		{opts.Annotations != "", "--annotations"},
	}
	for _, c := range conflicts {
		if c.bad {
			return "", "", fmt.Errorf("--compare-old/--compare-new cannot be used with %s", c.flag)
		}
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
