// Package theme provides theme file parsing and serialization for revdiff color palettes.
// Theme files use INI format with comment-based metadata (# name: ..., # description: ...).
package theme

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/umputun/revdiff/app/fsutil"
)

// DefaultThemeName is the name of the built-in default theme.
const DefaultThemeName = "revdiff"

// Theme represents a color theme with metadata and color key-value pairs.
type Theme struct {
	Name        string
	Description string
	Author      string
	Bundled     bool
	ChromaStyle string
	Colors      map[string]string // keys include the "color-" prefix, matching ini-name tags exactly (e.g. "color-accent")
}

// Clone returns a deep copy of the theme so callers can mutate it safely.
func (t Theme) Clone() Theme {
	clone := t
	if t.Colors != nil {
		clone.Colors = make(map[string]string, len(t.Colors))
		maps.Copy(clone.Colors, t.Colors)
	}
	return clone
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

// optionalColorKeys lists color keys that may be omitted from theme files.
// these correspond to CLI flags with no default value (terminal background is used instead).
var optionalColorKeys = map[string]bool{
	"color-cursor-bg": true,
	"color-tree-bg":   true,
	"color-diff-bg":   true,
}

// ColorKeys returns the ordered list of recognized color key names.
func ColorKeys() []string {
	result := make([]string, len(colorKeys))
	copy(result, colorKeys)
	return result
}

// OptionalColorKeys returns the set of color keys that may be omitted from theme files.
// these correspond to CLI flags with no default value (terminal background is used instead).
func OptionalColorKeys() map[string]bool {
	result := make(map[string]bool, len(optionalColorKeys))
	maps.Copy(result, optionalColorKeys)
	return result
}

// validateHexColor checks that s is a valid 6-digit hex color (e.g. "#aabbcc").
func validateHexColor(s string) error {
	if len(s) != 7 || s[0] != '#' {
		return fmt.Errorf("invalid hex color %q: must be #RRGGBB format", s)
	}
	if _, err := hex.DecodeString(s[1:]); err != nil {
		return fmt.Errorf("invalid hex color %q: must be #RRGGBB format", s)
	}
	return nil
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
			case strings.HasPrefix(comment, "author:"):
				t.Author = strings.TrimSpace(strings.TrimPrefix(comment, "author:"))
			case strings.HasPrefix(comment, "bundled:"):
				t.Bundled = strings.TrimSpace(strings.TrimPrefix(comment, "bundled:")) == "true"
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
			if err := validateHexColor(val); err != nil {
				return Theme{}, fmt.Errorf("key %q: %w", key, err)
			}
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

	// validate that all required color keys are present (optional keys may be omitted)
	var missing []string
	for _, key := range colorKeys {
		if optionalColorKeys[key] {
			continue
		}
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

// Load reads a theme by name. It first checks the local themes directory for a
// user-customized copy, then falls back to the embedded gallery. Name must be a
// plain filename without path separators or directory traversal components.
func Load(name, themesDir string) (Theme, error) {
	if name != filepath.Base(name) || name == ".." || name == "." {
		return Theme{}, fmt.Errorf("invalid theme name %q: must be a plain filename", name)
	}

	// try local file first (user may have customized it)
	if themesDir != "" {
		fpath := filepath.Join(themesDir, name)
		f, err := os.Open(fpath) //nolint:gosec // theme path is validated above and constructed from config dir + name
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Theme{}, fmt.Errorf("opening theme %q: %w", name, err)
		}
		if err == nil {
			defer f.Close()
			th, parseErr := Parse(f)
			if parseErr != nil {
				return Theme{}, fmt.Errorf("parsing theme %q: %w", name, parseErr)
			}
			return th, nil
		}
	}

	// fall back to embedded gallery
	th, err := GalleryTheme(name)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q not found (checked %s and gallery)", name, themesDir)
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

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// InitBundled writes bundled theme files (marked bundled: true in gallery) to the given directory,
// creating it if needed. Always overwrites files matching bundled theme names; does not touch user-added files.
func InitBundled(themesDir string) error {
	gallery, err := Gallery()
	if err != nil {
		return fmt.Errorf("loading gallery: %w", err)
	}

	if err := os.MkdirAll(themesDir, 0o750); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}

	for name, th := range gallery {
		if !th.Bundled {
			continue
		}
		if err := writeThemeFile(themesDir, name, th); err != nil {
			return err
		}
	}
	return nil
}

// InitAll writes all gallery themes to the given directory, creating it if needed.
// Always overwrites files matching gallery theme names; does not touch user-added files.
func InitAll(themesDir string) error {
	gallery, err := Gallery()
	if err != nil {
		return fmt.Errorf("loading gallery: %w", err)
	}

	if err := os.MkdirAll(themesDir, 0o750); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}

	for name, th := range gallery {
		if err := writeThemeFile(themesDir, name, th); err != nil {
			return err
		}
	}
	return nil
}

// InitNames writes specific named themes from the gallery to the given directory.
// Returns an error if any name is not found in the gallery.
func InitNames(themesDir string, names []string) error {
	gallery, err := Gallery()
	if err != nil {
		return fmt.Errorf("loading gallery: %w", err)
	}

	if err := os.MkdirAll(themesDir, 0o750); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}

	for _, name := range names {
		th, ok := gallery[name]
		if !ok {
			return fmt.Errorf("theme %q not found in gallery", name)
		}
		if err := writeThemeFile(themesDir, name, th); err != nil {
			return err
		}
	}
	return nil
}

