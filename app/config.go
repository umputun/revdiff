package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"
)

type options struct {
	Refs struct {
		Base    string `positional-arg-name:"base" description:"git ref to diff against (default: uncommitted changes)"`
		Against string `positional-arg-name:"against" description:"second git ref for two-ref diff (e.g. revdiff main feature)"`
	} `positional-args:"yes"`

	Staged           bool     `long:"staged" ini-name:"staged" env:"REVDIFF_STAGED" description:"show staged changes"`
	Untracked        bool     `long:"untracked" ini-name:"untracked" env:"REVDIFF_UNTRACKED" description:"show untracked files in the tree"`
	TreeWidth        int      `long:"tree-width" ini-name:"tree-width" env:"REVDIFF_TREE_WIDTH" default:"2" description:"file tree panel width in units (1-10, default 2 of 10)"`
	TabWidth         int      `long:"tab-width" ini-name:"tab-width" env:"REVDIFF_TAB_WIDTH" default:"4" description:"number of spaces per tab character"`
	NoColors         bool     `long:"no-colors" ini-name:"no-colors" env:"REVDIFF_NO_COLORS" description:"disable all colors including syntax highlighting"`
	NoStatusBar      bool     `long:"no-status-bar" ini-name:"no-status-bar" env:"REVDIFF_NO_STATUS_BAR" description:"hide the status bar"`
	NoConfirmDiscard bool     `long:"no-confirm-discard" ini-name:"no-confirm-discard" env:"REVDIFF_NO_CONFIRM_DISCARD" description:"skip confirmation prompt when discarding annotations with Q"`
	NoMouse          bool     `long:"no-mouse" ini-name:"no-mouse" env:"REVDIFF_NO_MOUSE" description:"disable mouse support (scroll wheel, click)"`
	Wrap             bool     `long:"wrap" ini-name:"wrap" env:"REVDIFF_WRAP" description:"enable line wrapping in diff view"`
	WrapIndent       int      `long:"wrap-indent" ini-name:"wrap-indent" env:"REVDIFF_WRAP_INDENT" default:"0" description:"indent wrap continuation rows by N columns so they hang under the first row's content (helps when reviewing markdown lists where unindented continuation can be misread as a new bullet)"`
	Collapsed        bool     `long:"collapsed" ini-name:"collapsed" env:"REVDIFF_COLLAPSED" description:"start in collapsed diff mode"`
	Compact          bool     `long:"compact" ini-name:"compact" env:"REVDIFF_COMPACT" description:"start in compact diff mode (small context around changes)"`
	CompactContext   int      `long:"compact-context" ini-name:"compact-context" env:"REVDIFF_COMPACT_CONTEXT" default:"5" description:"number of context lines around changes when in compact mode"`
	CrossFileHunks   bool     `long:"cross-file-hunks" ini-name:"cross-file-hunks" env:"REVDIFF_CROSS_FILE_HUNKS" description:"allow [ and ] to jump across file boundaries"`
	LineNumbers      bool     `long:"line-numbers" ini-name:"line-numbers" env:"REVDIFF_LINE_NUMBERS" description:"show line numbers in diff gutter"`
	Blame            bool     `long:"blame" ini-name:"blame" env:"REVDIFF_BLAME" description:"show blame gutter"`
	WordDiff         bool     `long:"word-diff" ini-name:"word-diff" env:"REVDIFF_WORD_DIFF" description:"highlight intra-line word-level changes in paired add/remove lines"`
	VimMotion        bool     `long:"vim-motion" ini-name:"vim-motion" env:"REVDIFF_VIM_MOTION" description:"enable vim-style motion preset (counts, gg, G, zz/zt/zb, ZZ/ZQ)"`
	ChromaStyle      string   `long:"chroma-style" ini-name:"chroma-style" env:"REVDIFF_CHROMA_STYLE" default:"catppuccin-macchiato" description:"chroma style for syntax highlighting"`
	AllFiles         bool     `long:"all-files" short:"A" no-ini:"true" description:"browse all tracked files, not just diffs (git and jj only)"`
	CompareOld       string   `long:"compare-old" no-ini:"true" description:"compare mode: old file path (use with --compare-new)"`
	CompareNew       string   `long:"compare-new" no-ini:"true" description:"compare mode: new file path (use with --compare-old)"`
	Stdin            bool     `long:"stdin" no-ini:"true" description:"review stdin as a scratch buffer"`
	StdinName        string   `long:"stdin-name" no-ini:"true" description:"synthetic file name for stdin content"`
	Annotations      string   `long:"annotations" no-ini:"true" description:"preload annotations from a markdown file written by -o (round-trip)"`
	Description      string   `long:"description" no-ini:"true" description:"prose context shown in the info popup (markdown; use shell multiline quoting or --description-file for multiple lines)"`
	DescriptionFile  string   `long:"description-file" no-ini:"true" description:"read the info-popup description from this file (markdown)"`
	Exclude          []string `long:"exclude" short:"X" ini-name:"exclude" env:"REVDIFF_EXCLUDE" env-delim:"," description:"exclude files matching prefix (may be repeated)"`
	Include          []string `long:"include" short:"I" ini-name:"include" env:"REVDIFF_INCLUDE" env-delim:"," description:"include only files matching prefix (may be repeated)"`
	Only             []string `long:"only" short:"F" no-ini:"true" description:"show only these files (may be repeated)"`
	HistoryDir       string   `long:"history-dir" ini-name:"history-dir" env:"REVDIFF_HISTORY_DIR" description:"directory for review history auto-saves"`
	Output           string   `long:"output" short:"o" env:"REVDIFF_OUTPUT" no-ini:"true" description:"write annotations to file instead of stdout"`
	Keys             string   `long:"keys" env:"REVDIFF_KEYS" no-ini:"true" description:"path to keybindings file"`
	DumpKeys         bool     `long:"dump-keys" no-ini:"true" description:"print effective keybindings to stdout and exit"`
	Theme            string   `long:"theme" ini-name:"theme" env:"REVDIFF_THEME" description:"load theme from themes directory"`
	DumpTheme        bool     `long:"dump-theme" no-ini:"true" description:"print currently resolved colors as theme file and exit"`
	ListThemes       bool     `long:"list-themes" no-ini:"true" description:"print available theme names and exit"`
	InitThemes       bool     `long:"init-themes" no-ini:"true" description:"write bundled theme files to themes dir and exit"`
	InitAllThemes    bool     `long:"init-all-themes" no-ini:"true" description:"write all gallery themes (bundled + community) to themes dir and exit"`
	InstallTheme     []string `long:"install-theme" no-ini:"true" description:"install theme(s) from gallery or local file path and exit"`
	Config           string   `long:"config" env:"REVDIFF_CONFIG" no-ini:"true" description:"path to config file"`
	DumpConfig       bool     `long:"dump-config" no-ini:"true" description:"print default config to stdout and exit"`
	Version          bool     `short:"V" long:"version" no-ini:"true" description:"show version info"`

	Colors struct {
		Accent       string `long:"color-accent"      ini-name:"color-accent"      env:"REVDIFF_COLOR_ACCENT"      default:"#D5895F" description:"active pane borders and directory names"`
		Border       string `long:"color-border"      ini-name:"color-border"      env:"REVDIFF_COLOR_BORDER"      default:"#585858" description:"inactive pane borders"`
		Normal       string `long:"color-normal"      ini-name:"color-normal"      env:"REVDIFF_COLOR_NORMAL"      default:"#d0d0d0" description:"file entries and context lines"`
		Muted        string `long:"color-muted"       ini-name:"color-muted"       env:"REVDIFF_COLOR_MUTED"       default:"#585858" description:"line numbers and status bar"`
		SelectedFg   string `long:"color-selected-fg" ini-name:"color-selected-fg" env:"REVDIFF_COLOR_SELECTED_FG" default:"#ffffaf" description:"selected file text color"`
		SelectedBg   string `long:"color-selected-bg" ini-name:"color-selected-bg" env:"REVDIFF_COLOR_SELECTED_BG" default:"#D5895F" description:"selected file background color"`
		Annotation   string `long:"color-annotation"  ini-name:"color-annotation"  env:"REVDIFF_COLOR_ANNOTATION"  default:"#ffd700" description:"annotation text and markers"`
		CursorFg     string `long:"color-cursor-fg"   ini-name:"color-cursor-fg"   env:"REVDIFF_COLOR_CURSOR_FG"   default:"#bbbb44" description:"diff cursor indicator color"`
		CursorBg     string `long:"color-cursor-bg"   ini-name:"color-cursor-bg"   env:"REVDIFF_COLOR_CURSOR_BG"   description:"diff cursor indicator background"`
		AddFg        string `long:"color-add-fg"      ini-name:"color-add-fg"      env:"REVDIFF_COLOR_ADD_FG"      default:"#87d787" description:"added line text color"`
		AddBg        string `long:"color-add-bg"      ini-name:"color-add-bg"      env:"REVDIFF_COLOR_ADD_BG"      default:"#123800" description:"added line background color"`
		RemoveFg     string `long:"color-remove-fg"   ini-name:"color-remove-fg"   env:"REVDIFF_COLOR_REMOVE_FG"   default:"#ff8787" description:"removed line text color"`
		RemoveBg     string `long:"color-remove-bg"   ini-name:"color-remove-bg"   env:"REVDIFF_COLOR_REMOVE_BG"   default:"#4D1100" description:"removed line background color"`
		WordAddBg    string `long:"color-word-add-bg"    ini-name:"color-word-add-bg"    env:"REVDIFF_COLOR_WORD_ADD_BG"    description:"intra-line word-diff add background (auto-derived if empty)"`
		WordRemoveBg string `long:"color-word-remove-bg" ini-name:"color-word-remove-bg" env:"REVDIFF_COLOR_WORD_REMOVE_BG" description:"intra-line word-diff remove background (auto-derived if empty)"`
		ModifyFg     string `long:"color-modify-fg"      ini-name:"color-modify-fg"      env:"REVDIFF_COLOR_MODIFY_FG"      default:"#f5c542" description:"modified line text color (collapsed mode)"`
		ModifyBg     string `long:"color-modify-bg"   ini-name:"color-modify-bg"   env:"REVDIFF_COLOR_MODIFY_BG"   default:"#3D2E00" description:"modified line background color (collapsed mode)"`
		TreeBg       string `long:"color-tree-bg"     ini-name:"color-tree-bg"     env:"REVDIFF_COLOR_TREE_BG"     description:"file tree pane background"`
		DiffBg       string `long:"color-diff-bg"     ini-name:"color-diff-bg"     env:"REVDIFF_COLOR_DIFF_BG"     description:"diff pane background"`
		StatusFg     string `long:"color-status-fg"   ini-name:"color-status-fg"   env:"REVDIFF_COLOR_STATUS_FG"   default:"#202020" description:"status bar foreground"`
		StatusBg     string `long:"color-status-bg"   ini-name:"color-status-bg"   env:"REVDIFF_COLOR_STATUS_BG"   default:"#C5794F" description:"status bar background"`
		SearchFg     string `long:"color-search-fg"   ini-name:"color-search-fg"   env:"REVDIFF_COLOR_SEARCH_FG"   default:"#1a1a1a" description:"search match foreground"`
		SearchBg     string `long:"color-search-bg"   ini-name:"color-search-bg"   env:"REVDIFF_COLOR_SEARCH_BG"   default:"#4a4a00" description:"search match background"`
	} `group:"color options"`

	compareAbsOld string
	compareAbsNew string
}

