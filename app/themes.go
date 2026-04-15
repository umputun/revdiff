package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/umputun/revdiff/app/highlight"
	"github.com/umputun/revdiff/app/theme"
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
func handleThemes(opts *options, themesDir string, stdout, stderr io.Writer) (bool, error) {
	// auto-init bundled themes on first run (silent, no error on failure)
	if themesDir != "" && len(opts.InstallTheme) == 0 {
		if _, err := os.Stat(themesDir); os.IsNotExist(err) {
			_ = theme.InitBundled(themesDir)
		}
	}

	if opts.InitThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := theme.InitBundled(themesDir); err != nil {
			return false, fmt.Errorf("init themes: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "bundled themes written to %s\n", themesDir)
		return true, nil
	}

	if opts.InitAllThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := theme.InitAll(themesDir); err != nil {
			return false, fmt.Errorf("init all themes: %w", err)
		}
		galleryNames, _ := theme.GalleryNames()
		_, _ = fmt.Fprintf(stdout, "all themes written to %s (%d themes)\n", themesDir, len(galleryNames))
		return true, nil
	}

	if len(opts.InstallTheme) > 0 {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := theme.Install(opts.InstallTheme, themesDir, highlight.IsValidStyle, stdout); err != nil {
			return false, fmt.Errorf("install theme: %w", err)
		}
		return true, nil
	}

	if opts.ListThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		if err := theme.PrintList(themesDir, stdout); err != nil {
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
	th, err := theme.Load(opts.Theme, themesDir)
	if err != nil {
		return false, fmt.Errorf("load theme: %w", err)
	}
	applyTheme(opts, th)
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
func applyTheme(opts *options, th theme.Theme) {
	opts.ChromaStyle = th.ChromaStyle
	optional := theme.OptionalColorKeys()
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
