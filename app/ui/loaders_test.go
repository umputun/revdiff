package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/worddiff"
)

func TestModel_FilesLoaded(t *testing.T) {
	m := testModel(nil, nil)

	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "internal/handler.go"}, {Path: "internal/store.go"}, {Path: "main.go"}}})
	model := result.(Model)

	// tree should be populated with 3 files
	assert.Equal(t, 3, model.tree.TotalFiles())
	assert.NotNil(t, cmd) // should auto-select first file
}

func TestModel_FilesLoadedError(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = true
	m.filesLoaded = false

	result, cmd := m.Update(filesLoadedMsg{err: assert.AnError})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Equal(t, 0, model.tree.TotalFiles())
	// filesLoaded must flip even on error, otherwise View() stays on "loading files..." forever
	assert.True(t, model.filesLoaded, "filesLoaded must be set before the error early-return so the loading screen exits")
}

func TestModel_FilesLoaded_DropsStaleResponses(t *testing.T) {
	// regression: a slow first load (seq=0) must not overwrite the tree after
	// a newer load (seq=1) was issued — e.g. user toggled untracked immediately after startup.
	m := testModel(nil, nil)
	m.filesLoaded = false
	m.filesLoadSeq = 1 // simulate a newer load already dispatched (e.g. toggleUntracked)

	// stale response (seq=0) arrives first — must be dropped
	stale := []diff.FileEntry{{Path: "stale.go"}}
	result, cmd := m.Update(filesLoadedMsg{seq: 0, entries: stale})
	model := result.(Model)
	assert.Nil(t, cmd)
	assert.False(t, model.filesLoaded, "stale response must not flip filesLoaded")
	assert.Equal(t, 0, model.tree.TotalFiles(), "stale entries must not populate tree")
	assert.Equal(t, "loading files...", model.View(), "View must still show loading while the current load is pending")

	// fresh response (seq=1) arrives — accepted
	fresh := []diff.FileEntry{{Path: "fresh.go"}}
	result, _ = m.Update(filesLoadedMsg{seq: 1, entries: fresh})
	model = result.(Model)
	assert.True(t, model.filesLoaded)
	assert.Equal(t, 1, model.tree.TotalFiles())
}

func TestModel_ToggleUntrackedBumpsFilesLoadSeq(t *testing.T) {
	// regression: toggleUntracked must bump filesLoadSeq so any in-flight load
	// from before the toggle is treated as stale by handleFilesLoaded.
	m := testModel(nil, nil)
	m.loadUntracked = func() ([]string, error) { return nil, nil }
	before := m.filesLoadSeq
	cmd := m.toggleUntracked()
	require.NotNil(t, cmd)
	assert.Greater(t, m.filesLoadSeq, before, "toggleUntracked must bump filesLoadSeq")
}

func TestModel_FilesLoadedMultipleFiles(t *testing.T) {
	m := testModel(nil, nil)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}})
	model := result.(Model)

	assert.False(t, model.file.singleFile, "singleFile should be false for multiple files")
}

func TestModel_FileLoaded(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})

	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func main() {}", ChangeType: diff.ChangeAdd},
	}

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.Equal(t, "a.go", model.file.name)
	assert.Len(t, model.file.lines, 2)
}

func TestModel_ComputeFileStats(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		adds    int
		removes int
	}{
		{name: "empty diff", lines: nil, adds: 0, removes: 0},
		{name: "context only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "// comment", ChangeType: diff.ChangeContext},
		}, adds: 0, removes: 0},
		{name: "adds only", lines: []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeAdd},
		}, adds: 3, removes: 0},
		{name: "removes only", lines: []diff.DiffLine{
			{OldNum: 1, Content: "old1", ChangeType: diff.ChangeRemove},
			{OldNum: 2, Content: "old2", ChangeType: diff.ChangeRemove},
		}, adds: 0, removes: 2},
		{name: "mixed changes", lines: []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
			{OldNum: 2, Content: "old func", ChangeType: diff.ChangeRemove},
			{NewNum: 2, Content: "new func", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "// ok", ChangeType: diff.ChangeContext},
			{Content: "", ChangeType: diff.ChangeDivider},
			{NewNum: 10, Content: "added line", ChangeType: diff.ChangeAdd},
		}, adds: 2, removes: 1},
		{name: "dividers ignored", lines: []diff.DiffLine{
			{Content: "", ChangeType: diff.ChangeDivider},
			{Content: "", ChangeType: diff.ChangeDivider},
		}, adds: 0, removes: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.lines = tt.lines
			m.computeFileStats()
			assert.Equal(t, tt.adds, m.file.adds, "fileAdds")
			assert.Equal(t, tt.removes, m.file.removes, "fileRemoves")
		})
	}
}

