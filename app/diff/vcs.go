package diff

import (
	"os"
	"path/filepath"
)

// VCSType identifies a version control system.
type VCSType string

const (
	VCSGit  VCSType = "git"
	VCSHg   VCSType = "hg"
	VCSNone VCSType = ""
)

// DetectVCS walks up from startDir looking for .git or .hg directories.
// returns the VCS type and repo root path. If no VCS is found, returns VCSNone and empty string.
// when both .git and .hg exist in the same directory, git takes precedence.
func DetectVCS(startDir string) (VCSType, string) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return VCSNone, ""
	}

	for {
		// check .git first (takes precedence); .git can be a file in worktrees/submodules
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return VCSGit, dir
		}
		if info, err := os.Stat(filepath.Join(dir, ".hg")); err == nil && info.IsDir() {
			return VCSHg, dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return VCSNone, ""
		}
		dir = parent
	}
}
