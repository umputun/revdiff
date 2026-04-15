package theme

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/umputun/revdiff/app/fsutil"
	"github.com/umputun/revdiff/themes"
)

// themeInfo holds classification metadata for a theme in an ordered listing.
type themeInfo struct {
	Name      string
	InGallery bool // true if name exists in the embedded gallery
	Bundled   bool // true if gallery entry has bundled: true
	Local     bool // true if installed locally but not in gallery
}

const galleryDir = "gallery"

var galleryLoad struct {
	once   sync.Once
	themes map[string]Theme
	err    error
}

// Catalog provides theme discovery, loading, and management from the themes directory.
type Catalog struct {
	themesDir string
}

// NewCatalog creates a Catalog for the given themes directory.
func NewCatalog(themesDir string) *Catalog {
	return &Catalog{themesDir: themesDir}
}

// Dir returns the themes directory path.
func (c *Catalog) Dir() string {
	return c.themesDir
}

// Resolve loads a theme by name and returns the parsed Theme and whether it was found.
func (c *Catalog) Resolve(name string) (Theme, bool) {
	th, err := c.Load(name)
	if err != nil {
		return Theme{}, false
	}
	return th, true
}

// Load reads a theme by name. It first checks the local themes directory for a
// user-customized copy, then falls back to the embedded gallery. Name must be a
// plain filename without path separators or directory traversal components.
func (c *Catalog) Load(name string) (Theme, error) {
	if name != filepath.Base(name) || name == ".." || name == "." {
		return Theme{}, fmt.Errorf("invalid theme name %q: must be a plain filename", name)
	}

	// try local file first (user may have customized it)
	if c.themesDir != "" {
		fpath := filepath.Join(c.themesDir, name)
		f, err := os.Open(fpath) //nolint:gosec // theme path is validated above and constructed from config dir + name
		switch {
		case err == nil:
			defer f.Close()
			th, parseErr := c.parse(f)
			if parseErr != nil {
				return Theme{}, fmt.Errorf("parsing theme %q: %w", name, parseErr)
			}
			return th, nil
		case !errors.Is(err, os.ErrNotExist):
			return Theme{}, fmt.Errorf("opening theme %q: %w", name, err)
		}
	}

	// fall back to embedded gallery
	th, err := c.galleryTheme(name)
	if err != nil {
		return Theme{}, fmt.Errorf("theme %q not found (checked %s and gallery)", name, c.themesDir)
	}
	return th, nil
}

// Entries returns the ordered list of available themes with classification metadata.
// order: default theme first, then local-only (sorted), then bundled gallery (sorted),
// then other gallery (sorted).
func (c *Catalog) Entries() ([]themeInfo, error) {
	gal, err := c.gallery()
	if err != nil {
		return nil, fmt.Errorf("loading gallery: %w", err)
	}

	installed, err := c.list()
	if err != nil {
		return nil, fmt.Errorf("listing installed themes: %w", err)
	}

	// merge all names
	allNames := make(map[string]bool)
	for name := range gal {
		allNames[name] = true
	}
	for _, name := range installed {
		allNames[name] = true
	}

	// classify into groups
	var defaultInfo *themeInfo
	var locals, bundled, other []themeInfo

	for name := range allNames {
		gth, inGallery := gal[name]
		info := themeInfo{
			Name:      name,
			InGallery: inGallery,
			Bundled:   inGallery && gth.Bundled,
			Local:     !inGallery,
		}
		switch {
		case name == defaultThemeName:
			defaultInfo = &info
		case !inGallery:
			locals = append(locals, info)
		case gth.Bundled:
			bundled = append(bundled, info)
		default:
			other = append(other, info)
		}
	}

	sortInfos := func(s []themeInfo) {
		slices.SortFunc(s, func(a, b themeInfo) int { return strings.Compare(a.Name, b.Name) })
	}
	sortInfos(locals)
	sortInfos(bundled)
	sortInfos(other)

	result := make([]themeInfo, 0, len(allNames))
	if defaultInfo != nil {
		result = append(result, *defaultInfo)
	}
	result = append(result, locals...)
	result = append(result, bundled...)
	result = append(result, other...)
	return result, nil
}

// InitBundled writes bundled theme files (marked bundled: true in gallery) to the themes directory,
// creating it if needed. Always overwrites files matching bundled theme names; does not touch user-added files.
func (c *Catalog) InitBundled() error {
	return c.initThemes(nil, func(_ string, th Theme) bool { return th.Bundled })
}

// InitAll writes all gallery themes to the themes directory, creating it if needed.
// always overwrites files matching gallery theme names; does not touch user-added files.
func (c *Catalog) InitAll() error {
	return c.initThemes(nil, nil)
}

