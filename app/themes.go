package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/umputun/revdiff/app/fsutil"
	"github.com/umputun/revdiff/app/highlight"
	"github.com/umputun/revdiff/app/theme"
	"github.com/umputun/revdiff/app/ui"
	"github.com/umputun/revdiff/app/ui/style"
)

// defaultThemesDir returns ~/.config/revdiff/themes.
func defaultThemesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "themes")
}

// handleThemes processes theme-related flags: auto-init on first run, --init-themes, --init-all-themes,
// --install-theme, --list-themes, and --theme.
// returns (true, nil) when the caller should exit successfully, (false, error) on failure, (false, nil) to continue.
func handleThemes(opts *options, cat *theme.Catalog, stdout, stderr io.Writer) (bool, error) {
	themesDir := cat.Dir()

	// auto-init bundled themes on first run (silent, no error on failure)
	if themesDir != "" && len(opts.InstallTheme) == 0 {
		if _, err := os.Stat(themesDir); os.IsNotExist(err) {
			_ = cat.InitBundled()
		}
	}

	if opts.InitThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := cat.InitBundled(); err != nil {
			return false, fmt.Errorf("init themes: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "bundled themes written to %s\n", themesDir)
		return true, nil
	}

	if opts.InitAllThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := cat.InitAll(); err != nil {
			return false, fmt.Errorf("init all themes: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "all gallery themes written to %s\n", themesDir)
		return true, nil
	}

	if len(opts.InstallTheme) > 0 {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := cat.Install(opts.InstallTheme, highlight.IsValidStyle, stdout); err != nil {
			return false, fmt.Errorf("install theme: %w", err)
		}
		return true, nil
	}

	if opts.ListThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := cat.PrintList(stdout); err != nil {
			return false, fmt.Errorf("list themes: %w", err)
		}
		return true, nil
	}

	if opts.Theme == "" {
		return false, nil
	}

	// apply theme — overwrites present color fields and clears absent optional ones.
	// theme takes over completely, ignoring any --color-* flags, env vars, or --no-colors.
	if opts.NoColors {
		_, _ = fmt.Fprintln(stderr, "warning: --no-colors ignored when --theme is set")
	}
	resolveThemeConflicts(opts)
	if themesDir == "" {
		return false, errors.New("cannot determine home directory for themes")
	}
	th, err := cat.Load(opts.Theme)
	if err != nil {
		return false, fmt.Errorf("load theme: %w", err)
	}
	applyTheme(opts, th, cat)
	return false, nil
}

// resolveThemeConflicts clears NoColors when a theme is set, since theme takes over completely.
func resolveThemeConflicts(opts *options) {
	if opts.Theme != "" && opts.NoColors {
		opts.NoColors = false
	}
}

// colorFieldPtrs maps color key names (matching ini-name tags) to pointers into opts.Colors fields.
// this is the single source of truth for the color key -> struct field mapping,
// used by both applyTheme and collectColors to avoid duplicating the 23-key list.
func colorFieldPtrs(opts *options) map[string]*string {
	return map[string]*string{
		"color-accent":         &opts.Colors.Accent,
		"color-border":         &opts.Colors.Border,
		"color-normal":         &opts.Colors.Normal,
		"color-muted":          &opts.Colors.Muted,
		"color-selected-fg":    &opts.Colors.SelectedFg,
		"color-selected-bg":    &opts.Colors.SelectedBg,
		"color-annotation":     &opts.Colors.Annotation,
		"color-cursor-fg":      &opts.Colors.CursorFg,
		"color-cursor-bg":      &opts.Colors.CursorBg,
		"color-add-fg":         &opts.Colors.AddFg,
		"color-add-bg":         &opts.Colors.AddBg,
		"color-remove-fg":      &opts.Colors.RemoveFg,
		"color-remove-bg":      &opts.Colors.RemoveBg,
		"color-word-add-bg":    &opts.Colors.WordAddBg,
		"color-word-remove-bg": &opts.Colors.WordRemoveBg,
		"color-modify-fg":      &opts.Colors.ModifyFg,
		"color-modify-bg":      &opts.Colors.ModifyBg,
		"color-tree-bg":        &opts.Colors.TreeBg,
		"color-diff-bg":        &opts.Colors.DiffBg,
		"color-status-fg":      &opts.Colors.StatusFg,
		"color-status-bg":      &opts.Colors.StatusBg,
		"color-search-fg":      &opts.Colors.SearchFg,
		"color-search-bg":      &opts.Colors.SearchBg,
	}
}

// applyTheme applies theme colors and chroma-style to opts, overwriting all matching fields unconditionally.
// optional color keys absent from the theme are cleared to empty (terminal background) so that
// prior config/env values don't leak through when a theme intentionally omits them.
func applyTheme(opts *options, th theme.Theme, cat *theme.Catalog) {
	opts.ChromaStyle = th.ChromaStyle
	optional := cat.OptionalColorKeys()
	for key, ptr := range colorFieldPtrs(opts) {
		if v, ok := th.Colors[key]; ok {
			*ptr = v
			continue
		}
		if optional[key] {
			*ptr = "" // theme omits this optional key, clear to use terminal background
		}
	}
}

// collectColors gathers all resolved color values from opts into a map keyed by ini-name.
// empty values (optional colors like cursor-bg, tree-bg, diff-bg) are omitted so that
// Dump produces a theme file that Parse can load back without validation errors.
func collectColors(opts options) map[string]string {
	ptrs := colorFieldPtrs(&opts)
	result := make(map[string]string, len(ptrs))
	for key, ptr := range ptrs {
		if *ptr != "" {
			result[key] = *ptr
		}
	}
	return result
}

// compile-time check: *themeCatalog satisfies ui.ThemeCatalog.
var _ ui.ThemeCatalog = (*themeCatalog)(nil)

// themeCatalog adapts theme.Catalog + config-patching persistence into the ui.ThemeCatalog interface.
// it composes theme discovery (from app/theme) with selected-theme persistence (config file patching).
type themeCatalog struct {
	catalog    *theme.Catalog
	configPath string
}

// Entries returns the ordered list of available themes as UI-facing ThemeEntry values.
func (tc *themeCatalog) Entries() ([]ui.ThemeEntry, error) {
	infos, err := tc.catalog.Entries()
	if err != nil {
		return nil, fmt.Errorf("theme catalog entries: %w", err)
	}
	entries := make([]ui.ThemeEntry, 0, len(infos))
	for _, info := range infos {
		th, ok := tc.catalog.Resolve(info.Name)
		if !ok {
			continue
		}
		entries = append(entries, ui.ThemeEntry{
			Name:        info.Name,
			Local:       info.Local,
			AccentColor: th.Colors["color-accent"],
		})
	}
	return entries, nil
}

// Resolve loads a theme by name and returns it as a UI-facing ThemeSpec.
func (tc *themeCatalog) Resolve(name string) (ui.ThemeSpec, bool) {
	th, ok := tc.catalog.Resolve(name)
	if !ok {
		return ui.ThemeSpec{}, false
	}
	return ui.ThemeSpec{
		Colors:      colorsFromTheme(th),
		ChromaStyle: th.ChromaStyle,
	}, true
}

// Persist saves the theme choice to the config file.
func (tc *themeCatalog) Persist(name string) error {
	if tc.configPath == "" {
		return nil
	}
	return patchConfigTheme(tc.configPath, name)
}

// colorsFromTheme converts a theme.Theme color map to a style.Colors struct.
func colorsFromTheme(th theme.Theme) style.Colors {
	return style.Colors{
		Accent:       th.Colors["color-accent"],
		Border:       th.Colors["color-border"],
		Normal:       th.Colors["color-normal"],
		Muted:        th.Colors["color-muted"],
		SelectedFg:   th.Colors["color-selected-fg"],
		SelectedBg:   th.Colors["color-selected-bg"],
		Annotation:   th.Colors["color-annotation"],
		CursorFg:     th.Colors["color-cursor-fg"],
		CursorBg:     th.Colors["color-cursor-bg"],
		AddFg:        th.Colors["color-add-fg"],
		AddBg:        th.Colors["color-add-bg"],
		RemoveFg:     th.Colors["color-remove-fg"],
		RemoveBg:     th.Colors["color-remove-bg"],
		WordAddBg:    th.Colors["color-word-add-bg"],
		WordRemoveBg: th.Colors["color-word-remove-bg"],
		ModifyFg:     th.Colors["color-modify-fg"],
		ModifyBg:     th.Colors["color-modify-bg"],
		TreeBg:       th.Colors["color-tree-bg"],
		DiffBg:       th.Colors["color-diff-bg"],
		StatusFg:     th.Colors["color-status-fg"],
		StatusBg:     th.Colors["color-status-bg"],
		SearchFg:     th.Colors["color-search-fg"],
		SearchBg:     th.Colors["color-search-bg"],
	}
}

// patchConfigTheme updates the theme setting in the INI config file.
// if a "theme = " line exists, it replaces the value. Otherwise appends it.
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
		lines = slices.Insert(lines, insertIdx, "theme = "+themeName)
	}

	if err := fsutil.AtomicWriteFile(configPath, []byte(strings.Join(lines, "\n"))); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}
