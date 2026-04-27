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
	"github.com/umputun/revdiff/app/review"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
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

func TestModel_ReviewInfoStats(t *testing.T) {
	entries := []diff.FileEntry{{Path: "a.go", Status: diff.FileAdded}, {Path: "b.go", Status: diff.FileModified}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(_, file string, _ bool, _ int) ([]diff.DiffLine, error) {
			switch file {
			case "a.go":
				return []diff.DiffLine{{ChangeType: diff.ChangeAdd}, {ChangeType: diff.ChangeAdd}}, nil
			case "b.go":
				return []diff.DiffLine{{ChangeType: diff.ChangeRemove}, {ChangeType: diff.ChangeContext}}, nil
			default:
				return nil, nil
			}
		},
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true}})
	m.setReviewEntries(entries)
	cmd := m.loadReviewStats(entries)
	require.NotNil(t, cmd)
	msg := cmd().(reviewStatsLoadedMsg)

	result, _ := m.Update(msg)
	model := result.(Model)
	assert.True(t, model.review.statsLoaded)
	assert.Equal(t, 2, model.review.adds)
	assert.Equal(t, 1, model.review.removes)
	assert.False(t, model.review.partial)
	assert.Equal(t, "A1 M1", model.reviewStatusText())
	assert.Equal(t, "+2/-1", model.reviewLinesText())

	spec := model.buildInfoSpec()
	// mode and stats moved from body rows to popup borders post-redesign.
	assert.Equal(t, "working tree changes", spec.HeaderText, "header summarizes the review mode")
	assert.Contains(t, spec.FooterText, "+2/-1", "footer carries aggregate +/- stats")
	assert.Contains(t, spec.FooterText, "A1 M1", "footer carries status histogram")
	assert.Contains(t, spec.FooterText, "2 files", "footer carries file count")
}

func TestModel_ReviewInfoAllFilesSkipsLineStats(t *testing.T) {
	m := testModel(nil, nil)
	m.review.cfg = ReviewInfoConfig{Enabled: true, AllFiles: true}
	entries := []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}
	m.setReviewEntries(entries)

	assert.Nil(t, m.loadReviewStats(entries))
	// triggerReviewStats short-circuits and marks loaded so the overlay does not show "loading…"
	cmd := m.triggerReviewStats()
	assert.Nil(t, cmd)
	assert.True(t, m.review.statsLoaded)
	assert.True(t, m.review.statsRequested)
	assert.Equal(t, "not calculated in all-files mode", m.reviewLinesText())
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

func TestModel_ReviewStatsLazyOnFirstOpen(t *testing.T) {
	// regression: previously stats were computed eagerly on filesLoadedMsg.
	// they must now be deferred until the user opens the review-info overlay.
	entries := []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}
	calls := 0
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(_, _ string, _ bool, _ int) ([]diff.DiffLine, error) {
			calls++
			return []diff.DiffLine{{ChangeType: diff.ChangeAdd}}, nil
		},
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true}})

	// drive the files-loaded path directly so FileDiff calls reflect post-load state
	result, _ := m.Update(filesLoadedMsg{entries: entries})
	m = result.(Model)
	assert.False(t, m.review.statsRequested, "stats must not be requested before the overlay is opened")

	// reset the FileDiff call counter — only the lazy stats fetch should bump it
	calls = 0

	statsCmd := m.triggerReviewStats()
	require.NotNil(t, statsCmd, "first open must produce a stats fetch command")
	assert.True(t, m.review.statsRequested)

	// run the lazy stats fetch — now FileDiff is called for review aggregation
	statsMsg := statsCmd().(reviewStatsLoadedMsg)
	assert.Equal(t, len(entries), calls, "lazy stats fetch must call FileDiff once per entry")
	result, _ = m.Update(statsMsg)
	m = result.(Model)
	assert.True(t, m.review.statsLoaded)

	// second open does NOT re-fetch
	assert.Nil(t, m.triggerReviewStats(), "second open within the same load generation must not re-fetch")
}

