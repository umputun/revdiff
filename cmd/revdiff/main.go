package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/revdiff/annotation"
	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/ui"
)

var opts struct {
	Ref struct {
		Ref string `positional-arg-name:"ref" description:"git ref to diff against (default: uncommitted changes)"`
	} `positional-args:"yes"`

	Staged    bool `long:"staged" env:"REVDIFF_STAGED" description:"show staged changes"`
	TreeWidth int  `long:"tree-width" env:"REVDIFF_TREE_WIDTH" default:"3" description:"file tree panel width in units (1-10, default 3 of 10)"`
	Version   bool `short:"V" long:"version" description:"show version info"`

	Colors struct {
		Accent     string `long:"color-accent"      env:"REVDIFF_COLOR_ACCENT"      default:"#5f87ff" description:"active pane borders and directory names"`
		Border     string `long:"color-border"      env:"REVDIFF_COLOR_BORDER"      default:"#585858" description:"inactive pane borders"`
		Normal     string `long:"color-normal"      env:"REVDIFF_COLOR_NORMAL"      default:"#d0d0d0" description:"file entries and context lines"`
		Muted      string `long:"color-muted"       env:"REVDIFF_COLOR_MUTED"       default:"#6c6c6c" description:"line numbers and status bar"`
		SelectedFg string `long:"color-selected-fg" env:"REVDIFF_COLOR_SELECTED_FG" default:"#ffffaf" description:"selected file text color"`
		SelectedBg string `long:"color-selected-bg" env:"REVDIFF_COLOR_SELECTED_BG" default:"#303030" description:"selected file background color"`
		Annotation string `long:"color-annotation"  env:"REVDIFF_COLOR_ANNOTATION"  default:"#ffd700" description:"annotation text and markers"`
		CursorBg   string `long:"color-cursor-bg"   env:"REVDIFF_COLOR_CURSOR_BG"   default:"#3a3a3a" description:"diff cursor line background"`
		AddFg      string `long:"color-add-fg"      env:"REVDIFF_COLOR_ADD_FG"      default:"#87d787" description:"added line text color"`
		AddBg      string `long:"color-add-bg"      env:"REVDIFF_COLOR_ADD_BG"      default:"#022800" description:"added line background color"`
		RemoveFg   string `long:"color-remove-fg"   env:"REVDIFF_COLOR_REMOVE_FG"   default:"#ff8787" description:"removed line text color"`
		RemoveBg   string `long:"color-remove-bg"   env:"REVDIFF_COLOR_REMOVE_BG"   default:"#3D0100" description:"removed line background color"`
	} `group:"color options"`
}

var revision = "unknown"

func main() {
	p := flags.NewParser(&opts, flags.Default)
	p.Usage = "[OPTIONS] [ref]"
	if _, err := p.Parse(); err != nil {
		var flagsErr *flags.Error
		if errors.As(err, &flagsErr) && flagsErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		os.Exit(1)
	}

	if opts.Version {
		fmt.Printf("version: %s\ngo: %s\n", revision, runtime.Version())
		os.Exit(0)
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	renderer := diff.NewGit(".")
	store := annotation.NewStore()
	model := ui.NewModel(renderer, store, ui.ModelConfig{
		Ref:            opts.Ref.Ref,
		Staged:         opts.Staged,
		TreeWidthRatio: opts.TreeWidth,
		Colors: ui.Colors{
			Accent:     opts.Colors.Accent,
			Border:     opts.Colors.Border,
			Normal:     opts.Colors.Normal,
			Muted:      opts.Colors.Muted,
			SelectedFg: opts.Colors.SelectedFg,
			SelectedBg: opts.Colors.SelectedBg,
			Annotation: opts.Colors.Annotation,
			CursorBg:   opts.Colors.CursorBg,
			AddFg:      opts.Colors.AddFg,
			AddBg:      opts.Colors.AddBg,
			RemoveFg:   opts.Colors.RemoveFg,
			RemoveBg:   opts.Colors.RemoveBg,
		},
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// output annotations to stdout
	if m, ok := finalModel.(ui.Model); ok {
		output := m.Store().FormatOutput()
		if output != "" {
			fmt.Print(output)
		}
	}
	return nil
}
