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

	Staged    bool `long:"staged" description:"show staged changes"`
	TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3" description:"file tree panel width in units (1-10, default 3 of 10)"`
	Version   bool `short:"V" long:"version" description:"show version info"`
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
	model := ui.NewModel(renderer, store, opts.Ref.Ref, opts.Staged, opts.TreeWidth)

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
