package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/highlight"
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
	ChromaStyle      string   `long:"chroma-style" ini-name:"chroma-style" env:"REVDIFF_CHROMA_STYLE" default:"catppuccin-macchiato" description:"chroma style for syntax highlighting"`
	Only             []string `long:"only" short:"F" no-ini:"true" description:"show only these files (may be repeated)"`
	Output           string   `long:"output" short:"o" env:"REVDIFF_OUTPUT" no-ini:"true" description:"write annotations to file instead of stdout"`
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
		os.Exit(1)
	}

	if opts.DumpConfig {
		dumpConfig(os.Args[1:], os.Stdout)
		os.Exit(0)
	}

	if opts.Version {
		fmt.Printf("version: %s\ngo: %s\n", revision, runtime.Version())
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

	return opts, nil
}

// dumpConfig writes the current config with defaults to the given writer.
func dumpConfig(args []string, w *os.File) {
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

func run(opts options) error {
	gitRoot, gitErr := gitTopLevel()
	renderer, workDir, err := makeRenderer(opts.Only, gitRoot, gitErr)
	if err != nil {
		return err
	}
	store := annotation.NewStore()
	hl := highlight.New(opts.ChromaStyle, !opts.NoColors)
	model := ui.NewModel(renderer, store, hl, ui.ModelConfig{
		NoColors:         opts.NoColors,
		NoStatusBar:      opts.NoStatusBar,
		NoConfirmDiscard: opts.NoConfirmDiscard,
		Wrap:             opts.Wrap,
		Collapsed:        opts.Collapsed,
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

// makeRenderer selects the appropriate renderer based on git availability and --only flags.
// if git is available with --only, it wraps diff.Git with FallbackRenderer.
// if git is available without --only, it returns diff.Git directly.
// if git is unavailable and --only is set, it uses FileReader to read files directly from disk.
// if git is unavailable and --only is not set, it returns an error.
func makeRenderer(only []string, gitRoot string, gitErr error) (ui.Renderer, string, error) {
	if gitErr == nil {
		inner := diff.NewGit(gitRoot)
		if len(only) > 0 {
			return diff.NewFallbackRenderer(inner, only, gitRoot), gitRoot, nil
		}
		return inner, gitRoot, nil
	}

	// no git repo available
	if len(only) > 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", fmt.Errorf("get working directory: %w", err)
		}
		return diff.NewFileReader(only, cwd), cwd, nil
	}

	return nil, "", fmt.Errorf("find git root: %w", gitErr)
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