// ChromaValidator checks whether a chroma style name is valid.
// Returns true if the style is known.
type ChromaValidator func(styleName string) bool

// InstallFile reads a theme from a local file path, validates it, and writes it
// to the themes directory using the file's base name. Returns the installed name.
// If validateChroma is non-nil, it is called to verify the theme's chroma-style
// refers to a known Chroma style.
func InstallFile(themesDir, filePath string, validateChroma ChromaValidator) (string, error) {
	name := filepath.Base(filePath)
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid theme file name %q", name)
	}

	f, err := os.Open(filePath) //nolint:gosec // path comes from user CLI flag
	if err != nil {
		return "", fmt.Errorf("opening theme file %q: %w", filePath, err)
	}
	defer f.Close()

	th, err := Parse(f)
	if err != nil {
		return "", fmt.Errorf("parsing theme file %q: %w", filePath, err)
	}
	if th.Name == "" {
		th.Name = name
	} else if th.Name != name {
		return "", fmt.Errorf("theme metadata name %q does not match file name %q", th.Name, name)
	}

	if validateChroma != nil && !validateChroma(th.ChromaStyle) {
		return "", fmt.Errorf("theme %q references unknown chroma style %q", name, th.ChromaStyle)
	}

	if err := os.MkdirAll(themesDir, 0o750); err != nil {
		return "", fmt.Errorf("creating themes dir: %w", err)
	}
	if err := writeThemeFile(themesDir, name, th); err != nil {
		return "", err
	}
	return name, nil
}