func TestModel_FileStatsText(t *testing.T) {
	tests := []struct {
		name    string
		lines   []diff.DiffLine
		adds    int
		removes int
		want    string
	}{
		{name: "context only shows line count", lines: []diff.DiffLine{
			{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
			{NewNum: 3, Content: "line3", ChangeType: diff.ChangeContext},
		}, adds: 0, removes: 0, want: "3 lines"},
		{name: "diff shows adds/removes", lines: []diff.DiffLine{
			{NewNum: 1, Content: "added", ChangeType: diff.ChangeAdd},
			{NewNum: 2, Content: "ctx", ChangeType: diff.ChangeContext},
		}, adds: 1, removes: 0, want: "+1/-0"},
		{name: "empty diff shows +0/-0", lines: nil, adds: 0, removes: 0, want: "+0/-0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.lines = tt.lines
			m.file.adds = tt.adds
			m.file.removes = tt.removes
			assert.Equal(t, tt.want, m.fileStatsText())
		})
	}
}

func TestModel_FileLoadedComputesStats(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added1", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added2", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.file.loadSeq = 1

	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: lines})
	model := result.(Model)
	assert.Equal(t, 2, model.file.adds)
	assert.Equal(t, 1, model.file.removes)
}

func TestModel_FilterOnly(t *testing.T) {
	toEntries := func(paths ...string) []diff.FileEntry {
		entries := make([]diff.FileEntry, len(paths))
		for i, p := range paths {
			entries[i] = diff.FileEntry{Path: p}
		}
		return entries
	}

	t.Run("no filter returns all files", func(t *testing.T) {
		m := testModel(nil, nil)
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, files, m.filterOnly(files))
	})

	t.Run("exact path match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"ui/model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("suffix match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("multiple patterns", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"model.go", "README.md"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go", "README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern resolved against workDir", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"/repo/README.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("ui/model.go", "README.md")
		assert.Equal(t, []string{"README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern with subdirectory", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"/repo/ui/model.go"}
		m.cfg.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path outside workDir does not match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"/other/README.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("README.md", "ui/model.go")
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("absolute path suffix match via resolved relative", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"/repo/model.go"}
		m.cfg.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"nonexistent.go"}
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("dot-slash prefix matches relative entry", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"./CLAUDE.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("CLAUDE.md", "ui/model.go")
		assert.Equal(t, []string{"CLAUDE.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("dot-slash prefix matches absolute entry", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"./CLAUDE.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("/repo/CLAUDE.md", "/repo/ui/model.go")
		assert.Equal(t, []string{"/repo/CLAUDE.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("dot-slash prefix with subdirectory", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"./ui/model.go"}
		m.cfg.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("relative pattern matches absolute entry", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"CLAUDE.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("/repo/CLAUDE.md", "/repo/ui/model.go")
		assert.Equal(t, []string{"/repo/CLAUDE.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("dot-slash prefix no workDir falls through to exact/suffix", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"./CLAUDE.md"}
		files := toEntries("CLAUDE.md", "ui/model.go")
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("relative pattern escaping workDir does not match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.cfg.only = []string{"../other/CLAUDE.md"}
		m.cfg.workDir = "/repo"
		files := toEntries("CLAUDE.md", "ui/model.go")
		assert.Empty(t, m.filterOnly(files))
	})
}

func TestModel_FilterOnlyNoMatchShowsMessage(t *testing.T) {
	m := testModel(nil, nil)
	m.cfg.only = []string{"nonexistent.go"}
	m.ready = true
	m.layout.width = 80
	m.layout.height = 24
	m.layout.viewport = viewport.New(76, 20)

	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "ui/model.go"}, {Path: "diff/diff.go"}}})
	model := result.(Model)
	assert.Nil(t, cmd, "should not trigger file load when no files match")
	assert.Contains(t, model.layout.viewport.View(), "no files match --only filter")
}

func TestModel_UntrackedStartup(t *testing.T) {
	t.Run("ShowUntracked=true with LoadUntracked initializes showUntracked on", func(t *testing.T) {
		renderer := &mocks.RendererMock{}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			ShowUntracked:  true,
			LoadUntracked:  func() ([]string, error) { return nil, nil },
		})
		assert.True(t, m.modes.showUntracked)
	})

	t.Run("ShowUntracked=true without LoadUntracked stays off", func(t *testing.T) {
		renderer := &mocks.RendererMock{}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			ShowUntracked:  true,
			LoadUntracked:  nil,
		})
		assert.False(t, m.modes.showUntracked, "no-op when LoadUntracked is not wired (e.g. stdin, compare modes)")
	})

	t.Run("ShowUntracked=false stays off even with LoadUntracked wired", func(t *testing.T) {
		renderer := &mocks.RendererMock{}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			ShowUntracked:  false,
			LoadUntracked:  func() ([]string, error) { return nil, nil },
		})
		assert.False(t, m.modes.showUntracked)
	})

	t.Run("startup loadFiles appends untracked entries when ShowUntracked=true", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			ShowUntracked:  true,
			LoadUntracked:  func() ([]string, error) { return []string{"newfile.go"}, nil },
		})
		require.True(t, m.modes.showUntracked)

		msg := m.loadFiles()()
		flMsg, ok := msg.(filesLoadedMsg)
		require.True(t, ok)
		paths := make([]string, 0, len(flMsg.entries))
		for _, e := range flMsg.entries {
			paths = append(paths, e.Path)
		}
		assert.Contains(t, paths, "main.go")
		assert.Contains(t, paths, "newfile.go")
	})

	t.Run("staged + untracked compose: both appear in startup load", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				assert.True(t, staged, "staged flag should be propagated to ChangedFiles")
				return []diff.FileEntry{{Path: "staged.go", Status: diff.FileAdded}}, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			Staged:         true,
			ShowUntracked:  true,
			LoadUntracked:  func() ([]string, error) { return []string{"newfile.go"}, nil },
		})

		msg := m.loadFiles()()
		flMsg, ok := msg.(filesLoadedMsg)
		require.True(t, ok)
		paths := make([]string, 0, len(flMsg.entries))
		for _, e := range flMsg.entries {
			paths = append(paths, e.Path)
		}
		assert.Contains(t, paths, "staged.go")
		assert.Contains(t, paths, "newfile.go")
	})
}

