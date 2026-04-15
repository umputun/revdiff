// Package theme provides theme file parsing and serialization for revdiff color palettes.
// Theme files use INI format with comment-based metadata (# name: ..., # description: ...).
package theme

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// defaultThemeName is the name of the built-in default theme.
const defaultThemeName = "revdiff"

// colorKeys is the ordered list of all 23 recognized color keys matching ini-name tags.
var colorKeys = []string{
	"color-accent", "color-border", "color-normal", "color-muted",
	"color-selected-fg", "color-selected-bg",
	"color-annotation",
	"color-cursor-fg", "color-cursor-bg",
	"color-add-fg", "color-add-bg",
	"color-remove-fg", "color-remove-bg",
	"color-word-add-bg", "color-word-remove-bg",
	"color-modify-fg", "color-modify-bg",
	"color-tree-bg", "color-diff-bg",
	"color-status-fg", "color-status-bg",
	"color-search-fg", "color-search-bg",
}

// optionalColorKeys lists color keys that may be omitted from theme files.
// these correspond to CLI flags with no default value (terminal background is used instead).
var optionalColorKeys = map[string]bool{
	"color-cursor-bg":      true,
	"color-tree-bg":        true,
	"color-diff-bg":        true,
	"color-word-add-bg":    true,
	"color-word-remove-bg": true,
}

// Theme represents a color theme with metadata and color key-value pairs.
type Theme struct {
	Name        string
	Description string
	Author      string
	Bundled     bool
	ChromaStyle string
	Colors      map[string]string // keys include the "color-" prefix, matching ini-name tags exactly (e.g. "color-accent")
}

// Dump writes a theme file to w from the given Theme.
// colors are written in the canonical order defined by colorKeys.
func (t Theme) Dump(w io.Writer) error {
	var b strings.Builder

	if t.Name != "" {
		fmt.Fprintf(&b, "# name: %s\n", t.Name)
	}
	if t.Description != "" {
		fmt.Fprintf(&b, "# description: %s\n", t.Description)
	}
	if t.Author != "" {
		fmt.Fprintf(&b, "# author: %s\n", t.Author)
	}
	if t.Bundled {
		b.WriteString("# bundled: true\n")
	}
	if t.Name != "" || t.Description != "" || t.Author != "" || t.Bundled {
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
		if !t.isCanonicalKey(key) {
			extra = append(extra, key)
		}
	}
	slices.Sort(extra)
	for _, key := range extra {
		fmt.Fprintf(&b, "%s = %s\n", key, t.Colors[key])
	}

	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("writing theme: %w", err)
	}
	return nil
}

// clone returns a deep copy of the theme so callers can mutate it safely.
func (t Theme) clone() Theme {
	c := t
	if t.Colors != nil {
		c.Colors = make(map[string]string, len(t.Colors))
		maps.Copy(c.Colors, t.Colors)
	}
	return c
}

// isCanonicalKey checks if a key is in the canonical colorKeys list.
func (t Theme) isCanonicalKey(key string) bool {
	return slices.Contains(colorKeys, key)
}
