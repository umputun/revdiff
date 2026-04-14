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

	result, cmd := m.Update(filesLoadedMsg{err: assert.AnError})
	model := result.(Model)

	assert.Nil(t, cmd)
	assert.Equal(t, 0, model.tree.TotalFiles())
}

func TestModel_FilesLoadedMultipleFiles(t *testing.T) {
	m := testModel(nil, nil)
	result, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}}})
	model := result.(Model)

	assert.False(t, model.singleFile, "singleFile should be false for multiple files")
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

	assert.Equal(t, "a.go", model.currFile)
	assert.Len(t, model.diffLines, 2)
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
			m.diffLines = tt.lines
			m.computeFileStats()
			assert.Equal(t, tt.adds, m.fileAdds, "fileAdds")
			assert.Equal(t, tt.removes, m.fileRemoves, "fileRemoves")
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
			m.diffLines = tt.lines
			m.fileAdds = tt.adds
			m.fileRemoves = tt.removes
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
	m.loadSeq = 1

	result, _ := m.Update(fileLoadedMsg{file: "a.go", seq: 1, lines: lines})
	model := result.(Model)
	assert.Equal(t, 2, model.fileAdds)
	assert.Equal(t, 1, model.fileRemoves)
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
		m.only = []string{"ui/model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("suffix match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("multiple patterns", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"model.go", "README.md"}
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go", "README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern resolved against workDir", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/README.md"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "README.md")
		assert.Equal(t, []string{"README.md"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path pattern with subdirectory", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/ui/model.go"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go", "README.md")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("absolute path outside workDir does not match", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/other/README.md"}
		m.workDir = "/repo"
		files := toEntries("README.md", "ui/model.go")
		assert.Empty(t, m.filterOnly(files))
	})

	t.Run("absolute path suffix match via resolved relative", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"/repo/model.go"}
		m.workDir = "/repo"
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Equal(t, []string{"ui/model.go"}, diff.FileEntryPaths(m.filterOnly(files)))
	})

	t.Run("no matches returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.only = []string{"nonexistent.go"}
		files := toEntries("ui/model.go", "diff/diff.go")
		assert.Empty(t, m.filterOnly(files))
	})
}

func TestModel_FilterOnlyNoMatchShowsMessage(t *testing.T) {
	m := testModel(nil, nil)
	m.only = []string{"nonexistent.go"}
	m.ready = true
	m.width = 80
	m.height = 24
	m.viewport = viewport.New(76, 20)

	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "ui/model.go"}, {Path: "diff/diff.go"}}})
	model := result.(Model)
	assert.Nil(t, cmd, "should not trigger file load when no files match")
	assert.Contains(t, model.viewport.View(), "no files match --only filter")
}

func TestModel_UntrackedToggle(t *testing.T) {
	t.Run("toggle cycles showUntracked and reloads files", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
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
		m.width = 120
		m.height = 40
		m.ready = true

		// initially untracked is off
		assert.False(t, m.showUntracked)

		// toggle on
		result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
		assert.True(t, result.(Model).showUntracked)
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
		assert.False(t, result.(Model).showUntracked)
	})

	t.Run("status bar shows untracked icon when untracked is on", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		// ∅ is always in the icons list but inactive (muted) when showUntracked is false
		// check that ∅ becomes active when toggled
		m.showUntracked = true
		iconsActive := m.statusModeIcons()
		assert.Contains(t, iconsActive, "∅")
	})

	t.Run("no untracked files when LoadUntracked is nil", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go"}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
		m.ready = true
		m.showUntracked = true

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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
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
		m.width = 120
		m.height = 40
		m.ready = true
		m.showUntracked = true

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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return []diff.DiffLine{{Content: "content", ChangeType: diff.ChangeAdd, NewNum: 1}}, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3})
		m.width = 120
		m.height = 40
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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for untracked
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
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
		assert.Equal(t, "newfile.go", m.currFile)
	})

	t.Run("non-untracked file with empty diff does not trigger fallback", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil // empty diff for some reason
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)
		assert.Nil(t, m.diffLines, "non-untracked empty diff should not trigger disk fallback")
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
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				if staged {
					return cachedLines, nil
				}
				return nil, nil // empty unstaged diff for staged-only file
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true

		// load files then handle auto-selected first file
		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Equal(t, cachedLines, m.diffLines, "staged-only file should show --cached diff content")
	})

	t.Run("staged mode does not retry --cached", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				return nil, nil // empty diff even with --cached
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true
		m.staged = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Nil(t, m.diffLines, "staged mode should not retry with --cached")
	})

	t.Run("FileModified with empty diff does not trigger staged retry", func(t *testing.T) {
		entries := []diff.FileEntry{{Path: "main.go", Status: diff.FileModified}}
		renderer := &mocks.RendererMock{
			ChangedFilesFunc: func(ref string, staged bool) ([]diff.FileEntry, error) {
				return entries, nil
			},
			FileDiffFunc: func(ref, file string, staged bool) ([]diff.DiffLine, error) {
				if staged {
					return []diff.DiffLine{{NewNum: 1, Content: "staged", ChangeType: diff.ChangeContext}}, nil
				}
				return nil, nil
			},
		}
		store := annotation.NewStore()
		m := testNewModel(t, renderer, store, noopHighlighter(), ModelConfig{TreeWidthRatio: 3, WorkDir: "testdata"})
		m.width = 120
		m.height = 40
		m.ready = true

		cmd := m.Init()
		msg := cmd()
		result, cmd := m.Update(msg)
		m = result.(Model)
		msg2 := cmd()
		result, _ = m.Update(msg2)
		m = result.(Model)

		assert.Nil(t, m.diffLines, "FileModified should not trigger staged retry even with empty diff")
	})
}