func TestModel_UntrackedToggle(t *testing.T) {
	t.Run("toggle cycles showUntracked and reloads files", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return []diff.DiffLine{{Content: "line1", ChangeType: diff.ChangeContext, OldNum: 1, NewNum: 1}}, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			LoadUntracked: func() ([]string, error) {
				return []string{"newfile.go"}, nil
			},
		})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		// initially untracked is off
		assert.False(t, m.modes.showUntracked)

		// toggle on
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		assert.True(t, result.(Model).modes.showUntracked)
		assert.NotNil(t, cmd)

		// execute loadFiles command — should include untracked file
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		paths := make([]string, 0, len(flMsg.entries))
		for _, e := range flMsg.entries {
			paths = append(paths, e.Path)
		}
		assert.Contains(t, paths, "main.go")
		assert.Contains(t, paths, "newfile.go")

		// toggle off — use result from toggle on
		m = result.(Model)
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		assert.False(t, result.(Model).modes.showUntracked)
	})

	t.Run("status bar shows untracked icon when untracked is on", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		// ∅ is always in the icons list but inactive (muted) when showUntracked is false
		// check that ∅ becomes active when toggled
		m.modes.showUntracked = true
		iconsActive := m.statusModeIcons()
		assert.Contains(t, iconsActive, "∅")
	})

	t.Run("no untracked files when LoadUntracked is nil", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go"}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true
		m.modes.showUntracked = true

		// directly execute loadFiles to check behavior without toggling
		cmd := m.loadFiles()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1, "should only have the original file, no untracked")
	})

	t.Run("dedup: untracked file already in staged list", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{
			TreeWidthRatio: 3,
			LoadUntracked: func() ([]string, error) {
				return []string{"newfile.go", "other.go"}, nil
			},
		})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true
		m.modes.showUntracked = true

		cmd := m.loadFiles()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 2, "newfile.go should not be duplicated")
	})
}

func TestModel_StagedOnlyFiles(t *testing.T) {
	t.Run("staged-only new files included in file list", func(t *testing.T) {
		// simulate: working tree has no changes, but index has a new file
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}, nil
				}
				return nil, nil // no unstaged changes
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return []diff.DiffLine{{Content: "content", ChangeType: diff.ChangeAdd, NewNum: 1}}, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1)
		assert.Equal(t, "newfile.go", flMsg.entries[0].Path)
		assert.Equal(t, diff.FileAdded, flMsg.entries[0].Status)
	})

	t.Run("staged-only files not duplicated when already in unstaged list", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
				}
				return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Len(t, flMsg.entries, 1, "main.go should not be duplicated")
	})

	t.Run("staged-only new files are not merged when unstaged changes exist", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}, nil
				}
				return []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Equal(t, []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}, flMsg.entries)
	})

	t.Run("staged fetch failure logged as warning", func(t *testing.T) {
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				if staged {
					return nil, errors.New("git error")
				}
				return nil, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		assert.Empty(t, flMsg.entries)
		assert.Len(t, flMsg.warnings, 1, "staged fetch error should be in warnings")
		assert.Contains(t, flMsg.warnings[0], "git error")
	})
}

func TestModel_HandleFileLoadedUntrackedFallback(t *testing.T) {
	t.Run("untracked file with empty diff falls back to disk read", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileUntracked}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for untracked
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		// load files then select the untracked file
		cmd := m.Init()
		msg := cmd()
		flMsg := msg.(filesLoadedMsg)
		_ = flMsg
		// handleFilesLoaded auto-selects first file and returns loadFileDiff cmd
		result, cmd := m.Update(msg)
		m = result.(Model)
		// handleFileLoaded — empty diff triggers fallback
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)
		// reaching here without panic means the fallback path handled the untracked file correctly.
		// if testdata/newfile.go doesn't exist on disk, diffLines stays nil — that's expected
		assert.Equal(t, "newfile.go", m.file.name)
	})

	t.Run("non-untracked file with empty diff does not trigger fallback", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for some reason
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)
		assert.Nil(t, m.file.lines, "non-untracked empty diff should not trigger disk fallback")
	})
}

