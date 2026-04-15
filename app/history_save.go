package main

import (
	"path/filepath"

	"github.com/umputun/revdiff/app/history"
)

type histReq struct {
	opts        options
	annotations string
	gitRoot     string
	workDir     string
	files       []string
}

// saveHistory auto-saves review annotations and relevant diffs as a safety net.
// for non-git single-file --only mode, uses the full file path for the header
// and the parent directory basename for the history subdirectory.
func saveHistory(r histReq) {
	histPath := r.workDir
	if histPath == "" {
		histPath = r.gitRoot
	}
	var histSubDir string
	if r.gitRoot == "" && len(r.opts.Only) == 1 {
		if abs, err := filepath.Abs(r.opts.Only[0]); err == nil {
			histPath = abs
			histSubDir = filepath.Base(filepath.Dir(abs))
		}
	}
	if r.opts.Stdin {
		histPath = "stdin"
	}
	history.New(r.opts.HistoryDir).Save(history.Params{
		Annotations:    r.annotations,
		Path:           histPath,
		Ref:            r.opts.ref(),
		Staged:         r.opts.Staged,
		GitRoot:        r.gitRoot,
		AnnotatedFiles: r.files,
		SubDir:         histSubDir,
	})
}