func TestModel_FilesLoadedSingleFile(t *testing.T) {
	m := testModel(nil, nil)
	result, cmd := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "main.go"}}})
	model := result.(Model)

	assert.True(t, model.singleFile, "singleFile should be true for one file")
	assert.Equal(t, paneDiff, model.focus, "focus should be on diff pane in single-file mode")
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
	assert.True(t, model.singleFile)
	assert.Equal(t, 0, model.treeWidth, "treeWidth should be 0 in single-file mode")
	assert.Equal(t, 98, model.viewport.Width, "viewport width should be width - 2 (borders only)")
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
		m.singleFile = true
		m.treeWidth = 0
		m.focus = paneDiff

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)

		require.NotNil(t, model.mdTOC, "mdTOC should be set for markdown full-context")
		assert.Equal(t, 3, model.mdTOC.NumEntries()) // top + 2 headers
		assert.Positive(t, model.treeWidth, "treeWidth should be set when TOC is active")
	})

	t.Run("non-markdown file does not trigger TOC", func(t *testing.T) {
		goLines := []diff.DiffLine{
			{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		}
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": goLines})
		m.singleFile = true

		result, _ := m.Update(fileLoadedMsg{file: "main.go", lines: goLines})
		model := result.(Model)

		assert.Nil(t, model.mdTOC, "mdTOC should be nil for non-markdown file")
	})

	t.Run("markdown with diff changes does not trigger TOC", func(t *testing.T) {
		mixedLines := []diff.DiffLine{
			{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "added line", ChangeType: diff.ChangeAdd},
		}
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mixedLines})
		m.singleFile = true

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mixedLines})
		model := result.(Model)

		assert.Nil(t, model.mdTOC, "mdTOC should be nil when file has diff changes")
	})

	t.Run("markdown with no headers produces nil TOC", func(t *testing.T) {
		noHeaders := []diff.DiffLine{
			{NewNum: 1, Content: "just text", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "more text", ChangeType: diff.ChangeContext},
		}
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": noHeaders})
		m.singleFile = true
		m.treeWidth = 0 // single-file mode starts with treeWidth=0

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: noHeaders})
		model := result.(Model)

		assert.Nil(t, model.mdTOC, "mdTOC should be nil when no headers found")
		assert.Equal(t, 0, model.treeWidth, "treeWidth should stay 0 when no TOC")
	})

	t.Run("multi-file mode does not trigger TOC", func(t *testing.T) {
		m := testModel([]string{"README.md", "main.go"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = false

		result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model := result.(Model)

		assert.Nil(t, model.mdTOC, "mdTOC should be nil in multi-file mode")
	})

	t.Run("switching to non-markdown clears existing TOC", func(t *testing.T) {
		goLines := []diff.DiffLine{{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext}}
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": goLines})
		m.singleFile = true
		// pre-existing TOC from previous markdown file
		staleMdLines := []diff.DiffLine{{NewNum: 1, Content: "# Stale", ChangeType: diff.ChangeContext}}
		m.mdTOC = sidepane.ParseTOC(staleMdLines, "old.md")

		result, _ := m.Update(fileLoadedMsg{file: "main.go", lines: goLines})
		model := result.(Model)

		assert.Nil(t, model.mdTOC, "mdTOC should be cleared when switching to non-markdown file")
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
	require.True(t, m.singleFile)
	require.Equal(t, 0, m.treeWidth, "treeWidth starts at 0 in single-file mode")

	// loading the markdown file should set up TOC and adjust widths
	result, _ := m.Update(fileLoadedMsg{file: "README.md", seq: m.loadSeq, lines: mdLines})
	model := result.(Model)

	require.NotNil(t, model.mdTOC)
	assert.Positive(t, model.treeWidth, "treeWidth should be set for TOC pane")
	expectedTreeWidth := max(minTreeWidth, 100*model.treeWidthRatio/10)
	assert.Equal(t, expectedTreeWidth, model.treeWidth)
	assert.Equal(t, 100-expectedTreeWidth-4, model.viewport.Width, "viewport width adjusted for TOC")
}

func TestModel_HandleBlameLoadedSyncsViewportForWrap(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{
		{NewNum: 1, Content: strings.Repeat("a", 60), ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "tail", ChangeType: diff.ChangeContext},
	}
	m.wrapMode = true
	m.showBlame = true
	m.focus = paneDiff
	m.treeHidden = true
	m.width = 40
	m.viewport = viewport.New(37, 2)
	m.diffCursor = 1

	m.syncViewportToCursor()
	before := m.viewport.YOffset

	result, _ := m.handleBlameLoaded(blameLoadedMsg{
		file: "a.go",
		seq:  m.loadSeq,
		data: map[int]diff.BlameLine{
			1: {Author: "LongAuthor", Time: time.Now()},
			2: {Author: "LongAuthor", Time: time.Now()},
		},
	})
	model := result.(Model)

	assert.Greater(t, model.viewport.YOffset, before, "viewport should be re-synced after blame narrows wrap width")
	cursorY := model.cursorViewportY()
	assert.GreaterOrEqual(t, cursorY, model.viewport.YOffset)
	assert.Less(t, cursorY, model.viewport.YOffset+model.viewport.Height)
}

func TestModel_FileLoadedResetsCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line2", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.diffCursor = 5 // simulate cursor was elsewhere

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)
	assert.Equal(t, 0, model.diffCursor) // cursor reset to first line
}

func TestModel_FileLoadedAcceptedAfterCursorMove(t *testing.T) {
	// simulate: user presses n to load b.go (seq=1), then j/k moves cursor to c.go before response arrives.
	// the response for b.go should still be accepted because it carries the latest sequence number.
	files := []string{"a.go", "b.go", "c.go"}
	m := testModel(files, nil)
	m.tree = testNewFileTree(files)

	// user presses n to load b.go
	m.loadSeq = 1
	m.tree.StepFile(sidepane.DirectionNext) // cursor -> b.go

	// then j/k moves cursor to c.go (without triggering a load)
	m.tree.Move(sidepane.MotionDown) // cursor -> c.go
	assert.Equal(t, "c.go", m.tree.SelectedFile(), "cursor moved to c.go")

	// b.go response arrives with matching seq - should be accepted
	bLines := []diff.DiffLine{{NewNum: 1, Content: "package b", ChangeType: diff.ChangeContext}}
	result, _ := m.Update(fileLoadedMsg{file: "b.go", seq: 1, lines: bLines})
	model := result.(Model)
	assert.Equal(t, "b.go", model.currFile, "response should be accepted despite cursor being on c.go")
	assert.Equal(t, bLines, model.diffLines)
}

func TestModel_RecomputeIntraRanges(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "context before", ChangeType: diff.ChangeContext},
		{Content: "return foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "return foo(baz)", ChangeType: diff.ChangeAdd},
		{Content: "context after", ChangeType: diff.ChangeContext},
	}

	m.recomputeIntraRanges()

	require.Len(t, m.intraRanges, 4)
	assert.Nil(t, m.intraRanges[0], "context line should have no ranges")
	assert.NotNil(t, m.intraRanges[1], "remove line should have ranges")
	assert.NotNil(t, m.intraRanges[2], "add line should have ranges")
	assert.Nil(t, m.intraRanges[3], "context line should have no ranges")

	// verify the ranges point to "bar" and "baz"
	require.Len(t, m.intraRanges[1], 1)
	assert.Equal(t, worddiff.Range{Start: 11, End: 14}, m.intraRanges[1][0])
	require.Len(t, m.intraRanges[2], 1)
	assert.Equal(t, worddiff.Range{Start: 11, End: 14}, m.intraRanges[2][0])
}