func TestModel_HandleFileLoadedStagedOnlyFallback(t *testing.T) {
	t.Run("staged-only FileAdded with empty diff retries with --cached", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		cachedLines := []diff.DiffLine{{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				if staged {
					return cachedLines, nil
				}
				return nil, nil // empty unstaged diff for staged-only file
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		// load files then handle auto-selected first file
		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Equal(t, cachedLines, m.file.lines, "staged-only file should show --cached diff content")
	})

	t.Run("staged mode does not retry --cached", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				return nil, nil // empty diff even with --cached
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true
		m.cfg.staged = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Nil(t, m.file.lines, "staged mode should not retry with --cached")
	})

	t.Run("FileModified with empty diff does not trigger staged retry", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
				if staged {
					return []diff.DiffLine{{NewNum: 1, Content: "staged", ChangeType: diff.ChangeContext}}, nil
				}
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Nil(t, m.file.lines, "FileModified should not trigger staged retry even with empty diff")
	})

	t.Run("staged retry propagates current compact context", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		cachedLines := []diff.DiffLine{{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext}}
		var stagedCtx int
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool, ctx int) ([]diff.DiffLine, error) {
				if staged {
					stagedCtx = ctx
					return cachedLines, nil
				}
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.layout.width = 120
		m.layout.height = 40
		m.ready = true
		m.compact.applicable = true
		m.modes.compact = true
		m.modes.compactContext = 5

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Equal(t, 5, stagedCtx, "staged retry must pass current compact context, not 0")
		assert.Equal(t, cachedLines, m.file.lines)
	})
}

func TestModel_FilesLoadedSingleFile(t *testing.T) {
	m := testModel(nil, nil)
	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "main.go"}}})
	model := result.(Model)

	assert.True(t, model.file.singleFile, "singleFile should be true for one file")
	assert.Equal(t, paneDiff, model.layout.focus, "focus should be on diff pane in single-file mode")
	assert.NotNil(t, cmd) // should auto-select first file
}

func TestModel_FilesLoadedSingleFileViewportWidth(t *testing.T) {
	m := testModel(nil, nil)
	// simulate initial resize (viewport created with multi-file width)
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	assert.True(t, m.ready, "model should be ready after resize")

	// now load single file — viewport width should be recalculated
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "main.go"}}})
	model := result.(Model)
	assert.True(t, model.file.singleFile)
	assert.Equal(t, 0, model.layout.treeWidth, "treeWidth should be 0 in single-file mode")
	assert.Equal(t, 98, model.layout.viewport.Width, "viewport width should be width - 2 (borders only)")
}

func TestModel_FileLoadedMarkdownTOCDetection(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "some text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Section", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "more text", ChangeType: diff.ChangeContext},
	}

	t.Run("markdown full-context triggers TOC", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.layout.treeWidth = 0
		m.layout.focus = paneDiff

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)

		require.NotNil(t, model.file.mdTOC, "mdTOC should be set for markdown full-context")
		assert.Equal(t, 3, model.file.mdTOC.NumEntries()) // top + 2 headers
		assert.Positive(t, model.layout.treeWidth, "treeWidth should be set when TOC is active")
	})

	t.Run("non-markdown file does not trigger TOC", func(t *testing.T) {
		goLines := []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		}
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": goLines})
		m.file.singleFile = true

		result, _ := m.Update(fileLoadedMsg{file: "main.go", lines: goLines})
		model := result.(Model)

		assert.Nil(t, model.file.mdTOC, "mdTOC should be nil for non-markdown file")
	})

	t.Run("markdown with diff changes does not trigger TOC", func(t *testing.T) {
		mixedLines := []diff.DiffLine{
			{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "added line", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mixedLines})
		m.file.singleFile = true

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mixedLines})
		model := result.(Model)

		assert.Nil(t, model.file.mdTOC, "mdTOC should be nil when file has diff changes")
	})

	t.Run("markdown with no headers produces nil TOC", func(t *testing.T) {
		noHeaders := []diff.DiffLine{
			{NewNum: 1, Content: "just text", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "more text", ChangeType: diff.ChangeContext},
		}
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": noHeaders})
		m.file.singleFile = true
		m.layout.treeWidth = 0 // single-file mode starts with treeWidth=0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: noHeaders})
		model := result.(Model)

		assert.Nil(t, model.file.mdTOC, "mdTOC should be nil when no headers found")
		assert.Equal(t, 0, model.layout.treeWidth, "treeWidth should stay 0 when no TOC")
	})

	t.Run("multi-file mode does not trigger TOC", func(t *testing.T) {
		m := testModel([]string{"README.md", "main.go"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = false

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)

		assert.Nil(t, model.file.mdTOC, "mdTOC should be nil in multi-file mode")
	})

	t.Run("switching to non-markdown clears existing TOC", func(t *testing.T) {
		goLines := []diff.DiffLine{{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext}}
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": goLines})
		m.file.singleFile = true
		// pre-existing TOC from previous markdown file
		staleMdLines := []diff.DiffLine{{NewNum: 1, Content: "# Stale", ChangeType: diff.ChangeContext}}
		m.file.mdTOC = sidepane.ParseTOC(staleMdLines, "old.md")

		result, _ := m.Update(fileLoadedMsg{file: "main.go", lines: goLines})
		model := result.(Model)

		assert.Nil(t, model.file.mdTOC, "mdTOC should be cleared when switching to non-markdown file")
	})
}

