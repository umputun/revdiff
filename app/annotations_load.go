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
	f, err := os.Open(path) //nolint:gosec // user-supplied path is intentional
	if err != nil {
		return fmt.Errorf("open annotations file: %w", err)
	}
	defer f.Close()

	records, err := annotation.Parse(f)
	if err != nil {
		return fmt.Errorf("parse annotations: %w", err)
	}

	known, err := resolveKnownFiles(renderer, ref, staged, untrackedFn, warnOut)
	if err != nil {
		return err
	}

	// cache parsed FileDiff per file so we don't re-fetch on each annotation
	lineCache := make(map[string]map[lineKey]struct{})

	for _, a := range records {
		status, ok := known[a.File]
		if !ok {
			_, _ = fmt.Fprintf(warnOut, "warning: --annotations: file %q not in diff, dropping annotation\n", a.File)
			continue
		}
		if a.Line == 0 {
			store.Add(a)
			continue
		}
		lines := lookupLineSet(renderer, ref, staged, workDir, a.File, status, lineCache, warnOut)
		if _, ok := lines[lineKey{line: a.Line, kind: a.Type}]; !ok {
			_, _ = fmt.Fprintf(warnOut, "warning: --annotations: %s:%d (%s) not in diff, dropping\n", a.File, a.Line, a.Type)
			continue
		}
		store.Add(a)
	}
	return nil
}

// resolveKnownFiles returns the set of paths the preload should accept,
// mirroring ui.loadFiles: when running with no ref and no --staged, staged-only
// FileAdded entries are folded in if the unstaged set is empty, and untracked
// files (when untrackedFn is provided) are always folded in so the
// show-untracked UI toggle round-trips.
func resolveKnownFiles(renderer ui.Renderer, ref string, staged bool, untrackedFn func() ([]string, error),
	warnOut io.Writer,
) (map[string]diff.FileStatus, error) {
	files, err := renderer.ChangedFiles(ref, staged)
	if err != nil {
		return nil, fmt.Errorf("resolve diff for annotation preload: %w", err)
	}
	known := make(map[string]diff.FileStatus, len(files))
	for _, fe := range files {
		known[fe.Path] = fe.Status
	}
	// fold in untracked files so annotations against them round-trip; matches
	// ui.loadFiles which appends FileUntracked entries from loadUntracked.
	if untrackedFn != nil {
		if ut, utErr := untrackedFn(); utErr != nil {
			_, _ = fmt.Fprintf(warnOut, "warning: --annotations: list untracked files: %v\n", utErr)
		} else {
			for _, p := range ut {
				if _, ok := known[p]; !ok {
					known[p] = diff.FileUntracked
				}
			}
		}
	}
	if ref != "" || staged || len(files) > 0 {
		return known, nil
	}
	stagedFiles, sErr := renderer.ChangedFiles("", true)
	if sErr != nil {
		_, _ = fmt.Fprintf(warnOut, "warning: --annotations: resolve staged files: %v\n", sErr)
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
func lookupLineSet(renderer ui.Renderer, ref string, staged bool, workDir, file string, status diff.FileStatus,
	cache map[string]map[lineKey]struct{}, warnOut io.Writer,
) map[lineKey]struct{} {
	if lines, ok := cache[file]; ok {
		return lines
	}
	var (
		dl   []diff.DiffLine
		err  error
		used string
	)
	switch {
	case status == diff.FileUntracked && workDir != "":
		used = "read"
		dl, err = diff.ReadFileAsAdded(filepath.Join(workDir, file))
	default:
		used = "diff"
		fileStaged := staged
		if !staged && ref == "" && status == diff.FileAdded {
			fileStaged = true
		}
		dl, err = renderer.FileDiff(ref, file, fileStaged, 0)
	}
	var lines map[lineKey]struct{}
	if err != nil {
		_, _ = fmt.Fprintf(warnOut, "warning: --annotations: %s diff for %q: %v\n", used, file, err)
		lines = map[lineKey]struct{}{}
	} else {
		lines = buildLineSet(dl)
	}
	cache[file] = lines
	return lines
}

type lineKey struct {
	line int
	kind string
}

// buildLineSet maps each renderable diff line to its (line-number, change-type)
// key. mirrors Model.diffLineNum: removals key on OldNum, all other change
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
