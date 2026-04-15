package main

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jessevdk/go-flags"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/highlight"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/theme"
	"github.com/umputun/revdiff/app/ui"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

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
		km := keymap.LoadOrDefault(resolveFlagPath(os.Args[1:], "keys", "REVDIFF_KEYS", defaultKeysPath))
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

func run(opts options) error {
	store := annotation.NewStore()
	hl := highlight.New(opts.ChromaStyle, !opts.NoColors)
	keysPath := opts.Keys
	if keysPath == "" {
		keysPath = defaultKeysPath()
	}
	km := keymap.LoadOrDefault(keysPath)

	var (
		renderer    ui.Renderer
		workDir     string
		gitRoot     string
		blamer      ui.Blamer
		untrackedFn func() ([]string, error)
		err         error
	)

	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.Stdin {
		var tty *os.File
		renderer, tty, err = prepareStdinMode(opts, os.Stdin)
		if err != nil {
			return err
		}
		defer tty.Close()
		programOptions = append(programOptions, tea.WithInput(tty))
	} else {
		var setup vcsSetup
		setup, err = setupVCSRenderer(opts)
		if err != nil {
			return err
		}
		renderer = setup.renderer
		gitRoot = setup.gitRoot
		workDir = setup.workDir
		blamer = setup.blamer
		untrackedFn = setup.untrackedFn
	}

	// construct the three style types per D15: Resolver first, Renderer from Resolver, SGR is zero-value
	styleColors := style.Colors{
		Accent:       opts.Colors.Accent,
		Border:       opts.Colors.Border,
		Normal:       opts.Colors.Normal,
		Muted:        opts.Colors.Muted,
		SelectedFg:   opts.Colors.SelectedFg,
		SelectedBg:   opts.Colors.SelectedBg,
		Annotation:   opts.Colors.Annotation,
		CursorFg:     opts.Colors.CursorFg,
		CursorBg:     opts.Colors.CursorBg,
		AddFg:        opts.Colors.AddFg,
		AddBg:        opts.Colors.AddBg,
		RemoveFg:     opts.Colors.RemoveFg,
		RemoveBg:     opts.Colors.RemoveBg,
		WordAddBg:    opts.Colors.WordAddBg,
		WordRemoveBg: opts.Colors.WordRemoveBg,
		ModifyFg:     opts.Colors.ModifyFg,
		ModifyBg:     opts.Colors.ModifyBg,
		TreeBg:       opts.Colors.TreeBg,
		DiffBg:       opts.Colors.DiffBg,
		StatusFg:     opts.Colors.StatusFg,
		StatusBg:     opts.Colors.StatusBg,
		SearchFg:     opts.Colors.SearchFg,
		SearchBg:     opts.Colors.SearchBg,
	}
	var res style.Resolver
	if opts.NoColors {
		res = style.PlainResolver()
	} else {
		res = style.NewResolver(styleColors)
	}

	model, err := ui.NewModel(ui.ModelConfig{
		Renderer:         renderer,
		Store:            store,
		Highlighter:      hl,
		StyleResolver:    res,
		StyleRenderer:    style.NewRenderer(res),
		SGR:              style.SGR{},
		WordDiffer:       worddiff.New(),
		Overlay:          overlay.NewManager(),
		Blamer:           blamer,
		LoadUntracked:    untrackedFn,
		Keymap:           km,
		NoColors:         opts.NoColors,
		NoStatusBar:      opts.NoStatusBar,
		NoConfirmDiscard: opts.NoConfirmDiscard,
		Wrap:             opts.Wrap,
		Collapsed:        opts.Collapsed,
		CrossFileHunks:   opts.CrossFileHunks,
		LineNumbers:      opts.LineNumbers,
		ShowBlame:        opts.Blame,
		WordDiff:         opts.WordDiff,
		TabWidth:         opts.TabWidth,
		Ref:              opts.ref(),
		Staged:           opts.Staged,
		TreeWidthRatio:   opts.TreeWidth,
		Only:             opts.Only,
		WorkDir:          workDir,
		ThemesDir:        defaultThemesDir(),
		ConfigPath:       resolveFlagPath(os.Args[1:], "config", "REVDIFF_CONFIG", defaultConfigPath),
		ActiveThemeName:  theme.ActiveName(opts.Theme),
		NewFileTree: func(entries []diff.FileEntry) ui.FileTreeComponent {
			return sidepane.NewFileTree(entries)
		},
		ParseTOC: func(lines []diff.DiffLine, filename string) ui.TOCComponent {
			toc := sidepane.ParseTOC(lines, filename)
			if toc == nil {
				return nil // collapse typed-nil *TOC into truly nil interface
			}
			return toc
		},
	})
	if err != nil {
		return fmt.Errorf("create model: %w", err)
	}

	p := tea.NewProgram(model, programOptions...)
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

	saveHistory(histReq{opts: opts, annotations: output, gitRoot: gitRoot, workDir: workDir, files: m.Store().Files()})

	if opts.Output != "" {
		if err := os.WriteFile(opts.Output, []byte(output), 0o600); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		return nil
	}
	fmt.Print(output)
	return nil
}