func TestModel_FileLoadedTOCViewportWidth(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "## Section", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})

	// simulate initial resize then single-file load
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	loaded, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "README.md"}}})
	m = loaded.(Model)
	require.True(t, m.file.singleFile)
	require.Equal(t, 0, m.layout.treeWidth, "treeWidth starts at 0 in single-file mode")

	// loading the markdown file should set up TOC and adjust widths
	result, _ := m.Update(fileLoadedMsg{file: "README.md", seq: m.file.loadSeq, lines: mdLines})
	model := result.(Model)

	require.NotNil(t, model.file.mdTOC)
	assert.Positive(t, model.layout.treeWidth, "treeWidth should be set for TOC pane")
	expectedTreeWidth := max(minTreeWidth, 100*model.cfg.treeWidthRatio/10)
	assert.Equal(t, expectedTreeWidth, model.layout.treeWidth)
	assert.Equal(t, 100-expectedTreeWidth-4, model.layout.viewport.Width, "viewport width adjusted for TOC")
}

func TestModel_HandleBlameLoadedSyncsViewportForWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{
		{NewNum: 1, Content: strings.Repeat("a", 60), ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "tail", ChangeType: diff.ChangeContext},
	}
	m.modes.wrap = true
	m.modes.showBlame = true
	m.layout.focus = paneDiff
	m.layout.treeHidden = true
	m.layout.width = 40
	m.layout.viewport = viewport.New(37, 2)
	m.nav.diffCursor = 1

	m.syncViewportToCursor()
	before := m.layout.viewport.YOffset

	result, _ := m.handleBlameLoaded(blameLoadedMsg{
		file: "a.go",
		seq:  m.file.loadSeq,
		data: map[int]diff.BlameLine{
			1: {Author: "LongAuthor", Time: time.Now()},
			2: {Author: "LongAuthor", Time: time.Now()},
		},
	})
	model := result.(Model)

	assert.Greater(t, model.layout.viewport.YOffset, before, "viewport should be re-synced after blame narrows wrap width")
	cursorY := model.cursorViewportY()
	assert.GreaterOrEqual(t, cursorY, model.layout.viewport.YOffset)
	assert.Less(t, cursorY, model.layout.viewport.YOffset+model.layout.viewport.Height)
}

func TestModel_FileLoadedResetsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.nav.diffCursor = 5 // simulate cursor was elsewhere

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor) // cursor reset to first line
}

func TestModel_FileLoadedAcceptedAfterCursorMove(t *testing.T) {
	// simulate: user presses n to load b.go (seq=1), then j/k moves cursor to c.go before response arrives.
	// the response for b.go should still be accepted because it carries the latest sequence number.
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)

	// user presses n to load b.go
	m.file.loadSeq = 1
	m.tree.StepFile(sidepane.DirectionNext) // cursor -> b.go

	// then j/k moves cursor to c.go (without triggering a load)
	m.tree.Move(sidepane.MotionDown) // cursor -> c.go
	assert.Equal(t, "c.go", m.tree.SelectedFile(), "cursor moved to c.go")

	// b.go response arrives with matching seq - should be accepted
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model := result.(Model)
	assert.Equal(t, "b.go", model.file.name, "response should be accepted despite cursor being on c.go")
	assert.Equal(t, bLines, model.file.lines)
}

func TestModel_TriggerReload_BumpsFileLoadSeq(t *testing.T) {
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{})
	oldSeq := m.file.loadSeq

	m.triggerReload()

	assert.Equal(t, oldSeq+1, m.file.loadSeq, "triggerReload must bump file.loadSeq to invalidate in-flight fileLoadedMsg")
}

func TestModel_TriggerReload_DropsStaleFileLoadedMsg(t *testing.T) {
	// regression: a fileLoadedMsg dispatched before R is pressed must be
	// dropped by handleFileLoaded because triggerReload bumps file.loadSeq.
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{})
	sentinelLine := diff.DiffLine{Content: "sentinel", ChangeType: diff.ChangeContext, NewNum: 1}
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{sentinelLine}

	// capture seq before reload; this is the seq the in-flight load was dispatched with
	oldSeq := m.file.loadSeq

	m.triggerReload() // bumps file.loadSeq — the old seq is now stale

	// deliver the stale fileLoadedMsg (seq matches oldSeq, not the bumped value)
	staleLine := diff.DiffLine{Content: "stale", ChangeType: diff.ChangeAdd, NewNum: 1}
	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: oldSeq, lines: []diff.DiffLine{staleLine}})
	model := result.(Model)

	// stale message must be dropped — sentinel lines must be unchanged
	assert.Equal(t, []diff.DiffLine{sentinelLine}, model.file.lines,
		"stale fileLoadedMsg must be dropped after triggerReload bumps file.loadSeq")
}

func TestModel_LoadCommits_ReturnsNilWhenNotApplicable(t *testing.T) {
	m := testModel(nil, nil)
	m.commits.source = &fakeCommitLog{}
	m.commits.applicable = false

	cmd := m.loadCommits()
	assert.Nil(t, cmd, "loadCommits must return nil when not applicable")
}