func TestModel_ReviewStatsEarlyInfoOpenFetchesAfterFilesLoad(t *testing.T) {
	entries := []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(_, _ string, _ bool, _ int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{{ChangeType: diff.ChangeAdd}}, nil
		},
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true}})

	cmd := m.handleInfo()
	assert.Nil(t, cmd, "files are not loaded yet, so the early open should defer stats fetch")
	assert.True(t, m.review.statsRequested)
	assert.False(t, m.review.statsLoaded)

	result, cmd := m.Update(filesLoadedMsg{entries: entries})
	m = result.(Model)
	require.NotNil(t, cmd, "filesLoaded after an early info open must include the deferred stats fetch")

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "initial file load and deferred stats fetch should be batched")
	var gotStats *reviewStatsLoadedMsg
	for _, inner := range batch {
		if msg, ok := inner().(reviewStatsLoadedMsg); ok {
			gotStats = &msg
			break
		}
	}
	require.NotNil(t, gotStats, "batch must include a reviewStatsLoadedMsg")
	assert.Equal(t, 1, gotStats.Adds)
	assert.Equal(t, 0, gotStats.Removes)
}

func TestModel_ReviewStatsStaleSeqDropped(t *testing.T) {
	m := testModel(nil, nil)
	m.review.cfg = ReviewInfoConfig{Enabled: true}
	m.review.statsLoadSeq = 5

	// stale message (older seq) is ignored
	stale := reviewStatsLoadedMsg{seq: 4, Stats: review.Stats{Adds: 99, Removes: 99, Err: errors.New("stale")}}
	result, _ := m.Update(stale)
	model := result.(Model)
	assert.False(t, model.review.statsLoaded, "stale stats must not flip statsLoaded")
	assert.Equal(t, 0, model.review.adds, "stale stats must not populate counts")
	assert.Equal(t, 0, model.review.removes)
	require.NoError(t, model.review.err)

	// fresh message (matching seq) is accepted
	fresh := reviewStatsLoadedMsg{seq: 5, Stats: review.Stats{Adds: 7, Removes: 3}}
	result, _ = m.Update(fresh)
	model = result.(Model)
	assert.True(t, model.review.statsLoaded)
	assert.Equal(t, 7, model.review.adds)
	assert.Equal(t, 3, model.review.removes)
}

func TestModel_ReviewStatsErrorState(t *testing.T) {
	m := testModel(nil, nil)
	m.review.cfg = ReviewInfoConfig{Enabled: true}
	m.review.statsLoadSeq = 1

	msg := reviewStatsLoadedMsg{seq: 1, Stats: review.Stats{Err: errors.New("boom")}}
	result, _ := m.Update(msg)
	model := result.(Model)

	assert.True(t, model.review.statsLoaded)
	assert.Equal(t, "stats unavailable", model.reviewLinesText())
	assert.Contains(t, model.reviewRows(), overlay.InfoRow{Label: "stats", Value: "unavailable: boom"})
}

func TestModel_ReviewStatsAddedFallback(t *testing.T) {
	// FileAdded entries with empty unstaged diff must retry against the index
	// (staged=true). When that fallback succeeds, lines count toward stats.
	entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(_, _ string, staged bool, _ int) ([]diff.DiffLine, error) {
			if staged {
				return []diff.DiffLine{{ChangeType: diff.ChangeAdd}, {ChangeType: diff.ChangeAdd}}, nil
			}
			return nil, nil
		},
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true}})
	m.setReviewEntries(entries)

	cmd := m.loadReviewStats(entries)
	require.NotNil(t, cmd)
	stats := cmd().(reviewStatsLoadedMsg)
	assert.Equal(t, 2, stats.Adds, "added-file fallback to staged diff must contribute to stats")
	assert.False(t, stats.Partial)
}

