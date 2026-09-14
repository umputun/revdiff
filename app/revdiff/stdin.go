package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui"
)

const defaultScratchBufferName = "scratch-buffer"

// maxStdinSize bounds the in-memory buffer used for stdin diff detection and
// parsing. The pre-PR path streamed stdin straight into context lines and only
// the final []DiffLine survived; the multi-file path needs the full byte slice
// (plus its string view) resident for sniff+split+parse. The cap protects
// against unbounded reads like `cat /dev/zero | revdiff --stdin` while still
// leaving plenty of room for real-world multi-file patches (64 MiB easily
// covers an entire repository's worth of churn).
const maxStdinSize = 64 * 1024 * 1024

type stdinStat interface {
	Stat() (os.FileInfo, error)
}

func validateStdinFlags(opts options) error {
	if opts.StdinName != "" && !opts.Stdin {
		return errors.New("--stdin-name requires --stdin")
	}
	if !opts.Stdin {
		return nil
	}
	if opts.Refs.Base != "" || opts.Refs.Against != "" {
		return errors.New("--stdin cannot be used with refs")
	}
	if opts.Staged {
		return errors.New("--stdin cannot be used with --staged")
	}
	if len(opts.Only) > 0 {
		return errors.New("--stdin cannot be used with --only")
	}
	if opts.AllFiles {
		return errors.New("--stdin cannot be used with --all-files")
	}
	if len(opts.Exclude) > 0 {
		return errors.New("--stdin cannot be used with --exclude")
	}
	if len(opts.Include) > 0 {
		return errors.New("--stdin cannot be used with --include")
	}
	if opts.Annotations != "" {
		return errors.New("--stdin cannot be used with --annotations")
	}
	return nil
}

func validateStdinInput(opts options, stdin stdinStat) error {
	if !opts.Stdin {
		return nil
	}
	info, err := stdin.Stat()
	if err != nil {
		return fmt.Errorf("stat stdin: %w", err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return errors.New("--stdin requires piped or redirected input")
	}
	return nil
}

func stdinName(name string) string {
	if name == "" {
		return defaultScratchBufferName
	}
	return name
}

func openTTY() (*os.File, error) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return nil, fmt.Errorf("open /dev/tty: %w", err)
	}
	return tty, nil
}

// readStdinCapped reads up to maxStdinSize bytes from r and returns the
// content as a string. Reads one byte past the cap so callers can detect
// overflow without relying on possibly-stale stream length.
func readStdinCapped(r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxStdinSize+1))
	if err != nil {
		return "", fmt.Errorf("read stdin: %w", err)
	}
	if len(data) > maxStdinSize {
		return "", fmt.Errorf("--stdin input exceeds %d-byte cap", maxStdinSize)
	}
	return string(data), nil
}

// selectStdinRenderer picks the renderer for piped stdin content: the
// multi-file unified-diff reader when the content sniffs as a diff and parses
// cleanly, otherwise the raw-text StdinReader. Non-sentinel errors from the
// multi-file path are logged before fall-through so partial parse failures
// are surfaced rather than silently routed to raw text.
func selectStdinRenderer(opts options, content string) (ui.Renderer, error) {
	multi, mErr := diff.NewMultiFileStdinReader(content)
	switch {
	case mErr == nil:
		return multi, nil
	case errors.Is(mErr, diff.ErrNotUnifiedDiff):
		// expected fall-through for plain text input
	default:
		log.Printf("[WARN] stdin: multi-file diff parse failed, falling back to raw text: %v", mErr)
	}

	renderer, err := diff.NewStdinReaderFromReader(stdinName(opts.StdinName), strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("create stdin reader: %w", err)
	}
	return renderer, nil
}

func prepareStdinMode(opts options, stdin *os.File) (ui.Renderer, *os.File, error) {
	if err := validateStdinInput(opts, stdin); err != nil {
		return nil, nil, err
	}

	content, err := readStdinCapped(stdin)
	if err != nil {
		return nil, nil, err
	}

	renderer, err := selectStdinRenderer(opts, content)
	if err != nil {
		return nil, nil, err
	}

	tty, err := openTTY()
	if err != nil {
		return nil, nil, err
	}
	return renderer, tty, nil
}
