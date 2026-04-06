package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/highlight"
	"github.com/umputun/revdiff/keymap"
	"github.com/umputun/revdiff/theme"
	"github.com/umputun/revdiff/ui"
)

type options struct {
	Refs struct {
		Base    string `positional-arg-name:"base" description:"git ref to diff against (default: uncommitted changes)"`
		Against string `positional-arg-name:"against" description:"second git ref for two-ref diff (e.g. revdiff main feature)"`
	} `positional-args:"yes"`

	Staged           bool     `long:"staged" ini-name:"staged" env:"REVDIFF_STAGED" description:"show staged changes"`
	TreeWidth        int      `long:"tree-width" ini-name:"tree-width" env:"REVDIFF_TREE_WIDTH" default:"2" description:"file tree panel width in units (1-10, default 2 of 10)"`
	TabWidth         int      `long:"tab-width" ini-name:"tab-width" env:"REVDIFF_TAB_WIDTH" default:"4" description:"number of spaces per tab character"`
	NoColors         bool     `long:"no-colors" ini-name:"no-colors" env:"REVDIFF_NO_COLORS" description:"disable all colors including syntax highlighting"`
	NoStatusBar      bool     `long:"no-status-bar" ini-name:"no-status-bar" env:"REVDIFF_NO_STATUS_BAR" description:"hide the status bar"`
	NoConfirmDiscard bool     `long:"no-confirm-discard" ini-name:"no-confirm-discard" env:"REVDIFF_NO_CONFIRM_DISCARD" description:"skip confirmation prompt when discarding annotations with Q"`
	Wrap             bool     `long:"wrap" ini-name:"wrap" env:"REVDIFF_WRAP" description:"enable line wrapping in diff view"`
	Collapsed        bool     `long:"collapsed" ini-name:"collapsed" env:"REVDIFF_COLLAPSED" description:"start in collapsed diff mode"`
	LineNumbers      bool     `long:"line-numbers" ini-name:"line-numbers" env:"REVDIFF_LINE_NUMBERS" description:"show line numbers in diff gutter"`
	ChromaStyle      string   `long:"chroma-style" ini-name:"chroma-style" env:"REVDIFF_CHROMA_STYLE" default:"catppuccin-macchiato" description:"chroma style for syntax highlighting"`
	AllFiles         bool     `long:"all-files" short:"A" no-ini:"true" description:"browse all git-tracked files, not just diffs"`
	Exclude          []string `long:"exclude" short:"X" ini-name:"exclude" env:"REVDIFF_EXCLUDE" env-delim:"," description:"exclude files matching prefix (may be repeated)"`
	Only             []string `long:"only" short:"F" no-ini:"true" description:"show only these files (may be repeated)"`
	Output           string   `long:"output" short:"o" env:"REVDIFF_OUTPUT" no-ini:"true" description:"write annotations to file instead of stdout"`
	Keys             string   `long:"keys" env:"REVDIFF_KEYS" no-ini:"true" description:"path to keybindings file"`
	DumpKeys         bool     `long:"dump-keys" no-ini:"true" description:"print effective keybindings to stdout and exit"`
	Theme            string   `long:"theme" ini-name:"theme" env:"REVDIFF_THEME" description:"load theme from themes directory"`
	DumpTheme        bool     `long:"dump-theme" no-ini:"true" description:"print currently resolved colors as theme file and exit"`
	ListThemes       bool     `long:"list-themes" no-ini:"true" description:"print available theme names and exit"`
	InitThemes       bool     `long:"init-themes" no-ini:"true" description:"write bundled theme files to themes dir and exit"`
	Config           string   `long:"config" env:"REVDIFF_CONFIG" no-ini:"true" description:"path to config file"`
	DumpConfig       bool     `long:"dump-config" no-ini:"true" description:"print default config to stdout and exit"`
	Version          bool     `short:"V" long:"version" no-ini:"true" description:"show version info"`

	Colors struct {
		Accent     string `long:"color-accent"      ini-name:"color-accent"      env:"REVDIFF_COLOR_ACCENT"      default:"#D5895F" description:"active pane borders and directory names"`
		Border     string `long:"color-border"      ini-name:"color-border"      env:"REVDIFF_COLOR_BORDER"      default:"#585858" description:"inactive pane borders"`
		Normal     string `long:"color-normal"      ini-name:"color-normal"      env:"REVDIFF_COLOR_NORMAL"      default:"#d0d0d0" description:"file entries and context lines"`
		Muted      string `long:"color-muted"       ini-name:"color-muted"       env:"REVDIFF_COLOR_MUTED"       default:"#585858" description:"line numbers and status bar"`
		SelectedFg string `long:"color-selected-fg" ini-name:"color-selected-fg" env:"REVDIFF_COLOR_SELECTED_FG" default:"#ffffaf" description:"selected file text color"`
		SelectedBg string `long:"color-selected-bg" ini-name:"color-selected-bg" env:"REVDIFF_COLOR_SELECTED_BG" default:"#D5895F" description:"selected file background color"`
		Annotation string `long:"color-annotation"  ini-name:"color-annotation"  env:"REVDIFF_COLOR_ANNOTATION"  default:"#ffd700" description:"annotation text and markers"`
		CursorFg   string `long:"color-cursor-fg"   ini-name:"color-cursor-fg"   env:"REVDIFF_COLOR_CURSOR_FG"   default:"#bbbb44" description:"diff cursor indicator color"`
		CursorBg   string `long:"color-cursor-bg"   ini-name:"color-cursor-bg"   env:"REVDIFF_COLOR_CURSOR_BG"   description:"diff cursor indicator background"`
		AddFg      string `long:"color-add-fg"      ini-name:"color-add-fg"      env:"REVDIFF_COLOR_ADD_FG"      default:"#87d787" description:"added line text color"`
		AddBg      string `long:"color-add-bg"      ini-name:"color-add-bg"      env:"REVDIFF_COLOR_ADD_BG"      default:"#123800" description:"added line background color"`
		RemoveFg   string `long:"color-remove-fg"   ini-name:"color-remove-fg"   env:"REVDIFF_COLOR_REMOVE_FG"   default:"#ff8787" description:"removed line text color"`
		RemoveBg   string `long:"color-remove-bg"   ini-name:"color-remove-bg"   env:"REVDIFF_COLOR_REMOVE_BG"   default:"#4D1100" description:"removed line background color"`
		ModifyFg   string `long:"color-modify-fg"   ini-name:"color-modify-fg"   env:"REVDIFF_COLOR_MODIFY_FG"   default:"#f5c542" description:"modified line text color (collapsed mode)"`
		ModifyBg   string `long:"color-modify-bg"   ini-name:"color-modify-bg"   env:"REVDIFF_COLOR_MODIFY_BG"   default:"#3D2E00" description:"modified line background color (collapsed mode)"`
		TreeBg     string `long:"color-tree-bg"     ini-name:"color-tree-bg"     env:"REVDIFF_COLOR_TREE_BG"     description:"file tree pane background"`
		DiffBg     string `long:"color-diff-bg"     ini-name:"color-diff-bg"     env:"REVDIFF_COLOR_DIFF_BG"     description:"diff pane background"`
		StatusFg   string `long:"color-status-fg"   ini-name:"color-status-fg"   env:"REVDIFF_COLOR_STATUS_FG"   default:"#202020" description:"status bar foreground"`
		StatusBg   string `long:"color-status-bg"   ini-name:"color-status-bg"   env:"REVDIFF_COLOR_STATUS_BG"   default:"#C5794F" description:"status bar background"`
		SearchFg   string `long:"color-search-fg"   ini-name:"color-search-fg"   env:"REVDIFF_COLOR_SEARCH_FG"   default:"#1a1a1a" description:"search match foreground"`
		SearchBg   string `long:"color-search-bg"   ini-name:"color-search-bg"   env:"REVDIFF_COLOR_SEARCH_BG"   default:"#4a4a00" description:"search match background"`
	} `group:"color options"`
}

