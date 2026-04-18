package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui"
)

type vcsSetup struct {
	renderer     ui.Renderer
	gitRoot      string // set only when VCS is git; used by history module to run git commands
	workDir      string
	blamer       ui.Blamer
	untrackedFn  func() ([]string, error)
	commitLogger diff.CommitLogger // VCS-backed commit log source; nil when VCS lacks the capability
}

// setupVCSRenderer detects the VCS and creates the appropriate renderer, blamer, and untracked function.
func setupVCSRenderer(opts options) (vcsSetup, error) {
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		cwd = "."
	}
	vcsType, vcsRoot := diff.DetectVCS(cwd)

	switch vcsType {
	case diff.VCSGit:
		g := diff.NewGit(vcsRoot)
		r, workDir, err := makeGitRenderer(g, opts, vcsRoot)
		if err != nil {
			return vcsSetup{}, err
		}
		return vcsSetup{renderer: r, gitRoot: vcsRoot, workDir: workDir, blamer: g, untrackedFn: g.UntrackedFiles, commitLogger: g}, nil
	case diff.VCSHg:
		if opts.Staged {
			fmt.Fprintln(os.Stderr, "warning: --staged ignored in mercurial repository (no staging area)")
		}
		h := diff.NewHg(vcsRoot)
		r, workDir, err := makeHgRenderer(h, opts, vcsRoot)
		if err != nil {
			return vcsSetup{}, err
		}
		return vcsSetup{renderer: r, workDir: workDir, blamer: h, untrackedFn: h.UntrackedFiles, commitLogger: h}, nil
	case diff.VCSJJ:
		if opts.Staged {
			fmt.Fprintln(os.Stderr, "warning: --staged ignored in jujutsu repository (no staging area)")
		}
		jj := diff.NewJj(vcsRoot)
		r, workDir, err := makeJjRenderer(jj, opts, vcsRoot)
		if err != nil {
			return vcsSetup{}, err
		}
		return vcsSetup{renderer: r, workDir: workDir, blamer: jj, untrackedFn: jj.UntrackedFiles, commitLogger: jj}, nil
	default:
		r, workDir, err := makeNoVCSRenderer(opts.Only, cwd)
		if err != nil {
			return vcsSetup{}, err
		}
		return vcsSetup{renderer: r, workDir: workDir}, nil
	}
}

// makeGitRenderer selects the appropriate git renderer based on flags.
// reuses the provided *Git instance as the default renderer to avoid double allocation.
func makeGitRenderer(g *diff.Git, opts options, repoRoot string) (ui.Renderer, string, error) { //nolint:unparam // error kept for consistency with makeHgRenderer/makeNoVCSRenderer
	var r ui.Renderer
	switch {
	case opts.AllFiles:
		r = diff.NewDirectoryReader(repoRoot)
	case len(opts.Only) > 0:
		r = diff.NewFallbackRenderer(g, opts.Only, repoRoot)
	default:
		r = g
	}
	return wrapFilters(r, opts), repoRoot, nil
}

// makeHgRenderer selects the appropriate mercurial renderer based on flags.
// reuses the provided *Hg instance as the default renderer to avoid double allocation.
func makeHgRenderer(h *diff.Hg, opts options, repoRoot string) (ui.Renderer, string, error) {
	var r ui.Renderer
	switch {
	case opts.AllFiles:
		return nil, "", errors.New("--all-files is not supported in mercurial repositories")
	case len(opts.Only) > 0:
		r = diff.NewFallbackRenderer(h, opts.Only, repoRoot)
	default:
		r = h
	}
	return wrapFilters(r, opts), repoRoot, nil
}

// makeJjRenderer selects the appropriate jujutsu renderer based on flags.
// reuses the provided *Jj instance as the default renderer to avoid double allocation.
func makeJjRenderer(j *diff.Jj, opts options, repoRoot string) (ui.Renderer, string, error) { //nolint:unparam // error kept for consistency with makeGitRenderer/makeHgRenderer
	var r ui.Renderer
	switch {
	case opts.AllFiles:
		r = diff.NewJjDirectoryReader(repoRoot)
	case len(opts.Only) > 0:
		r = diff.NewFallbackRenderer(j, opts.Only, repoRoot)
	default:
		r = j
	}
	return wrapFilters(r, opts), repoRoot, nil
}

// wrapFilters applies include/exclude filters to a renderer based on opts.
func wrapFilters(r ui.Renderer, opts options) ui.Renderer {
	if len(opts.Include) > 0 {
		r = diff.NewIncludeFilter(r, opts.Include)
	}
	if len(opts.Exclude) > 0 {
		r = diff.NewExcludeFilter(r, opts.Exclude)
	}
	return r
}

// makeNoVCSRenderer creates a renderer when no VCS is detected.
// No-VCS mode requires --only, which is mutually exclusive with --include.
// --exclude is a no-op here (FileReader only returns the --only files).
func makeNoVCSRenderer(only []string, cwd string) (ui.Renderer, string, error) {
	if len(only) == 0 {
		return nil, "", errors.New("no git, mercurial, or jujutsu repository found (use --only to review standalone files)")
	}
	return diff.NewFileReader(only, cwd), cwd, nil
}
