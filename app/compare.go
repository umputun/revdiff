package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// validateCompareFlag checks that --compare=old:new is well-formed and does not
// conflict with other mutually-exclusive flags.
func validateCompareFlag(opts options) error {
	if opts.Compare == "" {
		return nil
	}
	old, newPath, ok := strings.Cut(opts.Compare, ":")
	if !ok || old == "" || newPath == "" {
		return errors.New("--compare requires old:new format with a colon separator")
	}
	if opts.Refs.Base != "" || opts.Refs.Against != "" {
		return errors.New("--compare cannot be used with refs")
	}
	if opts.Staged {
		return errors.New("--compare cannot be used with --staged")
	}
	if len(opts.Only) > 0 {
		return errors.New("--compare cannot be used with --only")
	}
	if opts.AllFiles {
		return errors.New("--compare cannot be used with --all-files")
	}
	if opts.Stdin {
		return errors.New("--compare cannot be used with --stdin")
	}
	if len(opts.Include) > 0 {
		return errors.New("--compare cannot be used with --include")
	}
	if len(opts.Exclude) > 0 {
		return errors.New("--compare cannot be used with --exclude")
	}
	if opts.Annotations != "" {
		return errors.New("--compare cannot be used with --annotations")
	}
	oldInfo, err := os.Stat(old)
	if err != nil {
		return fmt.Errorf("--compare old path: %w", err)
	}
	if !oldInfo.Mode().IsRegular() {
		return errors.New("--compare old path must be a regular file")
	}
	newInfo, err := os.Stat(newPath)
	if err != nil {
		return fmt.Errorf("--compare new path: %w", err)
	}
	if !newInfo.Mode().IsRegular() {
		return errors.New("--compare new path must be a regular file")
	}
	return nil
}

// resolveComparePaths splits the --compare flag value on ":" and returns
// absolute paths for both sides.
func resolveComparePaths(compareFlag string) (absOld, absNew string, err error) {
	old, newPath, _ := strings.Cut(compareFlag, ":")
	absOld, err = filepath.Abs(old)
	if err != nil {
		return "", "", fmt.Errorf("resolve --compare old path: %w", err)
	}
	absNew, err = filepath.Abs(newPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve --compare new path: %w", err)
	}
	return absOld, absNew, nil
}
