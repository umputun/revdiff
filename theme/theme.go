// Package theme provides theme file parsing and serialization for revdiff color palettes.
// Theme files use INI format with comment-based metadata (# name: ..., # description: ...).
package theme

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Theme represents a color theme with metadata and color key-value pairs.
type Theme struct {
	Name        string
	Description string
	ChromaStyle string
	Colors      map[string]string // keys include the "color-" prefix, matching ini-name tags exactly (e.g. "color-accent")
}

// colorKeys is the ordered list of all 21 recognized color keys matching ini-name tags.
var colorKeys = []string{
	"color-accent", "color-border", "color-normal", "color-muted",
	"color-selected-fg", "color-selected-bg",
	"color-annotation",
	"color-cursor-fg", "color-cursor-bg",
	"color-add-fg", "color-add-bg",
	"color-remove-fg", "color-remove-bg",
	"color-modify-fg", "color-modify-bg",
	"color-tree-bg", "color-diff-bg",
	"color-status-fg", "color-status-bg",
	"color-search-fg", "color-search-bg",
}

// ColorKeys returns the ordered list of recognized color key names.
func ColorKeys() []string {
	result := make([]string, len(colorKeys))
	copy(result, colorKeys)
	return result
}

// Parse reads a theme file from r and returns the parsed Theme.
// Theme files use INI-style key=value pairs and comment metadata lines:
//
//	# name: theme-name
//	# description: one-line description
//	chroma-style = dracula
//	color-accent = #bd93f9
func Parse(r io.Reader) (Theme, error) {
	t := Theme{Colors: make(map[string]string)}
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// comment metadata lines
		if comment, ok := strings.CutPrefix(line, "#"); ok {
			comment = strings.TrimSpace(comment)
			switch {
			case strings.HasPrefix(comment, "name:"):
				t.Name = strings.TrimSpace(strings.TrimPrefix(comment, "name:"))
			case strings.HasPrefix(comment, "description:"):
				t.Description = strings.TrimSpace(strings.TrimPrefix(comment, "description:"))
			}
			continue
		}

		// key = value pairs
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			return Theme{}, fmt.Errorf("malformed line: %q", line)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		if key == "chroma-style" {
			t.ChromaStyle = val
			continue
		}
		if strings.HasPrefix(key, "color-") {
			t.Colors[key] = val
		}
	}

	if err := scanner.Err(); err != nil {
		return Theme{}, fmt.Errorf("reading theme: %w", err)
	}

	// validate that chroma-style is present
	if t.ChromaStyle == "" {
		return Theme{}, errors.New("theme missing required key: chroma-style")
	}

	// validate that all required color keys are present
	var missing []string
	for _, key := range colorKeys {
		if _, ok := t.Colors[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return Theme{}, fmt.Errorf("theme missing required keys: %s", strings.Join(missing, ", "))
	}

	return t, nil
}

// Dump writes a theme file to w from the given Theme.
// Colors are written in the canonical order defined by ColorKeys().
func Dump(t Theme, w io.Writer) error {
	var b strings.Builder

	if t.Name != "" {
		fmt.Fprintf(&b, "# name: %s\n", t.Name)
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "# description: %s\n", t.Description)
	}
	if t.Name != "" || t.Description != "" {
		b.WriteString("\n")
	}

	if t.ChromaStyle != "" {
		fmt.Fprintf(&b, "chroma-style = %s\n", t.ChromaStyle)
	}

	// write color keys in canonical order
	for _, key := range colorKeys {
		if val, ok := t.Colors[key]; ok {
			fmt.Fprintf(&b, "%s = %s\n", key, val)
		}
	}

	// write any extra keys not in the canonical list (sorted for stability)
	var extra []string
	for key := range t.Colors {
		if !isCanonicalKey(key) {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		fmt.Fprintf(&b, "%s = %s\n", key, t.Colors[key])
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing theme: %w", err)
	}
	return nil
}

// Load reads a theme file by name from the given themes directory.
// The file is expected at <themesDir>/<name>. Name must be a plain filename
// without path separators or directory traversal components.
func Load(name, themesDir string) (Theme, error) {
	if name != filepath.Base(name) || name == ".." || name == "." {
		return Theme{}, fmt.Errorf("invalid theme name %q: must be a plain filename", name)
	}
	fpath := filepath.Join(themesDir, name)
	f, err := os.Open(fpath) //nolint:gosec // theme path is validated above and constructed from config dir + name
	if err != nil {
		return Theme{}, fmt.Errorf("opening theme %q: %w", name, err)
	}
	defer f.Close()

	th, err := Parse(f)
	if err != nil {
		return Theme{}, fmt.Errorf("parsing theme %q: %w", name, err)
	}
	return th, nil
}

// List returns sorted names of theme files in the given directory.
// Returns an empty list if the directory does not exist.
func List(themesDir string) ([]string, error) {
	entries, err := os.ReadDir(themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading themes dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// InitBundled writes bundled theme files to the given directory, creating it if needed.
// Always overwrites files matching bundled theme names; does not touch user-added files.
func InitBundled(themesDir string) error {
	if err := os.MkdirAll(themesDir, 0o750); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}

	for name, content := range bundledThemes {
		fpath := filepath.Join(themesDir, name)
		if err := os.WriteFile(fpath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("writing bundled theme %q: %w", name, err)
		}
	}
	return nil
}

// BundledNames returns the sorted list of bundled theme names.
func BundledNames() []string {
	names := make([]string, 0, len(bundledThemes))
	for name := range bundledThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isCanonicalKey checks if a key is in the canonical colorKeys list.
func isCanonicalKey(key string) bool {
	return slices.Contains(colorKeys, key)
}
