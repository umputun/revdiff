package history

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Params holds parameters for saving a review history entry.
type Params struct {
	Annotations    string   // formatted annotations from Store.FormatOutput()
	Path           string   // full repo/file path or "stdin"
	Ref            string   // git ref (e.g. "master..HEAD") or empty
	Staged         bool     // whether --staged was used
	GitRoot        string   // git repo root, empty when git unavailable
	AnnotatedFiles []string // files with annotations, from Store.Files()
	SubDir         string   // history subdirectory name override; when empty, derived from Path
}

// Service manages review history persistence.
type Service struct {
	baseDir string // base history directory, empty = default (~/.config/revdiff/history/)
}

// New creates a history service with the given base directory.
// if baseDir is empty, defaults to ~/.config/revdiff/history/.
func New(baseDir string) *Service {
	return &Service{baseDir: baseDir}
}

// Save writes a review history entry to disk. Errors are logged to stderr, never returned.
// This is a safety net for preserving annotations — it must never fail the process.
func (s *Service) Save(p Params) {
	if p.Annotations == "" {
		return
	}

	dir := s.historyDir(p)
	if dir == "" {
		log.Printf("[WARN] history: cannot determine history directory")
		return
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("[WARN] history: create directory %s: %v", dir, err)
		return
	}

	now := time.Now()
	fname := filepath.Join(dir, now.Format("2006-01-02T15-04-05.000")+".md")

	var buf strings.Builder
	fmt.Fprintf(&buf, "# Review: %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&buf, "path: %s\n", p.Path)
	if p.Ref != "" {
		fmt.Fprintf(&buf, "refs: %s\n", p.Ref)
	}
	if hash := s.gitCommitHash(p.GitRoot); hash != "" {
		fmt.Fprintf(&buf, "commit: %s\n", hash)
	}

	buf.WriteString("\n## Annotations\n\n")
	buf.WriteString(p.Annotations)

	if diffOut := s.gitDiff(p); diffOut != "" {
		buf.WriteString("\n---\n\n## Diff\n\n")
		buf.WriteString(diffOut)
		if !strings.HasSuffix(diffOut, "\n") {
			buf.WriteString("\n")
		}
	}

	if err := os.WriteFile(fname, []byte(buf.String()), 0o600); err != nil {
		log.Printf("[WARN] history: write %s: %v", fname, err)
		return
	}
}

// historyDir returns the directory for saving history files.
// uses s.baseDir if set, otherwise ~/.config/revdiff/history/, with repo basename appended.
func (s *Service) historyDir(p Params) string {
	base := s.baseDir
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config", "revdiff", "history")
	}

	subdir := "unknown"
	switch {
	case p.SubDir != "":
		subdir = p.SubDir
	case p.Path == "stdin":
		subdir = "stdin"
	case p.Path != "":
		subdir = filepath.Base(p.Path)
	}
	return filepath.Join(base, subdir)
}

// gitDiff runs git diff for annotated files and returns the raw output.
// returns empty string if git is unavailable or on error.
// files outside the git repo are filtered out to prevent git from failing
// with "is outside repository" error, which would lose the diff for all files.
func (s *Service) gitDiff(p Params) string {
	if p.GitRoot == "" || len(p.AnnotatedFiles) == 0 {
		return ""
	}

	repoFiles := s.filterRepoFiles(p.GitRoot, p.AnnotatedFiles)
	if len(repoFiles) == 0 {
		return ""
	}

	args := []string{"diff", "--no-color", "--no-ext-diff"}
	if p.Staged {
		args = append(args, "--cached")
	}
	if p.Ref != "" {
		args = append(args, p.Ref)
	}
	args = append(args, "--")
	args = append(args, repoFiles...)

	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = p.GitRoot
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			log.Printf("[WARN] history: git diff: %s", strings.TrimSpace(string(exitErr.Stderr)))
		} else {
			log.Printf("[WARN] history: git diff: %v", err)
		}
		return ""
	}
	return string(out)
}

// gitCommitHash returns the short commit hash from the git root.
// returns empty string if git is unavailable or on error.
func (s *Service) gitCommitHash(gitRoot string) string {
	if gitRoot == "" {
		return ""
	}
	cmd := exec.CommandContext(context.Background(), "git", "rev-parse", "--short", "HEAD")
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// filterRepoFiles returns only files that are inside the git repo root,
// canonicalized to repo-relative paths. This ensures git receives clean pathspecs
// regardless of original path format (absolute, relative with .., etc).
func (s *Service) filterRepoFiles(gitRoot string, files []string) []string {
	result := make([]string, 0, len(files))
	for _, f := range files {
		absPath := f
		if !filepath.IsAbs(f) {
			absPath = filepath.Join(gitRoot, f)
		}
		rel, err := filepath.Rel(gitRoot, absPath)
		if err != nil || !filepath.IsLocal(rel) {
			continue // outside repo
		}
		result = append(result, rel)
	}
	return result
}
