package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui"
)

// maxDescriptionFileSize bounds --description-file reads. The popup is meant
// for a few paragraphs of prose context, not for arbitrary content, so 256KB
// is well above any realistic review description while keeping memory and
// later highlighting work bounded. Files exceeding the cap fail fast rather
// than load partially or OOM the highlighter.
const maxDescriptionFileSize = 256 * 1024

// reviewInfoInputs bundles the runtime values supplied alongside the parsed
// CLI options when constructing a ReviewInfoConfig. The fields are all
// produced post-parseArgs (workDir from VCS detection, vcsType from
// renderer setup, description from resolveDescription); collecting them in
// a single struct keeps reviewInfoFromOptions's signature stable as the
// popup gains more inputs and removes the same-typed-string swap risk of
// the prior positional-args shape.
type reviewInfoInputs struct {
	workDir     string
	vcsType     diff.VCSType
	description string
}

// reviewInfoFromOptions builds the production *ReviewInfoConfig threaded into
// ModelConfig.ReviewInfo. Production always returns a non-nil config so the
// review-info subsystem activates; focused tests pass nil directly to
// ModelConfig and bypass this constructor entirely (nil = subsystem off).
func reviewInfoFromOptions(opts options, in reviewInfoInputs) *ui.ReviewInfoConfig {
	ref := opts.ref()
	effectiveStaged := opts.Staged && in.vcsType == diff.VCSGit
	vcs := string(in.vcsType)
	if opts.Stdin {
		vcs = "stdin"
	} else if vcs == "" {
		vcs = "none"
	}
	stdinDisplayName := ""
	if opts.Stdin {
		stdinDisplayName = stdinName(opts.StdinName)
	}
	return &ui.ReviewInfoConfig{
		Description:    in.description,
		VCS:            vcs,
		WorkDir:        in.workDir,
		Ref:            ref,
		StdinName:      stdinDisplayName,
		Stdin:          opts.Stdin,
		Compare:        opts.compareAbsOld != "",
		Staged:         effectiveStaged,
		AllFiles:       opts.AllFiles,
		Only:           append([]string(nil), opts.Only...),
		Include:        append([]string(nil), opts.Include...),
		Exclude:        append([]string(nil), opts.Exclude...),
		Compact:        opts.Compact,
		CompactContext: opts.CompactContext,
	}
}

// resolveDescription returns the prose-description text for the info popup
// from either --description (literal string) or --description-file (path to
// a markdown file). parseArgs rejects the case where both flags are set; the
// resolver enforces the same invariant for defense-in-depth so any direct
// programmatic call sees the same contract. Returns "" when neither flag is
// set, which leaves the description section hidden in the popup.
//
// File-handling hardening: stats BEFORE opening (os.Open on a FIFO blocks
// until a writer connects, so the IsRegular guard would never be reached if
// we opened first), rejects anything that is not a regular file (FIFOs,
// devices, sockets, directories — Stat().Size() is not meaningful there and
// ReadFile may block forever or exhaust memory), then bounds the actual read
// with io.LimitReader so the size cap applies to the stream itself, not just
// to the possibly-stale Stat().Size() value. The window between Stat and
// Open is small and the worst case if a regular file is replaced with a
// FIFO mid-flight is the same blocking we used to have unconditionally.
func resolveDescription(opts options) (string, error) {
	if opts.Description != "" && opts.DescriptionFile != "" {
		return "", errors.New("--description and --description-file are mutually exclusive")
	}
	if opts.DescriptionFile == "" {
		return opts.Description, nil
	}
	fi, err := os.Stat(opts.DescriptionFile)
	if err != nil {
		return "", fmt.Errorf("stat --description-file %q: %w", opts.DescriptionFile, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("--description-file %q must be a regular file", opts.DescriptionFile)
	}
	f, err := os.Open(opts.DescriptionFile)
	if err != nil {
		return "", fmt.Errorf("open --description-file %q: %w", opts.DescriptionFile, err)
	}
	defer f.Close()

	// LimitReader caps the read at maxDescriptionFileSize+1 so we can detect
	// "exceeds cap" by checking len(data) > maxDescriptionFileSize after the
	// read, regardless of what Stat reported.
	data, err := io.ReadAll(io.LimitReader(f, maxDescriptionFileSize+1))
	if err != nil {
		return "", fmt.Errorf("read --description-file %q: %w", opts.DescriptionFile, err)
	}
	if len(data) > maxDescriptionFileSize {
		return "", fmt.Errorf("--description-file %q exceeds %d-byte cap", opts.DescriptionFile, maxDescriptionFileSize)
	}
	return string(data), nil
}
