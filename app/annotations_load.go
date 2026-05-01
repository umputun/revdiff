package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui"
)

// maxAnnotationsFileSize caps the bytes read from --annotations. Annotation
// files aggregate many records (LLM reviews, history exports) so 1 MiB is a
// generous practical ceiling; anything larger almost certainly indicates the
// flag was pointed at the wrong file.
const maxAnnotationsFileSize = 1 << 20

// preloader bundles the inputs and warning sink for --annotations preload.
// All helpers are methods so call-sites stay free of the wide parameter
// lists earlier shapes accumulated.
type preloader struct {
	store       *annotation.Store
	renderer    ui.Renderer
	ref         string
	staged      bool
	untrackedFn func() ([]string, error)
	workDir     string
	warnOut     io.Writer

	// lineCache memoises the (line, change-type) set per file so
	// repeated annotations on the same file do not re-fetch its diff.
	lineCache map[string]map[lineKey]struct{}
}

type lineKey struct {
	line int
	kind string
}

// preloadAnnotations parses the markdown file at path (same format as
// Store.FormatOutput) and feeds each record through store.Add after dropping
// orphans against the resolved diff. file-level records (Line == 0) require
// only that the file be present in ChangedFiles. Line-scoped records require
// both the file and a matching DiffLine (same line number for the record's
// change type) to be present. Untracked files (surfaced in the UI via the
// show-untracked toggle) are folded in via untrackedFn so annotations saved
// against them round-trip; their line-set is read from disk as all-added
// lines, mirroring ui.resolveEmptyDiff. Dropped records are warned to warnOut.
func preloadAnnotations(path string, store *annotation.Store, renderer ui.Renderer, ref string, staged bool,
	untrackedFn func() ([]string, error), workDir string, warnOut io.Writer,
) error {
	records, err := readAnnotationsFile(path)
	if err != nil {
		return err
	}

	p := &preloader{
		store:       store,
		renderer:    renderer,
		ref:         ref,
		staged:      staged,
		untrackedFn: untrackedFn,
		workDir:     workDir,
		warnOut:     warnOut,
		lineCache:   make(map[string]map[lineKey]struct{}),
	}
	return p.load(records)
}

