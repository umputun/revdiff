package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/umputun/revdiff/app/fsutil"
)

// patchConfigTheme updates the theme setting in the INI config file.
// If a "theme = " line exists, it replaces the value. Otherwise appends it.
func patchConfigTheme(configPath, themeName string) error {
	if strings.ContainsAny(themeName, "\r\n") {
		return fmt.Errorf("invalid theme name %q: must not contain newlines", themeName)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o750); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := os.ReadFile(configPath) //nolint:gosec // path from user's config
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := fsutil.AtomicWriteFile(configPath, []byte("theme = "+themeName+"\n")); writeErr != nil {
				return fmt.Errorf("writing config: %w", writeErr)
			}
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		// match "theme = ..." or "theme=..." but not commented-out lines
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "theme" {
			lines[i] = "theme = " + themeName
			found = true
			break
		}
	}

	if !found {
		// append before trailing empty lines
		insertIdx := len(lines)
		for insertIdx > 0 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		lines = append(lines[:insertIdx], append([]string{"theme = " + themeName}, lines[insertIdx:]...)...)
	}

	if err := fsutil.AtomicWriteFile(configPath, []byte(strings.Join(lines, "\n"))); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