// ref returns the combined ref string from positional args.
// two refs are joined with ".." to form a range (e.g. "main..feature").
func (o options) ref() string {
	if o.Refs.Against != "" {
		return o.Refs.Base + ".." + o.Refs.Against
	}
	return o.Refs.Base
}

// startupUntracked reports whether --untracked should activate.
// disabled in two-ref mode (both `a b` and `a..b` forms) because untracked
// files are working-tree state, not part of a historical diff between refs.
func (o options) startupUntracked() bool {
	if !o.Untracked {
		return false
	}
	if o.Refs.Against != "" || strings.Contains(o.Refs.Base, "..") {
		return false
	}
	return true
}

// parseArgs parses CLI arguments with config file support.
// config file is loaded first, then CLI args override.
// precedence: CLI flags > env vars > config file > built-in defaults.
func parseArgs(args []string) (options, error) {
	var opts options
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[OPTIONS] [base] [against]"

	// determine config path from args before full parsing
	configPath := resolveFlagPath(args, "config", "REVDIFF_CONFIG", defaultConfigPath)

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

	if len(opts.Include) > 0 && len(opts.Only) > 0 {
		return options{}, errors.New("--include cannot be used with --only")
	}

	if opts.CompactContext <= 0 {
		return options{}, errors.New("--compact-context must be >= 1")
	}

	if opts.Description != "" && opts.DescriptionFile != "" {
		return options{}, errors.New("--description and --description-file are mutually exclusive")
	}

	if err := validateStdinFlags(opts); err != nil {
		return options{}, err
	}

	absOld, absNew, err := validateCompareFlag(opts)
	if err != nil {
		return options{}, err
	}
	opts.compareAbsOld = absOld
	opts.compareAbsNew = absNew

	return opts, nil
}

// dumpConfig writes the current config with defaults to the given writer.
func dumpConfig(args []string, w io.Writer) {
	var opts options
	p := flags.NewParser(&opts, flags.Default)
	iniParser := flags.NewIniParser(p)
	configPath := resolveFlagPath(args, "config", "REVDIFF_CONFIG", defaultConfigPath)
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

// resolveFlagPath determines a file path from CLI args, env var, or default location.
// it checks args for --flag value and --flag=value forms, falls back to envVar, then defaultFn.
func resolveFlagPath(args []string, flag, envVar string, defaultFn func() string) string {
	longFlag := "--" + flag
	for i, arg := range args {
		if arg == longFlag && i+1 < len(args) {
			return args[i+1]
		}
		if after, ok := strings.CutPrefix(arg, longFlag+"="); ok {
			return after
		}
	}
	if p := os.Getenv(envVar); p != "" {
		return p
	}
	return defaultFn()
}

// defaultConfigPath returns ~/.config/revdiff/config.
func defaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "config")
}

// defaultKeysPath returns ~/.config/revdiff/keybindings.
func defaultKeysPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "revdiff", "keybindings")
}