// Install installs themes from gallery names or local file paths.
// Validates all gallery names upfront before performing any installs to prevent partial state.
func Install(args []string, themesDir string, validateChroma ChromaValidator, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	var galleryNames []string
	var localPaths []string

	for _, arg := range args {
		if IsLocalPath(arg) {
			localPaths = append(localPaths, arg)
			continue
		}
		galleryNames = append(galleryNames, arg)
	}

	for _, name := range galleryNames {
		if _, err := GalleryTheme(name); err != nil {
			return fmt.Errorf("theme %q not found in gallery", name)
		}
	}

	var installed int
	for _, path := range localPaths {
		name, err := InstallFile(themesDir, path, validateChroma)
		if err != nil {
			return fmt.Errorf("installing from file: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "installed %s from %s\n", name, path)
		installed++
	}

	if len(galleryNames) > 0 {
		if err := InitNames(themesDir, galleryNames); err != nil {
			return fmt.Errorf("installing from gallery: %w", err)
		}
		for _, name := range galleryNames {
			_, _ = fmt.Fprintf(stdout, "installed %s from gallery\n", name)
		}
		installed += len(galleryNames)
	}

	_, _ = fmt.Fprintf(stdout, "%d theme(s) installed to %s\n", installed, themesDir)
	return nil
}

// IsLocalPath returns true if the argument looks like a file path.
func IsLocalPath(s string) bool {
	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, filepath.Separator)
}

// PrintList prints all known theme names, one per line.
// Order: default theme first, then local, then bundled, then other gallery — sorted within each group.
func PrintList(themesDir string, w io.Writer) error {
	infos, err := ListOrdered(themesDir)
	if err != nil {
		return fmt.Errorf("listing themes: %w", err)
	}

	for _, info := range infos {
		_, _ = fmt.Fprintln(w, info.Name)
	}
	return nil
}

// ActiveName returns the theme name, defaulting to the built-in default when empty.
func ActiveName(name string) string {
	if name == "" {
		return DefaultThemeName
	}
	return name
}

// writeThemeFile dumps a theme to a file in the given directory.
func writeThemeFile(themesDir, name string, th Theme) error {
	var buf strings.Builder
	if err := Dump(th, &buf); err != nil {
		return fmt.Errorf("dumping theme %q: %w", name, err)
	}
	fpath := filepath.Join(themesDir, name)
	if err := fsutil.AtomicWriteFile(fpath, []byte(buf.String())); err != nil {
		return fmt.Errorf("writing theme %q: %w", name, err)
	}
	return nil
}


// BundledNames returns the sorted list of bundled theme names (those marked bundled: true in gallery).
func BundledNames() []string {
	gallery, err := Gallery()
	if err != nil {
		log.Printf("[ERROR] failed to load theme gallery: %v", err)
		return nil
	}
	names := make([]string, 0, len(gallery))
	for name, th := range gallery {
		if th.Bundled {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// ThemeInfo holds classification metadata for a theme in an ordered listing.
type ThemeInfo struct {
	Name      string
	InGallery bool // true if name exists in the embedded gallery
	Bundled   bool // true if gallery entry has bundled: true
	Local     bool // true if installed locally but not in gallery
}

// ListOrdered merges gallery and locally installed themes into a classified,
// deterministic list. Order: default theme first, then local-only (sorted),
// then bundled gallery (sorted), then other gallery (sorted).
func ListOrdered(themesDir string) ([]ThemeInfo, error) {
	gallery, err := Gallery()
	if err != nil {
		return nil, fmt.Errorf("loading gallery: %w", err)
	}

	installed, err := List(themesDir)
	if err != nil {
		return nil, fmt.Errorf("listing installed themes: %w", err)
	}

	// merge all names
	allNames := make(map[string]bool)
	for name := range gallery {
		allNames[name] = true
	}
	for _, name := range installed {
		allNames[name] = true
	}

	// classify into groups
	var defaultInfo *ThemeInfo
	var locals, bundled, other []ThemeInfo

	for name := range allNames {
		gth, inGallery := gallery[name]
		local := !inGallery
		info := ThemeInfo{
			Name:      name,
			InGallery: inGallery,
			Bundled:   inGallery && gth.Bundled,
			Local:     local,
		}
		switch {
		case name == DefaultThemeName:
			defaultInfo = &info
		case local:
			locals = append(locals, info)
		case gth.Bundled:
			bundled = append(bundled, info)
		default:
			other = append(other, info)
		}
	}

	sortInfos := func(s []ThemeInfo) {
		sort.Slice(s, func(i, j int) bool { return s[i].Name < s[j].Name })
	}
	sortInfos(locals)
	sortInfos(bundled)
	sortInfos(other)

	result := make([]ThemeInfo, 0, len(allNames))
	if defaultInfo != nil {
		result = append(result, *defaultInfo)
	}
	result = append(result, locals...)
	result = append(result, bundled...)
	result = append(result, other...)
	return result, nil
}

// isCanonicalKey checks if a key is in the canonical colorKeys list.
func isCanonicalKey(key string) bool {
	return slices.Contains(colorKeys, key)
}
