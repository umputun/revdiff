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

	// no theme-related flags set — nothing to do
	if !opts.InitThemes && !opts.InitAllThemes && len(opts.InstallTheme) == 0 && !opts.ListThemes && opts.Theme == "" {
		return false, nil
	}

	// all theme operations below require a valid themes directory
	if themesDir == "" {
		return false, errors.New("cannot determine home directory for themes")
	}

	switch {
	case opts.InitThemes:
		if err := cat.InitBundled(); err != nil {
			return false, fmt.Errorf("init themes: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "bundled themes written to %s\n", themesDir)
		return true, nil
	case opts.InitAllThemes:
		if err := cat.InitAll(); err != nil {
			return false, fmt.Errorf("init all themes: %w", err)
		}
		_, _ = fmt.Fprintf(stdout, "all gallery themes written to %s\n", themesDir)
		return true, nil
	case len(opts.InstallTheme) > 0:
		if err := cat.Install(opts.InstallTheme, highlight.IsValidStyle, stdout); err != nil {
			return false, fmt.Errorf("install theme: %w", err)
		}
		return true, nil
	case opts.ListThemes:
		if err := cat.PrintList(stdout); err != nil {
			return false, fmt.Errorf("list themes: %w", err)
		}
		return true, nil
	}

	// apply theme — overwrites present color fields and clears absent optional ones.
	// theme takes over completely, ignoring any --color-* flags, env vars, or --no-colors.
	if opts.NoColors {
		_, _ = fmt.Fprintln(stderr, "warning: --no-colors ignored when --theme is set")
		opts.NoColors = false
	}
	th, err := cat.Load(opts.Theme)
	if err != nil {
		return false, fmt.Errorf("load theme: %w", err)
	}
	applyTheme(opts, th, cat.OptionalColorKeys())
	return false, nil
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
func applyTheme(opts *options, th theme.Theme, optional map[string]bool) {
	opts.ChromaStyle = th.ChromaStyle
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
	return tc.patchConfigTheme(name)
}

// optsToStyleColors converts opts.Colors fields to a style.Colors struct.
func optsToStyleColors(opts options) style.Colors {
	c := opts.Colors
	return style.Colors{
		Accent: c.Accent, Border: c.Border, Normal: c.Normal, Muted: c.Muted,
		SelectedFg: c.SelectedFg, SelectedBg: c.SelectedBg, Annotation: c.Annotation,
		CursorFg: c.CursorFg, CursorBg: c.CursorBg,
		AddFg: c.AddFg, AddBg: c.AddBg, RemoveFg: c.RemoveFg, RemoveBg: c.RemoveBg,
		WordAddBg: c.WordAddBg, WordRemoveBg: c.WordRemoveBg,
		ModifyFg: c.ModifyFg, ModifyBg: c.ModifyBg,
		TreeBg: c.TreeBg, DiffBg: c.DiffBg,
		StatusFg: c.StatusFg, StatusBg: c.StatusBg,
		SearchFg: c.SearchFg, SearchBg: c.SearchBg,
	}
}

// colorsFromTheme converts a theme.Theme color map to a style.Colors struct.
func colorsFromTheme(th theme.Theme) style.Colors {
	m := th.Colors
	return style.Colors{
		Accent: m["color-accent"], Border: m["color-border"], Normal: m["color-normal"], Muted: m["color-muted"],
		SelectedFg: m["color-selected-fg"], SelectedBg: m["color-selected-bg"], Annotation: m["color-annotation"],
		CursorFg: m["color-cursor-fg"], CursorBg: m["color-cursor-bg"],
		AddFg: m["color-add-fg"], AddBg: m["color-add-bg"], RemoveFg: m["color-remove-fg"], RemoveBg: m["color-remove-bg"],
		WordAddBg: m["color-word-add-bg"], WordRemoveBg: m["color-word-remove-bg"],
		ModifyFg: m["color-modify-fg"], ModifyBg: m["color-modify-bg"],
		TreeBg: m["color-tree-bg"], DiffBg: m["color-diff-bg"],
		StatusFg: m["color-status-fg"], StatusBg: m["color-status-bg"],
		SearchFg: m["color-search-fg"], SearchBg: m["color-search-bg"],
	}
}

// patchConfigTheme updates the theme setting in the INI config file at tc.configPath.
// every "theme = ..." line sitting outside the default scope ([Application Options]
// or the unnamed top-of-file section) is removed — these are strays from configs
// corrupted by the pre-fix persist path, and leaving any of them behind keeps
// go-flags erroring with "unknown option: theme" even after a successful patch.
// if a default-scope line exists its value is replaced in place; otherwise a
// fresh "theme = ..." is inserted just before the first non-[Application Options]
// section header so the INI parser attributes it to the default scope.
func (tc *themeCatalog) patchConfigTheme(themeName string) error {
	if strings.ContainsAny(themeName, "\r\n") {
		return fmt.Errorf("invalid theme name %q: must not contain newlines", themeName)
	}
	if err := os.MkdirAll(filepath.Dir(tc.configPath), 0o750); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := os.ReadFile(tc.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			if writeErr := fsutil.AtomicWriteFile(tc.configPath, []byte("theme = "+themeName+"\n")); writeErr != nil {
				return fmt.Errorf("writing config: %w", writeErr)
			}
			return nil
		}
		return fmt.Errorf("reading config: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	defaultIdx, strayIdxs := tc.scanThemeLines(lines)
	// always remove every "theme = ..." that sits outside the default scope so a
	// config previously poisoned by the old persist path is fully healed (not
	// just patched in one spot while go-flags keeps erroring on a remaining stray).
	// deleting in reverse order keeps earlier indices stable.
	for i := len(strayIdxs) - 1; i >= 0; i-- {
		stray := strayIdxs[i]
		lines = slices.Delete(lines, stray, stray+1)
		if defaultIdx > stray {
			defaultIdx--
		}
	}
	if defaultIdx >= 0 {
		lines[defaultIdx] = "theme = " + themeName
	} else {
		lines = slices.Insert(lines, tc.defaultSectionInsertIdx(lines), "theme = "+themeName)
	}

	if err := fsutil.AtomicWriteFile(tc.configPath, []byte(strings.Join(lines, "\n"))); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// scanThemeLines walks lines tracking the active INI section and reports every
// "theme = ..." occurrence, splitting them into the first one found in the
// default scope ([Application Options] or the unnamed top-of-file section) and
// a list of stray ones sitting inside other sections. defaultIdx is -1 when no
// default-scope line exists; strayIdxs is nil when nothing is misplaced.
// reporting every stray (not just the first) lets callers fully heal configs
// corrupted by the pre-fix persist path — otherwise a leftover stray still
// makes go-flags error on startup.
func (tc *themeCatalog) scanThemeLines(lines []string) (defaultIdx int, strayIdxs []int) {
	defaultIdx = -1
	currentSection := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			currentSection = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			continue
		}
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if !ok || strings.TrimSpace(key) != "theme" {
			continue
		}
		inDefault := currentSection == "" || strings.EqualFold(currentSection, "Application Options")
		if inDefault && defaultIdx < 0 {
			defaultIdx = i
			continue
		}
		strayIdxs = append(strayIdxs, i)
	}
	return defaultIdx, strayIdxs
}

// defaultSectionInsertIdx returns the line index where a new default-scope entry
// should be placed: immediately before the first [section] header whose name is
// not [Application Options], backed up over any trailing blank lines. returns
// len(lines) (EOF) when the file has no such named section.
func (tc *themeCatalog) defaultSectionInsertIdx(lines []string) int {
	idx := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
			continue
		}
		name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		if strings.EqualFold(name, "Application Options") {
			continue
		}
		idx = i
		break
	}
	for idx > 0 && strings.TrimSpace(lines[idx-1]) == "" {
		idx--
	}
	return idx
}