func TestModel_LoadCommits_ReturnsNilWhenSourceIsNil(t *testing.T) {
	m := testModel(nil, nil)
	m.commits.source = nil
	m.commits.applicable = true

	cmd := m.loadCommits()
	assert.Nil(t, cmd, "loadCommits must return nil when source is nil")
}

func TestModel_LoadCommits_ReturnsCmdWhenApplicable(t *testing.T) {
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
		return []diff.CommitInfo{{Hash: "abc"}, {Hash: "def"}}, nil
	}}
	m := testModel(nil, nil)
	m.commits.source = fake
	m.commits.applicable = true
	m.cfg.ref = "HEAD~2"
	m.commits.loadSeq = 7

	cmd := m.loadCommits()
	require.NotNil(t, cmd, "loadCommits must return a command when applicable and source is set")

	msg := cmd()
	cmsg, ok := msg.(commitsLoadedMsg)
	require.True(t, ok, "command must emit a commitsLoadedMsg")
	assert.Equal(t, uint64(7), cmsg.seq, "captured seq must be on the message")
	assert.Len(t, cmsg.list, 2)
	assert.Equal(t, "abc", cmsg.list[0].Hash)
	assert.False(t, cmsg.truncated, "under MaxCommits must not be truncated")
	require.NoError(t, cmsg.err)
	assert.Equal(t, "HEAD~2", fake.lastRef, "CommitLog must be called with the captured ref")
}

func TestModel_LoadCommits_PropagatesError(t *testing.T) {
	boom := errors.New("vcs blew up")
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
		return nil, boom
	}}
	m := testModel(nil, nil)
	m.commits.source = fake
	m.commits.applicable = true
	m.cfg.ref = "bad"

	cmd := m.loadCommits()
	require.NotNil(t, cmd)
	msg := cmd()
	cmsg, ok := msg.(commitsLoadedMsg)
	require.True(t, ok)
	require.Error(t, cmsg.err)
	assert.Equal(t, boom, cmsg.err)
	assert.Empty(t, cmsg.list)
	assert.False(t, cmsg.truncated)
}

func TestModel_LoadCommits_TruncatedFlag(t *testing.T) {
	full := make([]diff.CommitInfo, diff.MaxCommits)
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) { return full, nil }}
	m := testModel(nil, nil)
	m.commits.source = fake
	m.commits.applicable = true

	cmd := m.loadCommits()
	require.NotNil(t, cmd)
	cmsg := cmd().(commitsLoadedMsg)
	assert.True(t, cmsg.truncated, "exactly MaxCommits results must mark truncated")
}

func TestModel_HandleCommitsLoaded_PopulatesState(t *testing.T) {
	m := testModel(nil, nil)
	m.commits.loadSeq = 3
	m.commits.loaded = false
	m.commits.list = nil
	m.commits.err = nil
	m.commits.truncated = false

	list := []diff.CommitInfo{{Hash: "abc"}, {Hash: "def"}}
	result, cmd := m.Update(commitsLoadedMsg{seq: 3, list: list, truncated: true})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.True(t, model.commits.loaded, "loaded must flip to true after matching seq")
	assert.Equal(t, list, model.commits.list)
	assert.True(t, model.commits.truncated)
	require.NoError(t, model.commits.err)
}

func TestModel_HandleCommitsLoaded_DropsStaleResult(t *testing.T) {
	// regression: a slow commit fetch (seq=0) must not overwrite state after a
	// newer load (seq=1) was issued — e.g. user pressed R immediately after startup.
	m := testModel(nil, nil)
	m.commits.loadSeq = 1 // simulate a newer load already dispatched (e.g. triggerReload)
	m.commits.loaded = false
	m.commits.list = nil

	stale := []diff.CommitInfo{{Hash: "stale"}}
	result, cmd := m.Update(commitsLoadedMsg{seq: 0, list: stale})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.False(t, model.commits.loaded, "stale result must not flip loaded")
	assert.Nil(t, model.commits.list, "stale result must not populate list")
}

func TestModel_HandleCommitsLoaded_SetsLoadedOnError(t *testing.T) {
	m := testModel(nil, nil)
	m.commits.loadSeq = 5
	m.commits.loaded = false

	boom := errors.New("vcs blew up")
	result, cmd := m.Update(commitsLoadedMsg{seq: 5, err: boom})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.True(t, model.commits.loaded, "error result must still mark loaded=true to cache the failure")
	assert.Equal(t, boom, model.commits.err)
	assert.Empty(t, model.commits.list)
}

