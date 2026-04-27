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

	themesDir := defaultThemesDir()
	cat := theme.NewCatalog(themesDir)
	done, thErr := handleThemes(&opts, cat, os.Stdout, os.Stderr)
	if thErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", thErr)
		os.Exit(1)
	}
	if done {
		os.Exit(0)
	}

	if opts.DumpTheme {
		colors := collectColors(opts)
		th := theme.Theme{Colors: colors, ChromaStyle: opts.ChromaStyle}
		if err := th.Dump(os.Stdout); err != nil {
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
		renderer     ui.Renderer
		workDir      string
		gitRoot      string
		blamer       ui.Blamer
		untrackedFn  func() ([]string, error)
		commitLogger diff.CommitLogger
		err          error
	)

	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if !opts.NoMouse {
		programOptions = append(programOptions, tea.WithMouseCellMotion())
	}
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
		commitLogger = setup.commitLogger
	}

	if opts.Annotations != "" {
		if perr := preloadAnnotations(opts.Annotations, store, renderer, opts.ref(), opts.Staged, untrackedFn, workDir, os.Stderr); perr != nil {
			return perr
		}
	}

	// construct the three style types per D15: Resolver first, Renderer from Resolver, SGR is zero-value
	styleColors := optsToStyleColors(opts)
	var res style.Resolver
	if opts.NoColors {
		res = style.PlainResolver()
	} else {
		res = style.NewResolver(styleColors)
	}

	themesDir := defaultThemesDir()
	configPath := resolveFlagPath(os.Args[1:], "config", "REVDIFF_CONFIG", defaultConfigPath)
	themes := &themeCatalog{
		catalog:    theme.NewCatalog(themesDir),
		configPath: configPath,
	}

	model, err := ui.NewModel(ui.ModelConfig{
		Renderer:          renderer,
		Store:             store,
		Highlighter:       hl,
		StyleResolver:     res,
		StyleRenderer:     style.NewRenderer(res),
		SGR:               style.SGR{},
		WordDiffer:        worddiff.New(),
		Overlay:           overlay.NewManager(),
		Themes:            themes,
		Blamer:            blamer,
		LoadUntracked:     untrackedFn,
		Keymap:            km,
		CommitLog:         commitLogger,
		CommitsApplicable: commitsApplicable(opts, commitLogger),
		ReloadApplicable:  reloadApplicable(opts),
		CompactApplicable: compactApplicable(opts, renderer),
		NoColors:          opts.NoColors,
		NoStatusBar:       opts.NoStatusBar,
		NoConfirmDiscard:  opts.NoConfirmDiscard,
		Wrap:              opts.Wrap,
		Collapsed:         opts.Collapsed,
		Compact:           opts.Compact,
		CompactContext:    opts.CompactContext,
		CrossFileHunks:    opts.CrossFileHunks,
		LineNumbers:       opts.LineNumbers,
		ShowBlame:         opts.Blame,
		WordDiff:          opts.WordDiff,
		VimMotion:         opts.VimMotion,
		TabWidth:          opts.TabWidth,
		Ref:               opts.ref(),
		Staged:            opts.Staged,
		TreeWidthRatio:    opts.TreeWidth,
		Only:              opts.Only,
		WorkDir:           workDir,
		ActiveThemeName:   themes.catalog.ActiveName(opts.Theme),
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

// reloadApplicable returns false when --stdin is active: the stream has already
// been consumed and cannot be re-read. All other modes support reload.
func reloadApplicable(opts options) bool {
	return !opts.Stdin
}

// commitsApplicable returns true when the current invocation can show a
// commit-info popup: a VCS-backed log source must be present and the mode
// must be ref-based (no stdin, staged, all-files, or empty ref). Computed
// once in the composition root so the Model does not re-derive from CLI
// flags. --only is fine when combined with a ref in a real repo; the empty
// ref check excludes the standalone --only / FileReader case where the
// commitLogger is nil anyway.
func commitsApplicable(opts options, cl diff.CommitLogger) bool {
	if cl == nil {
		return false
	}
	if opts.Stdin || opts.Staged || opts.AllFiles {
		return false
	}
	return opts.ref() != ""
}

// compactApplicable returns true when the current invocation can shrink the
// VCS diff via the compact toggle. false for stdin (no VCS), all-files (no
// hunks to contextualize), and standalone file review via FileReader (pure
// context-only source with no underlying VCS). All other renderer shapes —
// *Git / *Hg / *Jj, with or without Fallback / Include / Exclude wrappers —
// qualify because the wrapper chain delegates FileDiff straight through to
// a VCS that honors contextLines.
func compactApplicable(opts options, r ui.Renderer) bool {
	if opts.Stdin || opts.AllFiles {
		return false
	}
	if _, ok := r.(*diff.FileReader); ok {
		return false
	}
	return true
}
