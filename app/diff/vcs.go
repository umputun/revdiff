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
	VCSJJ   VCSType = "jj"
	VCSNone VCSType = ""
)

// DetectVCS walks up from startDir looking for .jj, .git, or .hg directories.
// returns the VCS type and repo root path. If no VCS is found, returns VCSNone and empty string.
// precedence (checked in order at each directory): jj, git, hg. Jujutsu is commonly
// colocated with a .git directory; in that case jj wins so operations target the jj
// working-copy model rather than bypassing it through git.
func DetectVCS(startDir string) (VCSType, string) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return VCSNone, ""
	}

	for {
		// check .jj first — jj often colocates with .git; jj wins so we don't bypass the jj model
		if info, err := os.Stat(filepath.Join(dir, ".jj")); err == nil && info.IsDir() {
			return VCSJJ, dir
		}
		// .git can be a file in worktrees/submodules
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