// ref returns the combined ref string from positional args.
// two refs are joined with ".." to form a range (e.g. "main..feature").
func (o options) ref() string {
	if o.Refs.Against != "" {
		return o.Refs.Base + ".." + o.Refs.Against
	}
	return o.Refs.Base
}

var revision = "unknown"

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		if !errors.As(err, &flagsErr) {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		os.Exit(1)
	}

	// early-exit commands that don't need theme resolution
	if opts.Version {
		fmt.Printf("version: %s\n", revision)
		os.Exit(0)
	}

	if opts.DumpConfig {
		dumpConfig(os.Args[1:], os.Stdout)
		os.Exit(0)
	}

	if opts.DumpKeys {
		km := keymap.LoadOrDefault(resolveKeysPath(os.Args[1:]))
		if err := km.Dump(os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	done, thErr := handleThemes(&opts, defaultThemesDir(), os.Stdout, os.Stderr)
	if thErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", thErr)
		os.Exit(1)
	}
	if done {
		os.Exit(0)
	}

	if opts.DumpTheme {
		colors := collectColors(opts)
		if err := theme.Dump(theme.Theme{Colors: colors, ChromaStyle: opts.ChromaStyle}, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs parses CLI arguments with config file support.
// config file is loaded first, then CLI args override.
// precedence: CLI flags > env vars > config file > built-in defaults.
func parseArgs(args []string) (options, error) {
	var opts options
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[OPTIONS] [base] [against]"

	// determine config path from args before full parsing
	configPath := resolveConfigPath(args)

	// load config file before parsing CLI args (CLI overrides config)
	iniParser := flags.NewIniParser(p)
	loadConfigFile(iniParser, configPath)

	if _, err := p.ParseArgs(args); err != nil {
		return options{}, fmt.Errorf("parse args: %w", err)
	}

	if opts.Staged && (opts.Refs.Against != "" || strings.Contains(opts.Refs.Base, "..")) {
		return options{}, errors.New("--staged cannot be used with two-ref diff")
	}

	if opts.AllFiles {
		if opts.Refs.Base != "" || opts.Refs.Against != "" {
			return options{}, errors.New("--all-files cannot be used with refs")
		}
		if opts.Staged {
			return options{}, errors.New("--all-files cannot be used with --staged")
		}
		if len(opts.Only) > 0 {
			return options{}, errors.New("--all-files cannot be used with --only")
		}
	}

	return opts, nil
}

// dumpConfig writes the current config with defaults to the given writer.
func dumpConfig(args []string, w io.Writer) {
	var opts options
	p := flags.NewParser(&opts, flags.Default)
	iniParser := flags.NewIniParser(p)
	configPath := resolveConfigPath(args)
	loadConfigFile(iniParser, configPath)
	_, _ = p.ParseArgs(args)
	iniParser.Write(w, flags.IniIncludeDefaults|flags.IniCommentDefaults|flags.IniIncludeComments)
}

// loadConfigFile attempts to parse a config file, logging a warning on parse errors.
// silently ignores missing files or empty paths.
func loadConfigFile(iniParser *flags.IniParser, configPath string) {
	if configPath == "" {
		return
	}
	err := iniParser.ParseFile(configPath)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return // file access error (permission denied, etc.)
	}
	fmt.Fprintf(os.Stderr, "warning: config %s: %v\n", configPath, err)
}

// resolveConfigPath determines the config file path from args, env, or default location.
func resolveConfigPath(args []string) string {
	// check if --config was passed in args (supports both --config value and --config=value)
	for i, arg := range args {
		if arg == "--config" && i+1 < len(args) {
			return args[i+1]
		}
		if after, ok := strings.CutPrefix(arg, "--config="); ok {
			return after
		}
	}
	// check env
	if p := os.Getenv("REVDIFF_CONFIG"); p != "" {
		return p
	}
	// default location
	return defaultConfigPath()
}

// defaultConfigPath returns ~/.config/revdiff/config.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "config")
}

// resolveKeysPath determines the keybindings file path from args, env, or default location.
func resolveKeysPath(args []string) string {
	// check if --keys was passed in args (supports both --keys value and --keys=value)
	for i, arg := range args {
		if arg == "--keys" && i+1 < len(args) {
			return args[i+1]
		}
		if after, ok := strings.CutPrefix(arg, "--keys="); ok {
			return after
		}
	}
	// check env
	if p := os.Getenv("REVDIFF_KEYS"); p != "" {
		return p
	}
	// default location
	return defaultKeysPath()
}

// defaultKeysPath returns ~/.config/revdiff/keybindings.
func defaultKeysPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "keybindings")
}