// readAnnotationsFile rejects non-regular files (FIFO, device, anything that
// would block os.Open) and oversize inputs before parsing. The size guard
// runs first against info.Size() so we never start scanning a 100 MiB file
// just to reject it; io.LimitReader is layered on top as belt-and-braces in
// case the file grows between Stat and Open.
func readAnnotationsFile(path string) ([]annotation.Annotation, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("open annotations file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("open annotations file: %s: not a regular file", path)
	}
	if info.Size() > maxAnnotationsFileSize {
		return nil, fmt.Errorf("annotations file %s exceeds %d bytes (got %d)", path, maxAnnotationsFileSize, info.Size())
	}
	f, err := os.Open(path) //nolint:gosec // user-supplied path is intentional
	if err != nil {
		return nil, fmt.Errorf("open annotations file: %w", err)
	}
	defer f.Close()

	records, err := annotation.Parse(io.LimitReader(f, maxAnnotationsFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("parse annotations: %w", err)
	}
	return records, nil
}

func (p *preloader) load(records []annotation.Annotation) error {
	known, err := p.resolveKnownFiles()
	if err != nil {
		return err
	}

	for _, a := range records {
		// Sanitize the comment text before it reaches Store.Add: the
		// preload source is user- or LLM-supplied and may carry stray
		// ANSI / CR-overwrite / C1 bytes that Renderer.AnnotationInline
		// wraps into the TUI verbatim. Mirrors diff.SanitizeCommitText
		// usage on commit Author/Subject/Body.
		a.Comment = diff.SanitizeCommitText(a.Comment)

		status, ok := known[a.File]
		if !ok {
			p.warnf("warning: --annotations: file %q not in diff, dropping annotation\n", a.File)
			continue
		}
		if a.Line == 0 {
			p.store.Add(a)
			continue
		}
		lines := p.lookupLineSet(a.File, status)
		if _, ok := lines[lineKey{line: a.Line, kind: a.Type}]; !ok {
			p.warnf("warning: --annotations: %s:%d (%s) not in diff, dropping\n", a.File, a.Line, a.Type)
			continue
		}
		p.store.Add(a)
	}
	return nil
}

// resolveKnownFiles returns the set of paths the preload should accept.
// Mirrors ui.loadFiles' assembly of the visible file set with one deliberate
// divergence: untracked files are always folded in here, whereas the UI only
// surfaces them when its show-untracked toggle is on. The preload is
// upstream of the toggle, so it has to accept either viewing mode for the
// round-trip to be lossless.
//
// When running with no ref and no --staged, staged-only FileAdded entries
// are folded in if the unstaged set is empty (matches the UI's empty-diff
// fallback). Files renamed since the annotations file was generated are
// keyed under their old path and will orphan-drop here — a known limitation
// of the path-based format.
func (p *preloader) resolveKnownFiles() (map[string]diff.FileStatus, error) {
	files, err := p.renderer.ChangedFiles(p.ref, p.staged)
	if err != nil {
		return nil, fmt.Errorf("resolve diff for annotation preload: %w", err)
	}
	known := make(map[string]diff.FileStatus, len(files))
	for _, fe := range files {
		known[fe.Path] = fe.Status
	}
	if p.untrackedFn != nil {
		if ut, utErr := p.untrackedFn(); utErr != nil {
			p.warnf("warning: --annotations: list untracked files: %v\n", utErr)
		} else {
			for _, path := range ut {
				if _, ok := known[path]; !ok {
					known[path] = diff.FileUntracked
				}
			}
		}
	}
	if p.ref != "" || p.staged || len(files) > 0 {
		return known, nil
	}
	stagedFiles, sErr := p.renderer.ChangedFiles("", true)
	if sErr != nil {
		p.warnf("warning: --annotations: resolve staged files: %v\n", sErr)
		return known, nil
	}
	for _, fe := range stagedFiles {
		if _, ok := known[fe.Path]; ok {
			continue
		}
		if fe.Status == diff.FileAdded {
			known[fe.Path] = fe.Status
		}
	}
	return known, nil
}

// lookupLineSet returns the cached line-set for file, fetching FileDiff on
// miss. Mirrors ui.resolveEmptyDiff: staged-only FileAdded entries retry with
// --cached when the request was unstaged, and FileUntracked entries are read
// from disk as all-added lines.
func (p *preloader) lookupLineSet(file string, status diff.FileStatus) map[lineKey]struct{} {
	if lines, ok := p.lineCache[file]; ok {
		return lines
	}
	var (
		dl   []diff.DiffLine
		err  error
		used string
	)
	switch {
	case status == diff.FileUntracked && p.workDir != "":
		used = "read"
		dl, err = diff.ReadFileAsAdded(filepath.Join(p.workDir, file))
	default:
		used = "diff"
		fileStaged := p.staged
		if !p.staged && p.ref == "" && status == diff.FileAdded {
			fileStaged = true
		}
		dl, err = p.renderer.FileDiff(p.ref, file, fileStaged, 0)
	}
	var lines map[lineKey]struct{}
	if err != nil {
		p.warnf("warning: --annotations: %s diff for %q: %v\n", used, file, err)
		lines = map[lineKey]struct{}{}
	} else {
		lines = buildLineSet(dl)
	}
	p.lineCache[file] = lines
	return lines
}

func (p *preloader) warnf(format string, args ...any) {
	_, _ = fmt.Fprintf(p.warnOut, format, args...)
}

// buildLineSet maps each renderable diff line to its (line-number, change-type)
// key. Mirrors Model.diffLineNum: removals key on OldNum, all other change
// types key on NewNum.
func buildLineSet(lines []diff.DiffLine) map[lineKey]struct{} {
	out := make(map[lineKey]struct{}, len(lines))
	for _, dl := range lines {
		if dl.ChangeType == diff.ChangeDivider {
			continue
		}
		var n int
		switch dl.ChangeType {
		case diff.ChangeRemove:
			n = dl.OldNum
		default:
			n = dl.NewNum
		}
		if n == 0 {
			continue
		}
		out[lineKey{line: n, kind: string(dl.ChangeType)}] = struct{}{}
	}
	return out
}