func TestModel_ReviewStatsAddedFallbackErrorMarksPartial(t *testing.T) {
	// when the staged-fallback fetch itself errors, the file must be flagged
	// as partial rather than silently treated as zero-line.
	entries := []diff.FileEntry{{Path: "newfile.go", Status: diff.FileAdded}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(_, _ string, staged bool, _ int) ([]diff.DiffLine, error) {
			if staged {
				return nil, errors.New("staged unavailable")
			}
			return nil, nil
		},
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true}})
	m.setReviewEntries(entries)

	stats := m.loadReviewStats(entries)().(reviewStatsLoadedMsg)
	assert.True(t, stats.Partial, "staged-fallback error must mark stats as partial")
	require.NoError(t, stats.Err, "fallback failures must not surface as a fatal err")

	result, _ := m.Update(stats)
	model := result.(Model)
	assert.Contains(t, model.reviewLinesText(), "(partial)", "reviewLinesText must annotate partial state")
}

func TestModel_ReviewStatsUntrackedFallbackOutsideWorkDir(t *testing.T) {
	// untracked file paths must not escape workDir even if they contain "..".
	// the safeWorkDirPath guard rejects, partial flag is set, and the file
	// contributes zero lines instead of being read off-tree.
	entries := []diff.FileEntry{{Path: "../../etc/passwd", Status: diff.FileUntracked}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc:     func(string, string, bool, int) ([]diff.DiffLine, error) { return nil, nil },
	}
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{
		WorkDir:    t.TempDir(),
		ReviewInfo: ReviewInfoConfig{Enabled: true},
	})
	m.setReviewEntries(entries)

	stats := m.loadReviewStats(entries)().(reviewStatsLoadedMsg)
	assert.True(t, stats.Partial, "path-escape must mark partial")
	assert.Equal(t, 0, stats.Adds)
	assert.Equal(t, 0, stats.Removes)
}

func TestReviewLinesText_NoChangedLines(t *testing.T) {
	m := testModel(nil, nil)
	m.review.cfg = ReviewInfoConfig{Enabled: true}
	m.review.statsLoaded = true
	assert.Equal(t, "no changed lines", m.reviewLinesText())

	m.review.partial = true
	assert.Equal(t, "no changed lines (partial)", m.reviewLinesText())
}

func TestReviewHeaderText(t *testing.T) {
	tests := []struct {
		name string
		cfg  ReviewInfoConfig
		want string
	}{
		{name: "empty config returns empty string", cfg: ReviewInfoConfig{}, want: ""},
		{name: "stdin with name", cfg: ReviewInfoConfig{Enabled: true, Stdin: true, StdinName: "patch.diff"}, want: "stdin: patch.diff"},
		{name: "stdin without name", cfg: ReviewInfoConfig{Enabled: true, Stdin: true}, want: "stdin scratch buffer"},
		{name: "all-files", cfg: ReviewInfoConfig{Enabled: true, AllFiles: true}, want: "all tracked files"},
		{name: "staged", cfg: ReviewInfoConfig{Enabled: true, Staged: true}, want: "staged changes"},
		{name: "working tree", cfg: ReviewInfoConfig{Enabled: true}, want: "working tree changes"},
		{name: "ref range", cfg: ReviewInfoConfig{Enabled: true, Ref: "main..feature"}, want: "ref range: main..feature"},
		{name: "single ref", cfg: ReviewInfoConfig{Enabled: true, Ref: "HEAD~3"}, want: "changes against HEAD~3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.review.cfg = tt.cfg
			assert.Equal(t, tt.want, m.reviewHeaderText())
		})
	}
}