func run(opts options) error {
	gitRoot, gitErr := gitTopLevel()
	renderer, workDir, err := makeRenderer(opts.Only, opts.Exclude, opts.AllFiles, gitRoot, gitErr)
	if err != nil {
		return err
	}
	store := annotation.NewStore()
	hl := highlight.New(opts.ChromaStyle, !opts.NoColors)
	keysPath := opts.Keys
	if keysPath == "" {
		keysPath = defaultKeysPath()
	}
	km := keymap.LoadOrDefault(keysPath)
	model := ui.NewModel(renderer, store, hl, ui.ModelConfig{
		Keymap:           km,
		NoColors:         opts.NoColors,
		NoStatusBar:      opts.NoStatusBar,
		NoConfirmDiscard: opts.NoConfirmDiscard,
		Wrap:             opts.Wrap,
		Collapsed:        opts.Collapsed,
		LineNumbers:      opts.LineNumbers,
		TabWidth:         opts.TabWidth,
		Ref:              opts.ref(),
		Staged:           opts.Staged,
		TreeWidthRatio:   opts.TreeWidth,
		Only:             opts.Only,
		WorkDir:          workDir,
		Colors: ui.Colors{
			Accent:     opts.Colors.Accent,
			Border:     opts.Colors.Border,
			Normal:     opts.Colors.Normal,
			Muted:      opts.Colors.Muted,
			SelectedFg: opts.Colors.SelectedFg,
			SelectedBg: opts.Colors.SelectedBg,
			Annotation: opts.Colors.Annotation,
			CursorFg:   opts.Colors.CursorFg,
			CursorBg:   opts.Colors.CursorBg,
			AddFg:      opts.Colors.AddFg,
			AddBg:      opts.Colors.AddBg,
			RemoveFg:   opts.Colors.RemoveFg,
			RemoveBg:   opts.Colors.RemoveBg,
			ModifyFg:   opts.Colors.ModifyFg,
			ModifyBg:   opts.Colors.ModifyBg,
			TreeBg:     opts.Colors.TreeBg,
			DiffBg:     opts.Colors.DiffBg,
			StatusFg:   opts.Colors.StatusFg,
			StatusBg:   opts.Colors.StatusBg,
			SearchFg:   opts.Colors.SearchFg,
			SearchBg:   opts.Colors.SearchBg,
		},
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// output annotations to stdout or file
	m, ok := finalModel.(ui.Model)
	if !ok {
		return nil
	}
	if m.Discarded() {
		return nil
	}
	output := m.Store().FormatOutput()
	if output == "" {
		return nil
	}
	if opts.Output != "" {
		if err := os.WriteFile(opts.Output, []byte(output), 0o600); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}
	fmt.Print(output)
	return nil
}

// makeRenderer selects the appropriate renderer based on git availability and flags.
// if --all-files is set, returns DirectoryReader (requires git repo).
// if git is available with --only, it wraps diff.Git with FallbackRenderer.
// if git is available without --only, it returns diff.Git directly.
// if git is unavailable and --only is set, it uses FileReader to read files directly from disk.
// if git is unavailable and --only is not set, it returns an error.
// when --exclude prefixes are present, wraps the result with ExcludeFilter.
func makeRenderer(only, exclude []string, allFiles bool, gitRoot string, gitErr error) (ui.Renderer, string, error) {
	var r ui.Renderer
	var workDir string

	switch {
	case allFiles && gitErr == nil:
		r = diff.NewDirectoryReader(gitRoot)
		workDir = gitRoot
	case allFiles:
		return nil, "", errors.New("--all-files requires a git repository")
	case gitErr == nil && len(only) > 0:
		r = diff.NewFallbackRenderer(diff.NewGit(gitRoot), only, gitRoot)
		workDir = gitRoot
	case gitErr == nil:
		r = diff.NewGit(gitRoot)
		workDir = gitRoot
	case len(only) > 0:
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", fmt.Errorf("get working directory: %w", err)
		}
		r = diff.NewFileReader(only, cwd)
		workDir = cwd
	default:
		return nil, "", fmt.Errorf("find git root: %w", gitErr)
	}

	if len(exclude) > 0 {
		r = diff.NewExcludeFilter(r, exclude)
	}
	return r, workDir, nil
}

// gitTopLevel returns the root directory of the current git repository.
func gitTopLevel() (string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git rev-parse --show-toplevel: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// defaultThemesDir returns ~/.config/revdiff/themes.
func defaultThemesDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "themes")
}