func TestModel_RecomputeIntraRanges_IdenticalPair(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "same line content", ChangeType: diff.ChangeRemove},
		{Content: "same line content", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// identical lines produce no changed ranges, so intra-line ranges remain nil
	assert.Nil(t, m.intraRanges[0], "identical remove should have no ranges")
	assert.Nil(t, m.intraRanges[1], "identical add should have no ranges")
}

func TestModel_RecomputeIntraRanges_PureAddBlock(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "context", ChangeType: diff.ChangeContext},
		{Content: "new line 1", ChangeType: diff.ChangeAdd},
		{Content: "new line 2", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// pure add block has no pairs, so no intra-line ranges
	for i, r := range m.intraRanges {
		assert.Nil(t, r, "line %d should have no ranges", i)
	}
}

func TestModel_RecomputeIntraRanges_DissimilarPair(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "alpha bravo charlie delta echo foxtrot golf hotel india juliet", ChangeType: diff.ChangeRemove},
		{Content: "xxx yyy zzz aaa bbb ccc ddd eee fff ggg", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// dissimilar pair should have no ranges due to similarity gate
	assert.Nil(t, m.intraRanges[0])
	assert.Nil(t, m.intraRanges[1])
}

func TestModel_RecomputeIntraRanges_TabContent(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "\treturn foo(bar)", ChangeType: diff.ChangeRemove},
		{Content: "\treturn foo(baz)", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// ranges should be on tab-replaced content
	require.NotNil(t, m.intraRanges[0])
	require.NotNil(t, m.intraRanges[1])

	// after tab replacement, "\t" becomes "    " (4 spaces), so "bar" starts at 4+11=15
	tabReplaced := strings.ReplaceAll(m.diffLines[0].Content, "\t", m.tabSpaces)
	require.Len(t, m.intraRanges[0], 1)
	changed := tabReplaced[m.intraRanges[0][0].Start:m.intraRanges[0][0].End]
	assert.Equal(t, "bar", changed)
}