func TestReviewFooterText(t *testing.T) {
	t.Run("includes files lines status vcs", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{Enabled: true, VCS: "git"}
		m.review.entries = make([]diff.FileEntry, 22)
		m.review.statusCounts = map[diff.FileStatus]int{diff.FileAdded: 2, diff.FileModified: 20}
		m.review.adds = 231
		m.review.removes = 18
		m.review.statsLoaded = true

		got := m.reviewFooterText()
		assert.Contains(t, got, "22 files")
		assert.Contains(t, got, "+231/-18")
		assert.Contains(t, got, "A2 M20")
		assert.Contains(t, got, "git")
		assert.Contains(t, got, " · ", "segments joined with middot separator")
	})

	t.Run("all-files mode drops lines segment", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{Enabled: true, AllFiles: true, VCS: "git"}
		m.review.entries = make([]diff.FileEntry, 12)
		got := m.reviewFooterText()
		assert.Contains(t, got, "12 tracked files")
		assert.NotContains(t, got, "+0/-0", "lines segment must be hidden in --all-files mode")
		assert.NotContains(t, got, "loading")
	})

	t.Run("stdin mode drops vcs segment (it's already in header)", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{Enabled: true, Stdin: true, VCS: "stdin"}
		m.review.entries = make([]diff.FileEntry, 1)
		m.review.statsLoaded = true
		got := m.reviewFooterText()
		assert.NotContains(t, got, "stdin", "vcs segment is suppressed when it would just say 'stdin'")
	})

	t.Run("empty config returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{}
		assert.Empty(t, m.reviewFooterText())
	})

	t.Run("disabled config with files loaded still returns empty", func(t *testing.T) {
		// Enabled=false is the off-switch for the entire review-info subsystem;
		// once files load the footer must STILL stay empty so it cannot get
		// stuck on "loading…" (triggerReviewStats short-circuits when disabled,
		// so stats never resolve in this state).
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{}
		m.review.entries = make([]diff.FileEntry, 5)
		m.review.statusCounts = map[diff.FileStatus]int{diff.FileModified: 5}
		assert.Empty(t, m.reviewFooterText())
	})
}

func TestModel_InfoOverlay_RefreshesOnStatsLoad(t *testing.T) {
	// regression: when the user opens the info popup before the lazy stats
	// fetch lands, the popup must refresh inline so the "loading…" footer
	// flips to the totals — without this, the spec captured at open() time
	// stays stale forever and the user has to dismiss/reopen.
	entries := []diff.FileEntry{{Path: "a.go", Status: diff.FileModified}}
	r := &mocks.RendererMock{
		ChangedFilesFunc: func(string, bool) ([]diff.FileEntry, error) { return entries, nil },
		FileDiffFunc: func(string, string, bool, int) ([]diff.DiffLine, error) {
			return []diff.DiffLine{{ChangeType: diff.ChangeAdd}, {ChangeType: diff.ChangeAdd}, {ChangeType: diff.ChangeRemove}}, nil
		},
	}
	mgr := overlay.NewManager()
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{
		Overlay:    mgr,
		ReviewInfo: ReviewInfoConfig{Enabled: true},
	})
	// drive setReviewEntries directly to simulate post-files-load state
	m.setReviewEntries(entries)

	// open popup with stats unloaded — footer should report loading
	cmd := m.handleInfo()
	require.NotNil(t, cmd, "first open must trigger a stats fetch")
	require.True(t, mgr.Active())
	require.Equal(t, overlay.KindInfo, mgr.Kind())

	// run the lazy fetch and dispatch the result
	statsMsg := cmd().(reviewStatsLoadedMsg)
	result, _ := m.Update(statsMsg)
	m = result.(Model)

	// after the stats land, the open popup's spec must reflect the totals,
	// not the original "loading…" snapshot. Build a fresh bg of plain spaces
	// and call Compose; the rendered popup is overlaid in the middle.
	ctx := overlay.RenderCtx{Width: 100, Height: 20, Resolver: style.PlainResolver()}
	bgRow := strings.Repeat(" ", 100)
	bg := strings.Repeat(bgRow+"\n", 20)
	out := mgr.Compose(bg, ctx)
	assert.Contains(t, out, "+2/-1", "open popup must update inline once stats land")
	assert.NotContains(t, out, "loading…", "loading placeholder must clear after refresh")
}