// handleThemes processes theme-related flags: auto-init on first run, --init-themes, --list-themes, and --theme.
// returns (true, nil) when the caller should exit successfully, (false, error) on failure, (false, nil) to continue.
func handleThemes(opts *options, themesDir string, stdout, stderr io.Writer) (bool, error) {
	// auto-init bundled themes on first run (silent, no error on failure)
	if themesDir != "" {
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

	if opts.ListThemes {
		if themesDir == "" {
			return false, errors.New("cannot determine home directory for themes")
		}
		names, err := theme.List(themesDir)
		if err != nil {
			return false, fmt.Errorf("list themes: %w", err)
		}
		for _, name := range names {
			_, _ = fmt.Fprintln(stdout, name)
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
// this is the single source of truth for the color key → struct field mapping,
// used by both applyTheme and collectColors to avoid duplicating the 21-key list.
func colorFieldPtrs(opts *options) map[string]*string {
	return map[string]*string{
		"color-accent":      &opts.Colors.Accent,
		"color-border":      &opts.Colors.Border,
		"color-normal":      &opts.Colors.Normal,
		"color-muted":       &opts.Colors.Muted,
		"color-selected-fg": &opts.Colors.SelectedFg,
		"color-selected-bg": &opts.Colors.SelectedBg,
		"color-annotation":  &opts.Colors.Annotation,
		"color-cursor-fg":   &opts.Colors.CursorFg,
		"color-cursor-bg":   &opts.Colors.CursorBg,
		"color-add-fg":      &opts.Colors.AddFg,
		"color-add-bg":      &opts.Colors.AddBg,
		"color-remove-fg":   &opts.Colors.RemoveFg,
		"color-remove-bg":   &opts.Colors.RemoveBg,
		"color-modify-fg":   &opts.Colors.ModifyFg,
		"color-modify-bg":   &opts.Colors.ModifyBg,
		"color-tree-bg":     &opts.Colors.TreeBg,
		"color-diff-bg":     &opts.Colors.DiffBg,
		"color-status-fg":   &opts.Colors.StatusFg,
		"color-status-bg":   &opts.Colors.StatusBg,
		"color-search-fg":   &opts.Colors.SearchFg,
		"color-search-bg":   &opts.Colors.SearchBg,
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