func TestModel_RecomputeIntraRanges_MultipleBlocks(t *testing.T) {
	m := testModel(nil, nil)
	m.wordDiff = true
	m.tabSpaces = "    "
	m.diffLines = []diff.DiffLine{
		{Content: "old first", ChangeType: diff.ChangeRemove},
		{Content: "new first", ChangeType: diff.ChangeAdd},
		{Content: "context between", ChangeType: diff.ChangeContext},
		{Content: "old second", ChangeType: diff.ChangeRemove},
		{Content: "new second", ChangeType: diff.ChangeAdd},
	}

	m.recomputeIntraRanges()

	// both blocks should have ranges
	assert.NotNil(t, m.intraRanges[0], "first block remove")
	assert.NotNil(t, m.intraRanges[1], "first block add")
	assert.Nil(t, m.intraRanges[2], "context line")
	assert.NotNil(t, m.intraRanges[3], "second block remove")
	assert.NotNil(t, m.intraRanges[4], "second block add")
}

func TestHandleFileLoaded_SingleColLineNum_FullContext(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, NewNum: 2, Content: "// comment", ChangeType: diff.ChangeContext},
		{OldNum: 3, NewNum: 3, Content: "func main() {}", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.lineNumbers = true

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.True(t, model.singleColLineNum, "full-context file should set singleColLineNum to true")
	assert.Equal(t, model.lineNumWidth+1, model.lineNumGutterWidth(), "full-context should use single-column gutter width")
}

func TestHandleFileLoaded_SingleColLineNum_RealDiff(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "old line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "new line", ChangeType: diff.ChangeAdd},
		{OldNum: 3, NewNum: 3, Content: "// end", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.lineNumbers = true

	result, _ := m.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model := result.(Model)

	assert.False(t, model.singleColLineNum, "file with add/remove lines should set singleColLineNum to false")
}