func TestModel_HighlightDescription(t *testing.T) {
	t.Run("empty description returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = ReviewInfoConfig{}
		assert.Empty(t, m.highlightDescription())
	})

	t.Run("multi-line description goes through highlighter", func(t *testing.T) {
		called := false
		var seenFile string
		var seenLineCount int
		m := testNewModel(t, &mocks.RendererMock{}, annotation.NewStore(), &mocks.SyntaxHighlighterMock{
			HighlightLinesFunc: func(filename string, lines []diff.DiffLine) []string {
				called = true
				seenFile = filename
				seenLineCount = len(lines)
				out := make([]string, len(lines))
				for i, l := range lines {
					out[i] = "[hl]" + l.Content
				}
				return out
			},
			SetStyleFunc:  func(string) bool { return true },
			StyleNameFunc: func() string { return "test" },
		}, ModelConfig{ReviewInfo: ReviewInfoConfig{Description: "# Heading\n\nbody text\n\n```go\ncode block\n```"}})

		got := m.highlightDescription()
		assert.True(t, called, "highlighter must be invoked for non-empty description")
		assert.Equal(t, "description.md", seenFile, "filename hint must be .md so chroma picks the markdown lexer")
		assert.Equal(t, 7, seenLineCount, "line count must match raw newline-split")
		assert.Contains(t, got, "[hl]# Heading")
		assert.Contains(t, got, "[hl]```go")
	})

	t.Run("highlighted description appears in popup spec", func(t *testing.T) {
		m := testNewModel(t, &mocks.RendererMock{}, annotation.NewStore(), &mocks.SyntaxHighlighterMock{
			HighlightLinesFunc: func(_ string, lines []diff.DiffLine) []string {
				out := make([]string, len(lines))
				for i, l := range lines {
					out[i] = l.Content
				}
				return out
			},
			SetStyleFunc:  func(string) bool { return true },
			StyleNameFunc: func() string { return "test" },
		}, ModelConfig{ReviewInfo: ReviewInfoConfig{Enabled: true, Description: "agent prose"}})

		spec := m.buildInfoSpec()
		assert.Equal(t, "agent prose", spec.Description)
	})

	t.Run("escape sequences and control bytes are stripped before highlighting", func(t *testing.T) {
		var seenLines []diff.DiffLine
		m := testNewModel(t, &mocks.RendererMock{}, annotation.NewStore(), &mocks.SyntaxHighlighterMock{
			HighlightLinesFunc: func(_ string, lines []diff.DiffLine) []string {
				seenLines = append([]diff.DiffLine(nil), lines...)
				out := make([]string, len(lines))
				for i, l := range lines {
					out[i] = l.Content
				}
				return out
			},
			SetStyleFunc:  func(string) bool { return true },
			StyleNameFunc: func() string { return "test" },
		}, ModelConfig{ReviewInfo: ReviewInfoConfig{
			// embed: ESC, BEL, NUL, CRLF, lone CR, plus benign tab + newline
			Description: "safe\x1b[31mred\x1b[0m\a\x00line\r\nwindows\rmac\tindented\nend",
		}})

		got := m.highlightDescription()
		require.NotEmpty(t, seenLines)
		// every line passed to the highlighter must be free of ESC/BEL/NUL
		for _, dl := range seenLines {
			assert.NotContains(t, dl.Content, "\x1b", "ESC must be stripped: %q", dl.Content)
			assert.NotContains(t, dl.Content, "\x00", "NUL must be stripped: %q", dl.Content)
			assert.NotContains(t, dl.Content, "\a", "BEL must be stripped: %q", dl.Content)
			assert.NotContains(t, dl.Content, "\r", "CR must be normalized: %q", dl.Content)
		}
		// CRLF and lone CR collapse to LF, producing real paragraph breaks
		assert.Contains(t, got, "windows")
		assert.Contains(t, got, "mac")
		assert.Contains(t, got, "\tindented", "tabs must be preserved (markdown indentation)")
	})
}