// Install installs themes from gallery names or local file paths.
// validates all gallery names upfront before performing any installs to prevent partial state.
func (c *Catalog) Install(args []string, validateChroma func(string) bool, stdout io.Writer) error {
	if stdout == nil {
		stdout = io.Discard
	}

	var galleryNamesList []string
	var localPaths []string

	for _, arg := range args {
		if c.isLocalPath(arg) {
			localPaths = append(localPaths, arg)
			continue
		}
		galleryNamesList = append(galleryNamesList, arg)
	}

	for _, name := range galleryNamesList {
		if _, err := c.galleryTheme(name); err != nil {
			return fmt.Errorf("theme %q not found in gallery", name)
		}
	}

	var installed int
	for _, path := range localPaths {
		name, err := c.installFile(path, validateChroma)
		if err != nil {
			return fmt.Errorf("installing from file: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "installed %s from %s\n", name, path)
		installed++
	}

	if len(galleryNamesList) > 0 {
		if err := c.initNames(galleryNamesList); err != nil {
			return fmt.Errorf("installing from gallery: %w", err)
		}
		for _, name := range galleryNamesList {
			_, _ = fmt.Fprintf(stdout, "installed %s from gallery\n", name)
		}
		installed += len(galleryNamesList)
	}

	_, _ = fmt.Fprintf(stdout, "%d theme(s) installed to %s\n", installed, c.themesDir)
	return nil
}

// PrintList prints all known theme names, one per line.
// order: default theme first, then local, then bundled, then other gallery — sorted within each group.
func (c *Catalog) PrintList(w io.Writer) error {
	infos, err := c.Entries()
	if err != nil {
		return fmt.Errorf("listing themes: %w", err)
	}

	for _, info := range infos {
		_, _ = fmt.Fprintln(w, info.Name)
	}
	return nil
}

// ActiveName returns the theme name, defaulting to the built-in default when empty.
func (c *Catalog) ActiveName(name string) string {
	if name == "" {
		return defaultThemeName
	}
	return name
}

// OptionalColorKeys returns the set of color keys that may be omitted from theme files.
// these correspond to CLI flags with no default value (terminal background is used instead).
func (c *Catalog) OptionalColorKeys() map[string]bool {
	result := make(map[string]bool, len(optionalColorKeys))
	maps.Copy(result, optionalColorKeys)
	return result
}

// parse reads a theme file from r and returns the parsed Theme.
// theme files use INI-style key=value pairs and comment metadata lines:
//
//	# name: theme-name
//	# description: one-line description
//	chroma-style = dracula
//	color-accent = #bd93f9
func (c *Catalog) parse(r io.Reader) (Theme, error) {
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
			if err := c.validateHexColor(val); err != nil {
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

// list returns sorted names of theme files in the themes directory.
// returns an empty list if the directory does not exist.
func (c *Catalog) list() ([]string, error) {
	entries, err := os.ReadDir(c.themesDir)
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
	slices.Sort(names)
	return names, nil
}

// initNames writes specific named themes from the gallery to the themes directory.
// returns an error if any name is not found in the gallery.
func (c *Catalog) initNames(names []string) error {
	gal, err := c.gallery()
	if err != nil {
		return fmt.Errorf("loading gallery: %w", err)
	}

	// validate all names upfront before writing anything
	for _, name := range names {
		if _, ok := gal[name]; !ok {
			return fmt.Errorf("theme %q not found in gallery", name)
		}
	}

	return c.initThemes(gal, func(name string, _ Theme) bool {
		return slices.Contains(names, name)
	})
}

// initThemes creates themesDir and writes gallery themes matching the filter.
// when filter is nil, all themes are written. if gal is nil, it is loaded from embedded assets.
func (c *Catalog) initThemes(gal map[string]Theme, filter func(string, Theme) bool) error {
	if gal == nil {
		var err error
		if gal, err = c.gallery(); err != nil {
			return fmt.Errorf("loading gallery: %w", err)
		}
	}

	if err := os.MkdirAll(c.themesDir, 0o750); err != nil {
		return fmt.Errorf("creating themes dir: %w", err)
	}

	for name, th := range gal {
		if filter != nil && !filter(name, th) {
			continue
		}
		if err := c.writeThemeFile(name, th); err != nil {
			return err
		}
	}
	return nil
}

// installFile reads a theme from a local file path, validates it, and writes it
// to the themes directory using the file's base name. returns the installed name.
// if validateChroma is non-nil, it is called to verify the theme's chroma-style
// refers to a known Chroma style.
func (c *Catalog) installFile(filePath string, validateChroma func(string) bool) (string, error) {
	name := filepath.Base(filePath)
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid theme file name %q", name)
	}

	f, err := os.Open(filePath) //nolint:gosec // path comes from user CLI flag
	if err != nil {
		return "", fmt.Errorf("opening theme file %q: %w", filePath, err)
	}
	defer f.Close()

	th, err := c.parse(f)
	if err != nil {
		return "", fmt.Errorf("parsing theme file %q: %w", filePath, err)
	}
	switch {
	case th.Name == "":
		th.Name = name
	case th.Name != name:
		return "", fmt.Errorf("theme metadata name %q does not match file name %q", th.Name, name)
	}

	if validateChroma != nil && !validateChroma(th.ChromaStyle) {
		return "", fmt.Errorf("theme %q references unknown chroma style %q", name, th.ChromaStyle)
	}

	if err := os.MkdirAll(c.themesDir, 0o750); err != nil {
		return "", fmt.Errorf("creating themes dir: %w", err)
	}
	if err := c.writeThemeFile(name, th); err != nil {
		return "", err
	}
	return name, nil
}

// writeThemeFile dumps a theme to a file in the themes directory.
func (c *Catalog) writeThemeFile(name string, th Theme) error {
	var buf strings.Builder
	if err := th.Dump(&buf); err != nil {
		return fmt.Errorf("dumping theme %q: %w", name, err)
	}
	fpath := filepath.Join(c.themesDir, name)
	if err := fsutil.AtomicWriteFile(fpath, []byte(buf.String())); err != nil {
		return fmt.Errorf("writing theme %q: %w", name, err)
	}
	return nil
}

// bundledNames returns the sorted list of bundled theme names (those marked bundled: true in gallery).
func (c *Catalog) bundledNames() ([]string, error) {
	gal, err := c.gallery()
	if err != nil {
		return nil, fmt.Errorf("loading gallery: %w", err)
	}
	names := make([]string, 0, len(gal))
	for name, th := range gal {
		if th.Bundled {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// isLocalPath returns true if the argument looks like a file path.
func (c *Catalog) isLocalPath(s string) bool {
	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, filepath.Separator)
}

// validateHexColor checks that s is a valid 6-digit hex color (e.g. "#aabbcc").
func (c *Catalog) validateHexColor(s string) error {
	if len(s) != 7 || s[0] != '#' {
		return fmt.Errorf("invalid hex color %q: must be #RRGGBB format", s)
	}
	if _, err := hex.DecodeString(s[1:]); err != nil {
		return fmt.Errorf("invalid hex color %q: must be #RRGGBB format", s)
	}
	return nil
}

// gallery returns all themes from the embedded gallery, keyed by filename.
// results are cached after the first call since the embedded FS is immutable.
// each call returns a deep copy so callers can mutate the result safely.
func (c *Catalog) gallery() (map[string]Theme, error) {
	galleryLoad.once.Do(func() {
		galleryLoad.themes, galleryLoad.err = c.loadGallery()
	})
	if galleryLoad.err != nil {
		return nil, galleryLoad.err
	}
	return c.cloneGallery(galleryLoad.themes), nil
}

// loadGallery parses all theme files from the embedded gallery.
func (c *Catalog) loadGallery() (map[string]Theme, error) {
	entries, err := fs.ReadDir(themes.FS, galleryDir)
	if err != nil {
		return nil, fmt.Errorf("reading gallery: %w", err)
	}

	result := make(map[string]Theme, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(themes.FS, galleryDir+"/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("reading gallery theme %q: %w", e.Name(), err)
		}
		th, err := c.parse(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("parsing gallery theme %q: %w", e.Name(), err)
		}
		result[e.Name()] = th
	}
	return result, nil
}

// galleryNames returns sorted names of all themes in the embedded gallery.
func (c *Catalog) galleryNames() ([]string, error) {
	gal, err := c.gallery()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(gal))
	for name := range gal {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// galleryTheme returns a single theme from the embedded gallery by name.
func (c *Catalog) galleryTheme(name string) (Theme, error) {
	gal, err := c.gallery()
	if err != nil {
		return Theme{}, err
	}
	th, ok := gal[name]
	if !ok {
		return Theme{}, fmt.Errorf("gallery theme %q not found", name)
	}
	return th, nil
}

// cloneGallery returns a deep copy of the gallery map.
func (c *Catalog) cloneGallery(src map[string]Theme) map[string]Theme {
	dst := make(map[string]Theme, len(src))
	for name, th := range src {
		dst[name] = th.clone()
	}
	return dst
}