func TestModel_TriggerReload_RefetchesCommits(t *testing.T) {
	fake := &fakeCommitLog{fn: func(string) ([]diff.CommitInfo, error) {
		return []diff.CommitInfo{{Hash: "fresh"}}, nil
	}}
	m := testNewModel(t, plainRenderer(), annotation.NewStore(), noopHighlighter(), ModelConfig{})
	m.commits.source = fake
	m.commits.applicable = true
	m.commits.loaded = true
	m.commits.list = []diff.CommitInfo{{Hash: "stale"}}
	oldSeq := m.commits.loadSeq

	cmd := m.triggerReload()

	assert.False(t, m.commits.loaded, "triggerReload must invalidate commit cache")
	assert.Nil(t, m.commits.list, "triggerReload must clear commit list")
	assert.Equal(t, oldSeq+1, m.commits.loadSeq, "triggerReload must bump commits.loadSeq")
	require.NotNil(t, cmd, "triggerReload must return a non-nil batch cmd")

	// execute the batch and find the commitsLoadedMsg to verify the refetch actually runs
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "triggerReload must return a tea.BatchMsg when both loaders are active")
	var gotCommits *commitsLoadedMsg
	for _, inner := range batch {
		if msg, ok := inner().(commitsLoadedMsg); ok {
			gotCommits = &msg
			break
		}
	}
	require.NotNil(t, gotCommits, "batch must include a commitsLoadedMsg")
	assert.Equal(t, 1, fake.calls, "CommitLog must be called once by the refetch")
	assert.Equal(t, []diff.CommitInfo{{Hash: "fresh"}}, gotCommits.list, "refetched commits must be fresh, not stale")
	assert.Equal(t, oldSeq+1, gotCommits.seq, "refetched commits must carry the bumped seq")
}

func TestModel_TriggerReload_BumpsSeqAndCallsLoadFiles(t *testing.T) {
	callCount := 0
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
			callCount++
			return []diff.FileEntry{{Path: "main.go"}}, nil
		},
		FileDiffFunc: func(ref, file string, staged bool, _ int) ([]diff.DiffLine, error) {
			return nil, nil
		},
	}
	m := testNewModel(t, renderer, annotation.NewStore(), noopHighlighter(), ModelConfig{})
	initialSeq := m.filesLoadSeq

	cmd := m.triggerReload()
	assert.Equal(t, initialSeq+1, m.filesLoadSeq, "triggerReload must bump filesLoadSeq")
	assert.NotNil(t, cmd, "triggerReload must return a loadFiles command")

	// execute the command to confirm it calls ChangedFiles
	msg := cmd()
	_, ok := msg.(filesLoadedMsg)
	assert.True(t, ok, "triggerReload command must emit filesLoadedMsg")
	assert.Equal(t, 1, callCount, "triggerReload must trigger ChangedFiles")
}

func TestModel_RecomputeIntraRanges(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "context before", ChangeType: diff.ChangeContext},
		{Content: "return foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "return foo(baz)", ChangeType: diff.ChangeAdd},
		{Content: "context after", ChangeType: diff.ChangeContext},
	}

	m.recomputeIntraRanges()

	require.Len(t, m.file.intraRanges, 4)
	assert.Nil(t, m.file.intraRanges[0], "context line should have no ranges")
	assert.NotNil(t, m.file.intraRanges[1], "remove line should have ranges")
	assert.NotNil(t, m.file.intraRanges[2], "add line should have ranges")
	assert.Nil(t, m.file.intraRanges[3], "context line should have no ranges")

	// verify the ranges point to "bar" and "baz"
	require.Len(t, m.file.intraRanges[1], 1)
	assert.Equal(t, worddiff.Range{Start: 11, End: 14}, m.file.intraRanges[1][0])
	require.Len(t, m.file.intraRanges[2], 1)
	assert.Equal(t, worddiff.Range{Start: 11, End: 14}, m.file.intraRanges[2][0])
}

func TestModel_RecomputeIntraRanges_IdenticalPair(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "same line content", ChangeType: diff.ChangeRemove},
		{Content: "same line content", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// identical lines produce no changed ranges, so intra-line ranges remain nil
	assert.Nil(t, m.file.intraRanges[0], "identical remove should have no ranges")
	assert.Nil(t, m.file.intraRanges[1], "identical add should have no ranges")
}

func TestModel_RecomputeIntraRanges_PureAddBlock(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "context", ChangeType: diff.ChangeContext},
		{Content: "new line 1", ChangeType: diff.ChangeAdd},
		{Content: "new line 2", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// pure add block has no pairs, so no intra-line ranges
	for i, r := range m.file.intraRanges {
		assert.Nil(t, r, "line %d should have no ranges", i)
	}
}

func TestModel_RecomputeIntraRanges_DissimilarPair(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "alpha bravo charlie delta echo foxtrot golf hotel india juliet", ChangeType: diff.ChangeRemove},
		{Content: "xxx yyy zzz aaa bbb ccc ddd eee fff ggg", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// dissimilar pair should have no ranges due to similarity gate
	assert.Nil(t, m.file.intraRanges[0])
	assert.Nil(t, m.file.intraRanges[1])
}

func TestModel_RecomputeIntraRanges_TabContent(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "\treturn foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "\treturn foo(baz)", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// ranges should be on tab-replaced content
	require.NotNil(t, m.file.intraRanges[0])
	require.NotNil(t, m.file.intraRanges[1])

	// after tab replacement, "\t" becomes "    " (4 spaces), so "bar" starts at 4+11=15
	tabReplaced := strings.ReplaceAll(m.file.lines[0].Content, "\t", m.cfg.tabSpaces)
	require.Len(t, m.file.intraRanges[0], 1)
	changed := tabReplaced[m.file.intraRanges[0][0].Start:m.file.intraRanges[0][0].End]
	assert.Equal(t, "bar", changed)
}

func TestModel_RecomputeIntraRanges_MultipleBlocks(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.wordDiff = true
	m.cfg.tabSpaces = "    "
	m.file.lines = []diff.DiffLine{
		{Content: "old first", ChangeType: diff.ChangeRemove},
		{Content: "new first", ChangeType: diff.ChangeAdd},
		{Content: "context between", ChangeType: diff.ChangeContext},
		{Content: "old second", ChangeType: diff.ChangeRemove},
		{Content: "new second", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// both blocks should have ranges
	assert.NotNil(t, m.file.intraRanges[0], "first block remove")
	assert.NotNil(t, m.file.intraRanges[1], "first block add")
	assert.Nil(t, m.file.intraRanges[2], "context line")
	assert.NotNil(t, m.file.intraRanges[3], "second block remove")
	assert.NotNil(t, m.file.intraRanges[4], "second block add")
}

func TestHandleFileLoaded_SingleColLineNum_FullContext(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "// comment", ChangeType: diff.ChangeContext},
		{OldNum: 3, NewNum: 3, Content: "func main() {}", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.modes.lineNumbers = true

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.True(t, model.file.singleColLineNum, "full-context file should set singleColLineNum to true")
	assert.Equal(t, model.file.lineNumWidth+1, model.lineNumGutterWidth(), "full-context should use single-column gutter width")
}

func TestHandleFileLoaded_SingleColLineNum_RealDiff(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new line", ChangeType: diff.ChangeAdd},
		{OldNum: 3, NewNum: 3, Content: "// end", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.modes.lineNumbers = true

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.False(t, model.file.singleColLineNum, "file with add/remove lines should set singleColLineNum to false")
}

func TestModel_CurrentContextLines(t *testing.T) {
	tests := []struct {
		name       string
		compact    bool
		ctx        int
		applicable bool
		want       int
	}{
		{name: "compact off, applicable", compact: false, ctx: 5, applicable: true, want: 0},
		{name: "compact off, not applicable", compact: false, ctx: 5, applicable: false, want: 0},
		{name: "compact on, applicable", compact: true, ctx: 5, applicable: true, want: 5},
		{name: "compact on, not applicable", compact: true, ctx: 5, applicable: false, want: 0},
		{name: "compact on, custom ctx", compact: true, ctx: 10, applicable: true, want: 10},
		{name: "compact on, zero ctx", compact: true, ctx: 0, applicable: true, want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := testModel([]string{"a.go"}, nil)
			m.modes.compact = tc.compact
			m.modes.compactContext = tc.ctx
			m.compact.applicable = tc.applicable
			assert.Equal(t, tc.want, m.currentContextLines())
		})
	}
}

func TestModel_LoadFileDiffPassesContextLines(t *testing.T) {
	var captured int
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error) {
			captured = contextLines
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.modes.compact = true
	m.modes.compactContext = 7
	m.compact.applicable = true

	cmd := m.loadFileDiff("a.go")
	require.NotNil(t, cmd)
	cmd() // executes and records contextLines
	assert.Equal(t, 7, captured, "compact mode should pass compactContext to FileDiff")
}

func TestModel_LoadFileDiffPassesZeroWhenNotApplicable(t *testing.T) {
	var captured = -1
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error) {
			captured = contextLines
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.modes.compact = true
	m.modes.compactContext = 5
	m.compact.applicable = false

	cmd := m.loadFileDiff("a.go")
	require.NotNil(t, cmd)
	cmd()
	assert.Equal(t, 0, captured, "non-applicable compact mode must still pass 0 (full file)")
}

func TestModel_ReloadCurrentFileBumpsLoadSeqAndFetches(t *testing.T) {
	var calls int
	renderer := &mocks.RendererMock{
		ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) { return nil, nil },
		FileDiffFunc: func(ref, file string, staged bool, contextLines int) ([]diff.DiffLine, error) {
			calls++
			return nil, nil
		},
	}
	m := testModel([]string{"a.go"}, nil)
	m.diffRenderer = renderer
	m.file.name = "a.go"
	beforeSeq := m.file.loadSeq

	cmd := m.reloadCurrentFile()
	require.NotNil(t, cmd)
	assert.Greater(t, m.file.loadSeq, beforeSeq, "reloadCurrentFile must bump file.loadSeq to invalidate prior in-flight loads")
	cmd()
	assert.Equal(t, 1, calls, "reloadCurrentFile command must invoke FileDiff once")
}

func TestModel_ReloadCurrentFileNoOpWhenEmpty(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.file.name = ""
	beforeSeq := m.file.loadSeq

	cmd := m.reloadCurrentFile()
	assert.Nil(t, cmd, "reloadCurrentFile must be a no-op when no file is loaded")
	assert.Equal(t, beforeSeq, m.file.loadSeq, "no load implies no seq bump")
}
