package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/sidepane"
	"github.com/umputun/revdiff/app/ui/style"
)

func TestModel_ResizeInSingleFileMode(t *testing.T) {
	m := testModel(nil, nil)
	// set up single-file mode via filesLoadedMsg
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	loaded, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "main.go"}}})
	m = loaded.(Model)
	require.True(t, m.file.singleFile)

	// resize while in single-file mode
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)

	assert.Equal(t, 0, model.layout.treeWidth, "treeWidth stays 0 after resize in single-file mode")
	assert.Equal(t, 78, model.layout.viewport.Width, "viewport width should be new width - 2")
}

func TestModel_StatusBarFilterIndicator(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("filter icon shown when filter active", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = testNewFileTree([]string{"a.go"})
		m.tree.ToggleFilter(map[string]bool{"a.go": true})
		m.ready = true
		m.file.name = "a.go"
		m.file.lines = lines
		m.layout.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "◉", "should show filter icon when filter active")
	})

	t.Run("filter icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = testNewFileTree([]string{"a.go"})
		m.ready = true
		m.file.name = "a.go"
		m.file.lines = lines
		m.layout.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "◉", "indicator always shown, muted when inactive")
	})
}

func TestModel_StatusModeIcons(t *testing.T) {
	t.Run("all icons always present", func(t *testing.T) {
		m := testModel(nil, nil)
		icons := m.statusModeIcons()
		assert.Contains(t, icons, "▼")
		assert.Contains(t, icons, "◉")
		assert.Contains(t, icons, "↩")
		assert.Contains(t, icons, "≋")
	})

	t.Run("with colors active icons use status fg", func(t *testing.T) {
		sc := style.Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
		m := testModel(nil, nil)
		res := style.NewResolver(sc)
		m.resolver = res
		m.renderer = style.NewRenderer(res)
		m.modes.collapsed.enabled = true
		// filter is off by default
		icons := m.statusModeIcons()
		// active collapsed icon should have status fg sequence
		assert.Contains(t, icons, "\033[38;2;32;32;32m▼")
		// inactive filter icon should have muted fg sequence
		assert.Contains(t, icons, "\033[38;2;108;108;108m◉")
	})
}

func TestModel_StatusBarWrapIndicator(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("wrap icon shown when active", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = testNewFileTree([]string{"a.go"})
		m.ready = true
		m.file.name = "a.go"
		m.file.lines = lines
		m.modes.wrap = true
		m.layout.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "↩", "should show wrap icon when wrap active")
	})

	t.Run("wrap icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = testNewFileTree([]string{"a.go"})
		m.ready = true
		m.file.name = "a.go"
		m.file.lines = lines
		m.layout.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "↩", "indicator always shown, muted when inactive")
	})
}

func TestModel_StatusBarShowsFilenameAndStats(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.ready = true
	m.file.name = "a.go"
	m.file.lines = lines
	m.file.adds = 1
	m.layout.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "a.go", "status bar should show filename")
	assert.Contains(t, status, "+1/-0", "status bar should show diff stats")
	assert.Contains(t, status, "? help", "status bar should show help hint")
}

func TestModel_StatusBarFilenameTruncation(t *testing.T) {
	longFile := "very/long/path/to/some/deeply/nested/file/in/the/project/structure.go"
	m := testModel(nil, nil)
	m.file.name = longFile
	m.file.adds = 3
	m.file.removes = 1
	m.layout.focus = paneDiff
	m.layout.width = 40 // narrow terminal forces truncation

	status := m.statusBarText()
	assert.Contains(t, status, "…", "should truncate filename with ellipsis")
	assert.Contains(t, status, "+3/-1", "should still show stats after truncation")
	assert.Contains(t, status, "? help", "should still show help hint")
}

func TestModel_StatusBarFilenameTruncationWideChars(t *testing.T) {
	// CJK characters are 2 display cells wide per rune, the truncation must use
	// display-width measurement, not rune count, to avoid overflowing the status line
	wideFile := "path/to/日本語のファイル名/テスト.go"
	m := testModel(nil, nil)
	m.file.name = wideFile
	m.file.adds = 1
	m.file.removes = 0
	m.layout.focus = paneDiff
	m.layout.width = 45

	status := m.statusBarText()
	assert.Contains(t, status, "…", "should truncate wide-char filename with ellipsis")
	assert.Contains(t, status, "+1/-0", "should still show stats after truncation")
	assert.LessOrEqual(t, lipgloss.Width(status), m.layout.width-2, "status text must fit within terminal width minus padding")
}

func TestModel_StatusBarModeIndicators(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff
	m.layout.width = 200

	t.Run("both indicators when collapsed and filtered", func(t *testing.T) {
		m.modes.collapsed.enabled = true
		m.modes.collapsed.expandedHunks = make(map[int]bool)
		if !m.tree.FilterActive() {
			m.tree.ToggleFilter(map[string]bool{"a.go": true})
		}
		status := m.statusBarText()
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
	})

	t.Run("indicators always present in default mode", func(t *testing.T) {
		m.modes.collapsed.enabled = false
		if m.tree.FilterActive() {
			m.tree.ToggleFilter(nil) // toggle filter off
		}
		status := m.statusBarText()
		assert.Contains(t, status, "▼", "always shown, muted when inactive")
		assert.Contains(t, status, "◉", "always shown, muted when inactive")
	})
}

func TestModel_StatusBarNarrowTerminalDegradation(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1
	m.file.adds = 1
	m.layout.focus = paneDiff
	m.modes.collapsed.enabled = true
	m.modes.collapsed.expandedHunks = make(map[int]bool)
	if !m.tree.FilterActive() {
		m.tree.ToggleFilter(map[string]bool{"a.go": true})
	}

	t.Run("wide terminal shows all segments", func(t *testing.T) {
		m.layout.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "hunk 1/1")
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
		assert.Contains(t, status, "? help")
	})

	t.Run("narrow terminal drops hunk from left first", func(t *testing.T) {
		m.layout.width = 50
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "? help")
	})

	t.Run("very narrow terminal drops hunk info", func(t *testing.T) {
		m.layout.width = 28
		status := m.statusBarText()
		assert.Contains(t, status, "? help")
		assert.NotContains(t, status, "hunk", "hunk should be dropped on very narrow terminal")
	})
}

func TestModel_StatusBarStatsDisplay(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "main.go"
	m.file.adds = 10
	m.file.removes = 5
	m.layout.width = 120

	status := m.statusBarText()
	assert.Contains(t, status, "main.go")
	assert.Contains(t, status, "+10/-5")
}

func TestModel_StatusBarNoShortcutHintsInDiffPane(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.ready = true
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.annot.cursorOnAnnotation = true
	m.layout.focus = paneDiff
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: " ", Comment: "review this"})

	status := m.statusBarText()
	// shortcut hints moved to help overlay
	assert.NotContains(t, status, "[d]")
	assert.NotContains(t, status, "[enter/a]")
	assert.NotContains(t, status, "[A]")
	assert.NotContains(t, status, "[Q]")
	assert.NotContains(t, status, "[q]")
	// should show filename, stats, annotation count, help hint
	assert.Contains(t, status, "a.go")
	assert.Contains(t, status, "1 annotation")
	assert.Contains(t, status, "? help")
}

func TestModel_StatusBarShowsHelpHint(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	m.ready = true
	m.file.name = "a.go"
	m.file.lines = lines

	m.layout.focus = paneTree
	status := m.statusBarText()
	assert.Contains(t, status, "? help", "tree pane should show help hint")

	m.layout.focus = paneDiff
	status = m.statusBarText()
	assert.Contains(t, status, "? help", "diff pane should show help hint")
}

func TestModel_StatusBarNoFilenameWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.ready = true
	m.file.name = ""
	m.layout.focus = paneTree

	status := m.statusBarText()
	assert.NotContains(t, status, "/-", "no diff stats should be shown without a file")
	assert.Contains(t, status, "? help")
}

func TestModel_StatusBarShowsHunkIndicator(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},
	}

	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 1
	m.file.name = "a.go"
	m.layout.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/2")

	m.nav.diffCursor = 3
	status = m.statusBarText()
	assert.Contains(t, status, "hunk 2/2")
}

func TestModel_StatusBarNoHunkIndicatorWithoutChanges(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}

	m := testModel(nil, nil)
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.file.name = "a.go"
	m.layout.focus = paneDiff

	status := m.statusBarText()
	assert.NotContains(t, status, "hunk", "should not show hunk when cursor on context line")
}

func TestModel_StatusBarHunkOnlyInDiffPane(t *testing.T) {
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.nav.diffCursor = 0
	m.layout.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/1")

	// tree pane should not show hunk
	m.layout.focus = paneTree
	status = m.statusBarText()
	assert.NotContains(t, status, "hunk")
}

func TestModel_StatusBarHunkCountOnContextLine(t *testing.T) {
	t.Run("plural hunks", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},
			{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
			{NewNum: 4, Content: "add2", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 0
		m.file.name = "a.go"
		m.layout.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "2 hunks")
		assert.NotContains(t, status, "hunk 0/")
	})

	t.Run("singular hunk", func(t *testing.T) {
		lines := []diff.DiffLine{
			{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "add1", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 0
		m.file.name = "a.go"
		m.layout.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "1 hunk")
		assert.NotContains(t, status, "1 hunks", "should use singular form for one hunk")
	})
}

func TestModel_StatusBarShowsLineNumber(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 10, NewNum: 10, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 11, NewNum: 11, Content: "ctx2", ChangeType: diff.ChangeContext},
	}
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 0
	m.file.adds = 0
	m.layout.focus = paneDiff
	m.layout.width = 200

	status := m.statusBarText()
	assert.Contains(t, status, "L:10/11", "status bar should show line number")
}

func TestModel_StatusBarLineNumberAfterHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 0, NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1
	m.file.adds = 1
	m.layout.focus = paneDiff
	m.layout.width = 200

	status := m.statusBarText()
	hunkIdx := strings.Index(status, "hunk")
	lineIdx := strings.Index(status, "L:2/2")
	assert.Greater(t, hunkIdx, -1, "should contain hunk segment")
	assert.Greater(t, lineIdx, -1, "should contain line number segment")
	assert.Greater(t, lineIdx, hunkIdx, "line number should appear after hunk")
}

func TestModel_StatusBarLineNumberDegradation(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 0, NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.file.name = "a.go"
	m.file.lines = lines
	m.nav.diffCursor = 1
	m.file.adds = 1
	m.layout.focus = paneDiff

	t.Run("wide terminal shows line number", func(t *testing.T) {
		m.layout.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "L:2/2")
	})

	t.Run("no-search level still shows line number", func(t *testing.T) {
		segments := m.statusSegmentsNoSearch()
		joined := strings.Join(segments, " | ")
		assert.Contains(t, joined, "L:2/2")
	})

	t.Run("minimal level drops line number", func(t *testing.T) {
		segments := m.statusSegmentsMinimal()
		joined := strings.Join(segments, " | ")
		assert.NotContains(t, joined, "L:", "minimal degradation should not contain line number")
	})
}

func TestModel_StatusBarPipeSeparators(t *testing.T) {
	t.Run("plain styles", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.file.adds = 1
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "no full ANSI reset in separator")
	})

	t.Run("with colors", func(t *testing.T) {
		sc := style.Colors{Muted: "#6c6c6c", StatusFg: "#202020", StatusBg: "#C5794F", Normal: "#d0d0d0"}
		m := testModel(nil, nil)
		res := style.NewResolver(sc)
		m.resolver = res
		m.renderer = style.NewRenderer(res)
		m.file.name = "a.go"
		m.file.lines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.file.adds = 1
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "separator must use raw ANSI fg, not lipgloss Render")
		assert.Contains(t, status, "\033[38;2;108;108;108m", "separator should have muted fg ANSI sequence")
		assert.Contains(t, status, "\033[38;2;32;32;32m", "separator should restore status fg after pipe")
	})
}

func TestModel_ViewNoStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = testNewFileTree([]string{"a.go"})
	m.cfg.noStatusBar = true
	m.ready = true
	m.layout.width = 120
	m.layout.height = 40
	m.layout.treeWidth = 24
	view := m.View()
	assert.NotContains(t, view, "quit", "status bar should be hidden")
	assert.Contains(t, view, "a.go", "tree content should still appear")
}

func TestModel_QKeyNoStatusBarSkipsConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.cfg.noStatusBar = true
	m.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "test"})

	result, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	require.NotNil(t, cmd)

	model := result.(Model)
	assert.True(t, model.Discarded(), "should immediately discard when status bar is hidden")
	assert.False(t, model.inConfirmDiscard, "should not enter confirming state without status bar")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "should quit immediately")
}

func TestModel_StatusBarNoKeyHints(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.layout.width = 120

	t.Run("tree pane has no shortcut hints", func(t *testing.T) {
		m.layout.focus = paneTree
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[j/k]")
		assert.Contains(t, status, "? help")
	})

	t.Run("diff pane has no shortcut hints", func(t *testing.T) {
		m.layout.focus = paneDiff
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[enter/a]")
		assert.Contains(t, status, "? help")
	})
}

func TestModel_ViewSingleFileMode(t *testing.T) {
	t.Run("single-file mode renders full-width diff without tree pane", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.tree = testNewFileTree([]string{"main.go"})
		m.file.singleFile = true
		m.layout.treeWidth = 0
		m.layout.focus = paneDiff
		m.file.name = "main.go"
		m.cfg.noStatusBar = true
		m.ready = true

		view := m.View()
		assert.Contains(t, view, "main.go")

		// every rendered line must be full terminal width (diff pane uses m.layout.width - 2 + 2 border = m.layout.width)
		lines := strings.Split(view, "\n")
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w == 0 {
				continue // skip empty trailing lines
			}
			assert.Equal(t, m.layout.width, w, "line %d should be full width (%d), got %d", i, m.layout.width, w)
		}

		// single-file mode must not contain adjacent pane borders (││) from JoinHorizontal
		stripped := ansi.Strip(view)
		assert.NotContains(t, stripped, "││", "single-file mode should not have two adjacent pane borders")
	})

	t.Run("multi-file mode renders tree and diff panes side by side", func(t *testing.T) {
		m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
		m.tree = testNewFileTree([]string{"internal/a.go", "internal/b.go"})
		m.file.singleFile = false
		m.layout.focus = paneTree
		m.cfg.noStatusBar = true
		m.ready = true

		view := m.View()
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "a.go")
		assert.Contains(t, stripped, "b.go")

		// multi-file mode should have adjacent pane borders from JoinHorizontal
		assert.Contains(t, stripped, "││", "multi-file mode should have two pane borders from tree+diff join")
	})
}

func TestModel_DiffContentWidthSingleFile(t *testing.T) {
	t.Run("single-file mode", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.layout.width = 100
		m.layout.treeWidth = 0
		assert.Equal(t, 96, m.diffContentWidth()) // width - 4 (borders + cursor bar + right padding)
	})

	t.Run("multi-file mode", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.file.singleFile = false
		m.layout.width = 120
		m.layout.treeWidth = 36
		assert.Equal(t, 78, m.diffContentWidth()) // 120 - 36 - 4 - 2
	})

	t.Run("single-file mode minimum width", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.layout.width = 5
		m.layout.treeWidth = 0
		assert.Equal(t, 10, m.diffContentWidth()) // min 10
	})
}

func TestModel_SingleFileKeysNoOp(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "line two", ChangeType: diff.ChangeAdd},
	}
	setup := func() Model {
		m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
		m.tree = testNewFileTree([]string{"main.go"})
		m.file.singleFile = true
		m.layout.focus = paneDiff
		m.file.name = "main.go"
		m.file.lines = lines
		m.file.highlighted = noopHighlighter().HighlightLines("main.go", lines)
		plainRes := style.PlainResolver()
		m.resolver = plainRes
		m.renderer = style.NewRenderer(plainRes)
		return m
	}

	t.Run("tab is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "tab should not switch pane in single-file mode")
	})

	t.Run("h is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "h should not switch to tree in single-file mode")
	})

	t.Run("f is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		m.store.Add(annotation.Annotation{File: "main.go", Line: 1, Type: "+", Comment: "test"})
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.FilterActive(), "f should not toggle filter in single-file mode")
	})

	t.Run("p is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.SelectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.SelectedFile(), "p should not change file in single-file mode")
	})

	t.Run("n is no-op for file nav in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.SelectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.SelectedFile(), "n should not advance file in single-file mode")
	})
}

func TestModel_SingleFileSearchNavStillWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no hit", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.file.singleFile = true
	model.layout.focus = paneDiff
	model.search.matches = []int{0, 2}
	model.search.cursor = 0
	model.nav.diffCursor = 0

	// n should navigate to next search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 1, model.search.cursor, "n should advance search cursor in single-file mode")
	assert.Equal(t, 2, model.nav.diffCursor, "cursor should move to second match")

	// n should navigate to previous search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 0, model.search.cursor, "N should go back in single-file mode")
	assert.Equal(t, 0, model.nav.diffCursor, "cursor should return to first match")
}

func TestModel_SingleFileWrapModeWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "short line", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: strings.Repeat("long ", 50), ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.file.singleFile = true
	model.layout.focus = paneDiff

	assert.False(t, model.modes.wrap, "wrap should be off initially")

	// toggle wrap on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.True(t, model.modes.wrap, "w should toggle wrap on in single-file mode")
	assert.Equal(t, 0, model.layout.scrollX, "wrap should reset horizontal scroll")

	// toggle wrap off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.modes.wrap, "w should toggle wrap off in single-file mode")
}

func TestModel_SingleFileCollapsedModeWorks(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"main.go"}, map[string][]diff.DiffLine{"main.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "main.go", lines: lines})
	model = result.(Model)
	model.file.singleFile = true
	model.layout.focus = paneDiff

	assert.False(t, model.modes.collapsed.enabled, "collapsed should be off initially")

	// toggle collapsed on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.True(t, model.modes.collapsed.enabled, "v should toggle collapsed on in single-file mode")

	// toggle collapsed off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.False(t, model.modes.collapsed.enabled, "v should toggle collapsed off in single-file mode")
}

func TestModel_SingleFileMultiFileModeUnchanged(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line one", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "a.go"}, {Path: "b.go"}}})
	model = result.(Model)

	assert.False(t, model.file.singleFile, "multi-file should not be in single-file mode")
	assert.Equal(t, paneTree, model.layout.focus, "multi-file should start on tree pane")

	// tab should switch panes
	model.layout.focus = paneTree
	model.file.name = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus, "tab should switch to diff pane in multi-file mode")

	// tab back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneTree, model.layout.focus, "tab should switch back to tree in multi-file mode")

	// f should toggle filter (with annotations present)
	model.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	model.layout.focus = paneDiff
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = result.(Model)
	assert.True(t, model.tree.FilterActive(), "f should toggle filter in multi-file mode")
}

func TestModel_ResizeWithTOCActive(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "## Section", ChangeType: diff.ChangeContext},
	}

	t.Run("resize preserves treeWidth when TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)

		result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		model := result.(Model)

		expectedTreeWidth := max(minTreeWidth, 100*model.cfg.treeWidthRatio/10)
		assert.Equal(t, expectedTreeWidth, model.layout.treeWidth, "treeWidth should be ratio-based when TOC is active")
		assert.Equal(t, 100-expectedTreeWidth-4, model.layout.viewport.Width, "viewport width accounts for TOC pane")
	})

	t.Run("resize sets treeWidth=0 when single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.file.mdTOC = nil

		result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		model := result.(Model)

		assert.Equal(t, 0, model.layout.treeWidth, "treeWidth should be 0 for single-file without TOC")
		assert.Equal(t, 78, model.layout.viewport.Width, "viewport width should be width - 2")
	})
}

func TestModel_DiffContentWidthWithTOC(t *testing.T) {
	m := testModel([]string{"README.md"}, nil)
	m.file.singleFile = true
	m.layout.width = 100
	m.layout.treeWidth = 30
	m.file.mdTOC = sidepane.ParseTOC([]diff.DiffLine{{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext}}, "README.md")

	// with TOC active, should use multi-file formula: width - treeWidth - 4 - 2
	assert.Equal(t, 64, m.diffContentWidth()) // 100 - 30 - 4 - 2
}

func TestModel_ViewWithTOCPane(t *testing.T) {
	t.Run("markdown single-file with TOC renders two-pane layout", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.tree = testNewFileTree([]string{"README.md"})
		m.file.singleFile = true
		m.layout.treeWidth = 25
		m.layout.focus = paneDiff
		m.file.name = "README.md"
		m.cfg.noStatusBar = true
		m.ready = true
		tocLines := []diff.DiffLine{
			{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
			{NewNum: 2, Content: "text", ChangeType: diff.ChangeContext},
			{NewNum: 3, Content: "## Section", ChangeType: diff.ChangeContext},
		}
		m.file.mdTOC = sidepane.ParseTOC(tocLines, "README.md")

		view := m.View()
		stripped := ansi.Strip(view)

		// TOC pane should contain header titles
		assert.Contains(t, stripped, "Title")
		assert.Contains(t, stripped, "Section")

		// two-pane layout should have adjacent pane borders from JoinHorizontal
		assert.Contains(t, stripped, "││", "TOC + diff layout should have two adjacent pane borders")
	})

	t.Run("non-markdown single-file without TOC renders full width", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.tree = testNewFileTree([]string{"main.go"})
		m.file.singleFile = true
		m.layout.treeWidth = 0
		m.layout.focus = paneDiff
		m.file.name = "main.go"
		m.cfg.noStatusBar = true
		m.ready = true

		view := m.View()
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "main.go")

		// single-file mode without TOC must not have adjacent pane borders
		assert.NotContains(t, stripped, "││", "single-file without TOC should not have two pane borders")
	})

	t.Run("TOC pane uses active style when focused", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.tree = testNewFileTree([]string{"README.md"})
		m.file.singleFile = true
		m.layout.treeWidth = 25
		m.layout.focus = paneTree // TOC pane focused
		m.file.name = "README.md"
		m.cfg.noStatusBar = true
		m.ready = true
		m.file.mdTOC = sidepane.ParseTOC([]diff.DiffLine{{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext}}, "README.md")

		view := m.View()
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "Title")
		// two-pane layout present
		assert.Contains(t, stripped, "││")
	})
}

func TestModel_DiffContentWidthWithTOCActive(t *testing.T) {
	t.Run("single-file without TOC uses full width", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.layout.width = 100
		m.layout.treeWidth = 0
		assert.Equal(t, 96, m.diffContentWidth()) // width - 4
	})

	t.Run("single-file with TOC uses multi-file formula", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.file.singleFile = true
		m.layout.width = 100
		m.layout.treeWidth = 30
		m.file.mdTOC = sidepane.ParseTOC([]diff.DiffLine{{NewNum: 1, Content: "# T", ChangeType: diff.ChangeContext}}, "t.md")
		assert.Equal(t, 64, m.diffContentWidth()) // 100 - 30 - 4 - 2
	})

	t.Run("minimum width enforced", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.file.singleFile = true
		m.layout.width = 20
		m.layout.treeWidth = 15
		m.file.mdTOC = sidepane.ParseTOC([]diff.DiffLine{{NewNum: 1, Content: "# T", ChangeType: diff.ChangeContext}}, "t.md")
		assert.Equal(t, 10, m.diffContentWidth()) // min 10
	})
}

func TestModel_TabTogglingWithTOC(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "some text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Section", ChangeType: diff.ChangeContext},
	}

	t.Run("tab cycles between TOC and diff", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.layout.focus = paneDiff

		// tab from diff -> TOC (paneTree)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus)

		// tab from TOC -> diff
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model = result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus)
	})

	t.Run("tab no-op in single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.file.mdTOC = nil
		m.file.name = "main.go"
		m.layout.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "tab should be no-op without TOC in single-file mode")
	})
}

func TestModel_HKeySwitchesToTOC(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "## Section", ChangeType: diff.ChangeContext},
	}

	t.Run("h key in diff pane switches to TOC", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.file.lines = mdLines
		m.layout.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus, "h key should switch to TOC pane")
	})

	t.Run("h key no-op in single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.file.singleFile = true
		m.file.mdTOC = nil
		m.file.name = "main.go"
		m.layout.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "h key should be no-op without TOC")
	})
}

func TestModel_TOCPaneNavigation(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Second", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "text", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "### Third", ChangeType: diff.ChangeContext},
	}

	setup := func(t *testing.T) Model {
		t.Helper()
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.file.lines = mdLines
		m.layout.focus = paneTree
		return m
	}

	// TOC entries: [0]=⌂ top (lineIdx=0), [1]=First (lineIdx=0), [2]=Second (lineIdx=2), [3]=Third (lineIdx=4)

	t.Run("j moves cursor down in TOC and auto-jumps diff", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, tocLineIdx(t, m.file.mdTOC)) // starts on "top" entry (lineIdx=0)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 0, tocLineIdx(t, model.file.mdTOC)) // First entry also at lineIdx=0
		assert.Equal(t, 0, model.nav.diffCursor, "diff cursor should jump to # First at index 0")
		assert.Equal(t, paneTree, model.layout.focus, "focus should stay on TOC pane")
	})

	t.Run("k moves cursor up in TOC and auto-jumps diff", func(t *testing.T) {
		m := setup(t)
		moveTOCTo(m.file.mdTOC, 3) // on "Third"

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 2, tocLineIdx(t, model.file.mdTOC), "should be on Second (lineIdx=2)")
		assert.Equal(t, 2, model.nav.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneTree, model.layout.focus, "focus should stay on TOC pane")
	})

	t.Run("j clamped at last entry", func(t *testing.T) {
		m := setup(t)
		moveTOCTo(m.file.mdTOC, 3) // last entry (Third)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 4, tocLineIdx(t, model.file.mdTOC), "should stay on Third (lineIdx=4)")
	})

	t.Run("k clamped at first entry", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, tocLineIdx(t, m.file.mdTOC)) // starts at top (lineIdx=0)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 0, tocLineIdx(t, model.file.mdTOC), "should stay at top (lineIdx=0)")
	})

	t.Run("home moves to first entry", func(t *testing.T) {
		m := setup(t)
		moveTOCTo(m.file.mdTOC, 3)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
		model := result.(Model)
		assert.Equal(t, 0, tocLineIdx(t, model.file.mdTOC), "should be at top (lineIdx=0)")
	})

	t.Run("end moves to last entry", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, tocLineIdx(t, m.file.mdTOC)) // starts at top (lineIdx=0)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		model := result.(Model)
		assert.Equal(t, 4, tocLineIdx(t, model.file.mdTOC), "should be on Third (lineIdx=4)")
	})

	t.Run("l switches to diff pane", func(t *testing.T) {
		m := setup(t)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus)
	})

	t.Run("pgdn moves cursor by page size", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, tocLineIdx(t, m.file.mdTOC)) // starts at top

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		model := result.(Model)
		assert.Equal(t, 4, tocLineIdx(t, model.file.mdTOC), "pgdn should move to last entry (lineIdx=4)")
	})

	t.Run("pgup moves cursor by page size", func(t *testing.T) {
		m := setup(t)
		moveTOCTo(m.file.mdTOC, 3)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		model := result.(Model)
		assert.Equal(t, 0, tocLineIdx(t, model.file.mdTOC), "pgup should move to first entry")
	})

	t.Run("tab back to TOC syncs cursor to active section", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 0)          // cursor was on top entry
		m.file.mdTOC.UpdateActiveSection(4) // diff scrolled to Third section (lineIdx=4 → entry 3)
		m.nav.diffCursor = 4                // cursor on Third header line

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus)
		assert.Equal(t, 4, tocLineIdx(t, model.file.mdTOC), "TOC cursor should sync to active section on tab back (lineIdx=4)")
	})

	t.Run("h key syncs TOC cursor to active section", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 0)
		m.file.mdTOC.UpdateActiveSection(2) // second section (lineIdx=2 → entry 2)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneTree, model.layout.focus)
		assert.Equal(t, 2, tocLineIdx(t, model.file.mdTOC), "TOC cursor should sync to active section on h key (lineIdx=2)")
	})

	t.Run("n jumps to next TOC entry from diff pane", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 1)          // on First
		m.file.mdTOC.UpdateActiveSection(0) // entry 1 has lineIdx=0; UpdateActiveSection(0) → activeSection=1

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, 2, tocLineIdx(t, model.file.mdTOC), "n should advance TOC cursor to Second (lineIdx=2)")
		assert.Equal(t, 2, model.nav.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneDiff, model.layout.focus, "focus should stay on diff pane")
	})

	t.Run("p jumps to prev TOC entry from diff pane", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 3)          // on Third
		m.file.mdTOC.UpdateActiveSection(4) // entry 3 has lineIdx=4

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, 2, tocLineIdx(t, model.file.mdTOC), "p should move TOC cursor to Second (lineIdx=2)")
		assert.Equal(t, 2, model.nav.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneDiff, model.layout.focus, "focus should stay on diff pane")
	})

	t.Run("n clamped at last TOC entry", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 3)          // last entry
		m.file.mdTOC.UpdateActiveSection(4) // entry 3 has lineIdx=4

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, 4, tocLineIdx(t, model.file.mdTOC), "should stay on Third (lineIdx=4)")
	})

	t.Run("p clamped at first TOC entry", func(t *testing.T) {
		m := setup(t)
		m.layout.focus = paneDiff
		moveTOCTo(m.file.mdTOC, 0) // cursor at first entry

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, 0, tocLineIdx(t, model.file.mdTOC), "should stay at top (lineIdx=0)")
	})
}

func TestModel_EnterInTOCPane(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# First", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "text line 1", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "text line 2", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "## Second", ChangeType: diff.ChangeContext},
		{NewNum: 5, Content: "text line 3", ChangeType: diff.ChangeContext},
		{NewNum: 6, Content: "### Third", ChangeType: diff.ChangeContext},
	}

	t.Run("enter jumps to header line", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.file.lines = mdLines
		m.file.highlighted = make([]string, len(mdLines))
		m.layout.focus = paneTree

		// move cursor to third entry (## Second at lineIdx=3), accounting for top entry at [0]
		moveTOCTo(m.file.mdTOC, 2)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)

		assert.Equal(t, 3, model.nav.diffCursor, "diffCursor should jump to Second header at index 3")
		assert.Equal(t, paneDiff, model.layout.focus, "focus should switch to diff pane after enter")
		assert.Equal(t, 3, tocActiveLineIdx(t, model.file.mdTOC), "active section should track jumped entry (lineIdx=3)")
	})

	t.Run("enter on last TOC entry", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.file.singleFile = true
		m.file.mdTOC = sidepane.ParseTOC(mdLines, "README.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "README.md"
		m.file.lines = mdLines
		m.file.highlighted = make([]string, len(mdLines))
		m.layout.focus = paneTree
		moveTOCTo(m.file.mdTOC, 3) // ### Third at lineIdx=5, accounting for top entry at [0]

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)

		assert.Equal(t, 5, model.nav.diffCursor, "diffCursor should jump to Third header at index 5")
		assert.Equal(t, paneDiff, model.layout.focus)
	})
}

func TestModel_MarkdownNoHeadersFallback(t *testing.T) {
	noHeaders := []diff.DiffLine{
		{NewNum: 1, Content: "just text", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "more text", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": noHeaders})
	m.file.singleFile = true
	m.layout.treeWidth = 0
	m.layout.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: noHeaders})
	model := result.(Model)

	assert.Nil(t, model.file.mdTOC, "mdTOC should be nil when no headers")
	assert.Equal(t, 0, model.layout.treeWidth, "treeWidth should be 0 in fallback mode")

	// tab should be no-op in single-file mode without TOC
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneDiff, model.layout.focus, "tab should be no-op without TOC")
}

func TestModel_StatusModeIconsLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.lineNumbers = true
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "#")
}

func TestModel_StatusModeIconsBlame(t *testing.T) {
	m := testModel(nil, nil)
	m.modes.showBlame = true
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "b")
	assert.NotContains(t, icons, "@")
}

func TestModel_StatusModeIconsWordDiff(t *testing.T) {
	m := testModel(nil, nil)
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "±", "word-diff icon should always be present")

	m.modes.wordDiff = true
	sc := style.Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
	res := style.NewResolver(sc)
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	icons = m.statusModeIcons()
	assert.Contains(t, icons, "\033[38;2;32;32;32m±", "active word-diff icon uses status fg")
}

func TestModel_StatusModeIconsCompact(t *testing.T) {
	m := testModel(nil, nil)
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "⊂", "compact icon should always be present")

	m.modes.compact = true
	sc := style.Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
	res := style.NewResolver(sc)
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	icons = m.statusModeIcons()
	assert.Contains(t, icons, "\033[38;2;32;32;32m⊂", "active compact icon uses status fg")
}

func TestModel_StatusModeIconsCompactWithCollapsed(t *testing.T) {
	m := testModel(nil, nil)
	sc := style.Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
	res := style.NewResolver(sc)
	m.resolver = res
	m.renderer = style.NewRenderer(res)
	m.modes.compact = true
	m.modes.collapsed.enabled = true

	icons := m.statusModeIcons()
	assert.Contains(t, icons, "\033[38;2;32;32;32m▼", "collapsed icon active with status fg")
	assert.Contains(t, icons, "\033[38;2;32;32;32m⊂", "compact icon active with status fg")
}

func TestModel_ReviewedStatusBar(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go", "c.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines, "c.go": lines,
	})
	m.tree = testNewFileTree([]string{"a.go", "b.go", "c.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.width = 200

	// no reviewed count when nothing reviewed
	status := m.statusBarText()
	assert.NotContains(t, status, "✓ 0")

	// mark one file as reviewed
	m.tree.ToggleReviewed("a.go")
	status = m.statusBarText()
	assert.Contains(t, status, "✓ 1/3", "status bar should show reviewed progress")

	// mark another
	m.tree.ToggleReviewed("b.go")
	status = m.statusBarText()
	assert.Contains(t, status, "✓ 2/3")
}

func TestModel_ReviewedModeIcon(t *testing.T) {
	m := testModel(nil, nil)
	m.tree = testNewFileTree([]string{"a.go"})

	icons := m.statusModeIcons()
	assert.Contains(t, icons, "✓", "reviewed icon should always be present")

	// when no files reviewed, icon should use muted color
	m.tree.ToggleReviewed("a.go")
	icons = m.statusModeIcons()
	assert.Contains(t, icons, "✓", "reviewed icon should be present when files reviewed")
}

func TestModel_ViewOutput(t *testing.T) {
	m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
	m.tree = testNewFileTree([]string{"internal/a.go", "internal/b.go"})
	m.ready = true

	// tree pane focused - should show file tree and help hint
	m.layout.focus = paneTree
	view := m.View()
	assert.Contains(t, view, "a.go")
	assert.Contains(t, view, "b.go")
	assert.Contains(t, view, "? help")

	// diff pane focused - should show help hint
	m.layout.focus = paneDiff
	view = m.View()
	assert.Contains(t, view, "? help")
}

func TestModel_ViewNotReady(t *testing.T) {
	m := testModel(nil, nil)
	m.ready = false

	assert.Equal(t, "loading...", m.View())
}

func TestModel_ViewScrollbarThumb(t *testing.T) {
	// testModel does not dispatch a WindowSizeMsg, so viewport.Height defaults
	// to 0; set it manually to make the scrollbar code path active.
	const vh = 30
	const vw = 80

	t.Run("thumb appears when content scrollable", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.file.name = "a.go"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent(strings.Repeat("filler line\n", 200))

		view := m.View()
		assert.Contains(t, view, scrollbarThumbRune, "scrollbar thumb should appear when content exceeds viewport")
	})

	t.Run("no thumb when content fits", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.file.name = "a.go"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent("one\ntwo\nthree\n")

		view := m.View()
		assert.NotContains(t, view, scrollbarThumbRune, "no thumb when content fits in viewport")
	})

	t.Run("thumb appears in single-file mode", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.tree = testNewFileTree([]string{"main.go"})
		m.file.singleFile = true
		m.layout.treeWidth = 0
		m.file.name = "main.go"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = m.layout.width - 2
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent(strings.Repeat("filler\n", 200))

		view := m.View()
		assert.Contains(t, view, scrollbarThumbRune, "single-file mode should also show thumb")
	})

	t.Run("thumb position shifts with YOffset", func(t *testing.T) {
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.file.name = "a.go"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		// 501 total lines, vh=30, thumbSize = 30*30/501 = 1
		m.layout.viewport.SetContent(strings.Repeat("filler\n", 500))

		collect := func(view string) []int {
			rows := []int{}
			for i, line := range strings.Split(view, "\n") {
				if strings.Contains(line, scrollbarThumbRune) {
					rows = append(rows, i)
				}
			}
			return rows
		}

		m.layout.viewport.SetYOffset(0)
		topRows := collect(m.View())
		require.Len(t, topRows, 1, "thumb size must be 1 row")
		assert.Equal(t, diffScrollbarFirstViewportRow, topRows[0], "yOff=0 must put thumb on first viewport row")

		m.layout.viewport.SetYOffset(471) // fully scrolled (total - vh = 471)
		bottomRows := collect(m.View())
		require.Len(t, bottomRows, 1, "thumb size invariant under offset")
		assert.Equal(t, diffScrollbarFirstViewportRow+vh-1, bottomRows[0], "fully-scrolled must put thumb on last viewport row")
	})

	t.Run("thumb appears with paneTree focus", func(t *testing.T) {
		// G7: scrollbar must work regardless of which pane has focus. with
		// paneTree focus the diff pane uses StyleKeyDiffPane (inactive border
		// color) instead of StyleKeyDiffPaneActive (accent). lipgloss might in
		// principle emit the inactive border via a different code path; assert
		// the thumb still lands.
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.file.name = "a.go"
		m.layout.focus = paneTree
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent(strings.Repeat("filler line\n", 200))

		view := m.View()
		assert.Contains(t, view, scrollbarThumbRune, "scrollbar thumb must appear even when tree pane has focus")
	})

	t.Run("navigation tree thumb appears when file list scrollable", func(t *testing.T) {
		files := make([]string, 1000)
		for i := range files {
			files[i] = fmt.Sprintf("pkg/file-%04d.go", i)
		}
		m := testModel(files, nil)
		m.tree = testNewFileTree(files)
		m.file.name = files[0]
		m.layout.focus = paneTree
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent("diff fits\n")

		rows := thumbRows(m.View())
		require.Len(t, rows, 1, "only the navigation pane should have a thumb")
		assert.Equal(t, navigationScrollbarFirstViewportRow, rows[0], "tree thumb starts on first content row")
	})

	t.Run("navigation tree thumb position shifts with cursor", func(t *testing.T) {
		files := make([]string, 1000)
		for i := range files {
			files[i] = fmt.Sprintf("pkg/file-%04d.go", i)
		}
		m := testModel(files, nil)
		m.tree = testNewFileTree(files)
		m.tree.Move(sidepane.MotionLast)
		m.file.name = files[0]
		m.layout.focus = paneTree
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent("diff fits\n")

		rows := thumbRows(m.View())
		require.Len(t, rows, 1, "only the navigation pane should have a thumb")
		assert.Equal(t, navigationScrollbarFirstViewportRow+m.paneHeight()-1, rows[0], "tree thumb reaches last content row")
	})

	t.Run("navigation tree thumb appears with paneDiff focus", func(t *testing.T) {
		// inactive-border style emits a different ANSI envelope around the right
		// border │ than the active style. the rune-level slice replacement must
		// land correctly regardless of the surrounding bytes.
		files := make([]string, 1000)
		for i := range files {
			files[i] = fmt.Sprintf("pkg/file-%04d.go", i)
		}
		m := testModel(files, nil)
		m.tree = testNewFileTree(files)
		m.file.name = files[0]
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent("diff fits\n")

		view := m.View()
		assert.Contains(t, view, scrollbarThumbRune, "navigation thumb must appear even when diff pane has focus")
	})

	t.Run("navigation TOC thumb appears when TOC scrollable", func(t *testing.T) {
		lines := make([]diff.DiffLine, 1000)
		for i := range lines {
			lines[i] = diff.DiffLine{Content: fmt.Sprintf("# Section %04d", i), ChangeType: diff.ChangeContext, NewNum: i + 1}
		}
		m := testModel([]string{"plan.md"}, nil)
		m.tree = testNewFileTree([]string{"plan.md"})
		m.file.singleFile = true
		m.file.mdTOC = testParseTOCFactory()(lines, "plan.md")
		require.NotNil(t, m.file.mdTOC)
		m.file.name = "plan.md"
		m.layout.focus = paneTree
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent("diff fits\n")

		rows := thumbRows(m.View())
		require.Len(t, rows, 1, "only the TOC pane should have a thumb")
		assert.Equal(t, navigationScrollbarFirstViewportRow, rows[0], "TOC thumb starts on first content row")
	})

	t.Run("thumb appears in markdown TOC layout", func(t *testing.T) {
		// G6: single-file markdown with TOC routes through renderTwoPaneLayout
		// (different switch arm than the file-tree default). cover that path.
		tocLines := []diff.DiffLine{
			{Content: "# Header 1", ChangeType: diff.ChangeContext, NewNum: 1},
			{Content: "", ChangeType: diff.ChangeContext, NewNum: 2},
			{Content: "## Header 2", ChangeType: diff.ChangeContext, NewNum: 3},
		}
		m := testModel([]string{"plan.md"}, nil)
		m.tree = testNewFileTree([]string{"plan.md"})
		m.file.singleFile = true
		m.file.mdTOC = testParseTOCFactory()(tocLines, "plan.md")
		require.NotNil(t, m.file.mdTOC, "test factory must produce a TOC")
		m.file.name = "plan.md"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent(strings.Repeat("body\n", 200))

		view := m.View()
		assert.Contains(t, view, scrollbarThumbRune, "scrollbar must work in markdown TOC layout")
	})

	t.Run("long filename does not push thumb past viewport rows", func(t *testing.T) {
		// C1 regression: before truncateHeaderTitle was added, lipgloss
		// soft-wrapped a too-long header onto multiple rows, which pushed the
		// real viewport rows past the hardcoded diffScrollbarFirstViewportRow=2
		// offset. truncating the header guarantees the layout invariant.
		m := testModel([]string{"a.go"}, nil)
		m.tree = testNewFileTree([]string{"a.go"})
		m.file.name = strings.Repeat("very/long/path/segment-", 40) + "structure.go"
		m.layout.focus = paneDiff
		m.layout.viewport.Width = vw
		m.layout.viewport.Height = vh
		m.layout.viewport.SetContent(strings.Repeat("filler\n", 200))
		m.layout.viewport.SetYOffset(0)

		view := m.View()
		thumbRowsFound := []int{}
		for i, line := range strings.Split(view, "\n") {
			if strings.Contains(line, scrollbarThumbRune) {
				thumbRowsFound = append(thumbRowsFound, i)
			}
		}
		require.NotEmpty(t, thumbRowsFound, "thumb must appear despite long filename")
		assert.Equal(t, diffScrollbarFirstViewportRow, thumbRowsFound[0],
			"long filename must be truncated so thumb still starts at row %d (header stays single-line)",
			diffScrollbarFirstViewportRow)

		// header row (index 1) must contain the truncation ellipsis, proving
		// the header was actually shortened rather than emitted whole
		lines := strings.Split(view, "\n")
		require.Greater(t, len(lines), 1)
		assert.Contains(t, lines[1], "…", "long header must be truncated with ellipsis")
	})
}

// codex iter-3 C1: crafted filenames must not break or spoof the status
// bar layout. iter-2 added sanitizeFilenameForDisplay but only wired it
// into the diff header; iter-3 wires it into status-bar segments too.
func TestModel_StatusBarSanitizesFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		// chars that must NOT appear anywhere in the rendered status bar
		forbidden []string
	}{
		{name: "newline", filename: "foo\nbar.go", forbidden: []string{"\n"}},
		{name: "carriage return", filename: "foo\rbar.go", forbidden: []string{"\r"}},
		{name: "tab", filename: "foo\tbar.go", forbidden: []string{"\t"}},
		{name: "esc", filename: "foo\x1b[31mevil\x1b[0m.go", forbidden: []string{"\x1b[31m", "\x1b[0m"}},
		{name: "rtl override", filename: "good\u202egp.os", forbidden: []string{"\u202e"}},
		{name: "bom", filename: "\ufeffbom.go", forbidden: []string{"\ufeff"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel([]string{tt.filename}, nil)
			m.tree = testNewFileTree([]string{tt.filename})
			m.file.name = tt.filename
			m.layout.focus = paneDiff
			status := m.statusBarText()
			for _, c := range tt.forbidden {
				assert.NotContains(t, status, c, "status bar must strip control/format byte %q", c)
			}
			// status bar must remain a single visual line
			assert.NotContains(t, status, "\n", "status bar must never contain a literal newline")
		})
	}
}

func TestModel_TruncateHeaderTitle(t *testing.T) {
	m := testModel(nil, nil)
	tests := []struct {
		name  string
		title string
		paneW int
		want  string
	}{
		{name: "fits exactly", title: "a.go", paneW: 5, want: " a.go"},
		{name: "fits with room", title: "a.go", paneW: 80, want: " a.go"},
		{name: "truncates from left", title: "very/long/path/to/file.go", paneW: 12, want: " …to/file.go"},
		{name: "wide chars truncate by display width", title: "テスト.go", paneW: 6, want: " ….go"},
		{name: "boundary paneW=2", title: "abc", paneW: 2, want: " …"},
		{name: "boundary paneW=3 keeps last char", title: "abc", paneW: 3, want: " …c"},
		{name: "boundary paneW=4 fits", title: "abc", paneW: 4, want: " abc"},
		{name: "extreme narrow paneW=1 returns single ellipsis", title: "anything", paneW: 1, want: "…"},
		{name: "extreme narrow paneW=0 returns empty", title: "anything", paneW: 0, want: ""},
		{name: "negative paneW returns empty", title: "anything", paneW: -5, want: ""},
		{name: "empty title fits", title: "", paneW: 10, want: " "},
		// codex iter-2 C1: control-character-bearing filenames must be sanitized
		// before width budgeting so they cannot re-wrap the diff header.
		{name: "newline in title stripped", title: "foo\nbar.go", paneW: 80, want: " foobar.go"},
		{name: "esc sequence in title stripped", title: "\x1b[31mevil\x1b[0m.go", paneW: 80, want: " [31mevil[0m.go"},
		{name: "tab in title stripped", title: "tab\there.go", paneW: 80, want: " tabhere.go"},
		{name: "long title with newline still truncates", title: "very/long/path/" + "\n" + "to/some/file.go", paneW: 12, want: " …me/file.go"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.truncateHeaderTitle(tt.title, tt.paneW)
			assert.Equal(t, tt.want, got)
			if tt.paneW > 0 {
				assert.LessOrEqual(t, lipgloss.Width(got), tt.paneW, "truncated header must fit in paneW")
			}
			// invariant the scrollbar relies on: result must be a single line
			assert.NotContains(t, got, "\n", "header must never contain newlines")
		})
	}
}

func TestModel_LineNumberSegment(t *testing.T) {
	t.Run("context line", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 10, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 11, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff
		assert.Equal(t, "L:10/11", m.lineNumberSegment())
	})

	t.Run("add line", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 5, NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 0, NewNum: 6, Content: "new", ChangeType: diff.ChangeAdd},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 1
		m.layout.focus = paneDiff
		assert.Equal(t, "L:6/6", m.lineNumberSegment())
	})

	t.Run("remove line uses old max", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 5, NewNum: 5, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 6, NewNum: 0, Content: "old", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 1
		m.layout.focus = paneDiff
		assert.Equal(t, "L:6/6", m.lineNumberSegment())
	})

	t.Run("remove line denominator differs from new max", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 9, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
			{OldNum: 12, NewNum: 0, Content: "removed2", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 1
		m.layout.focus = paneDiff
		// on removed line: denominator = maxOld (12), not maxNew (9)
		assert.Equal(t, "L:11/12", m.lineNumberSegment())
	})

	t.Run("context line uses new max not old", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 10, NewNum: 9, Content: "ctx", ChangeType: diff.ChangeContext},
			{OldNum: 11, NewNum: 0, Content: "removed", ChangeType: diff.ChangeRemove},
			{OldNum: 12, NewNum: 0, Content: "removed2", ChangeType: diff.ChangeRemove},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff
		// on context line: denominator = maxNew (9), not maxOld (12)
		assert.Equal(t, "L:9/9", m.lineNumberSegment())
	})

	t.Run("divider line returns empty", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
			{Content: "...", ChangeType: diff.ChangeDivider},
			{OldNum: 50, NewNum: 50, Content: "ctx2", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 1
		m.layout.focus = paneDiff
		assert.Empty(t, m.lineNumberSegment())
	})

	t.Run("empty diffLines returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.file.lines = nil
		m.nav.diffCursor = 0
		m.layout.focus = paneDiff
		assert.Empty(t, m.lineNumberSegment())
	})

	t.Run("tree focus returns empty", func(t *testing.T) {
		lines := []diff.DiffLine{
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		}
		m := testModel(nil, nil)
		m.file.lines = lines
		m.nav.diffCursor = 0
		m.layout.focus = paneTree
		assert.Empty(t, m.lineNumberSegment())
	})
}

func TestModel_PadContentBg(t *testing.T) {
	m := testModel(nil, nil)
	bg := style.Color("\033[48;2;46;52;64m") // pre-built ANSI bg sequence for #2e3440

	t.Run("empty bg is no-op", func(t *testing.T) {
		assert.Equal(t, "hello", m.padContentBg("hello", 20, ""))
	})

	t.Run("zero width is no-op", func(t *testing.T) {
		assert.Equal(t, "hello", m.padContentBg("hello", 0, bg))
	})

	t.Run("pads short line", func(t *testing.T) {
		result := m.padContentBg("hi", 10, bg)
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Contains(t, result, "\033[49m")
		assert.Equal(t, 10, lipgloss.Width(result))
	})

	t.Run("strips trailing spaces before padding", func(t *testing.T) {
		result := m.padContentBg("hi      ", 10, bg)
		assert.Contains(t, result, "\033[48;2;46;52;64m")
		assert.Equal(t, 10, lipgloss.Width(result))
	})

	t.Run("multi-line pads each line", func(t *testing.T) {
		result := m.padContentBg("ab\ncd", 5, bg)
		lines := strings.Split(result, "\n")
		assert.Len(t, lines, 2)
		assert.Equal(t, 5, lipgloss.Width(lines[0]))
		assert.Equal(t, 5, lipgloss.Width(lines[1]))
	})

	t.Run("line at target width is no-op", func(t *testing.T) {
		result := m.padContentBg("abcde", 5, bg)
		assert.Equal(t, "abcde", result)
	})
}

func TestModel_TreePaneToggle(t *testing.T) {
	lines := []diff.DiffLine{{ChangeType: diff.ChangeContext, Content: "x", NewNum: 1}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines, "b.go": lines})
	m.tree = testNewFileTree([]string{"a.go", "b.go"})
	m.file.name = "a.go"
	m.file.lines = lines
	m.layout.focus = paneTree
	m.layout.viewport = viewport.New(80, 30)
	origTreeWidth := m.layout.treeWidth

	t.Run("t hides tree pane", func(t *testing.T) {
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.True(t, model.layout.treeHidden)
		assert.Equal(t, 0, model.layout.treeWidth)
		assert.Equal(t, paneDiff, model.layout.focus, "focus should move to diff when hiding tree")
		assert.Equal(t, model.layout.width-2, model.layout.viewport.Width, "diff should use full width")
	})

	t.Run("t shows tree pane again", func(t *testing.T) {
		m2 := m
		m2.layout.treeHidden = true
		m2.layout.treeWidth = 0
		m2.layout.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.False(t, model.layout.treeHidden)
		assert.Equal(t, origTreeWidth, model.layout.treeWidth)
	})

	t.Run("tab is no-op when tree hidden", func(t *testing.T) {
		m2 := m
		m2.layout.treeHidden = true
		m2.layout.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "tab should not switch pane when tree hidden")
	})

	t.Run("h is no-op when tree hidden", func(t *testing.T) {
		m2 := m
		m2.layout.treeHidden = true
		m2.layout.focus = paneDiff
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.layout.focus, "h should not switch to tree when hidden")
	})

	t.Run("no-op in single-file mode without TOC", func(t *testing.T) {
		m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m2.file.singleFile = true
		m2.layout.viewport = viewport.New(80, 30)
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.False(t, model.layout.treeHidden, "t should be no-op in single-file mode without TOC")
	})

	t.Run("toggle works in single-file markdown with TOC", func(t *testing.T) {
		m2 := testModel([]string{"readme.md"}, map[string][]diff.DiffLine{"readme.md": lines})
		m2.file.singleFile = true
		m2.file.mdTOC = sidepane.ParseTOC([]diff.DiffLine{{NewNum: 1, Content: "# Header", ChangeType: diff.ChangeContext}}, "readme.md")
		m2.layout.treeWidth = max(minTreeWidth, m2.layout.width*m2.cfg.treeWidthRatio/10)
		m2.layout.viewport = viewport.New(80, 30)
		m2.layout.focus = paneTree
		result, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
		model := result.(Model)
		assert.True(t, model.layout.treeHidden, "t should hide TOC pane in single-file markdown mode")
		assert.Equal(t, 0, model.layout.treeWidth)
		assert.Equal(t, paneDiff, model.layout.focus)
	})

	t.Run("resize preserves hidden state", func(t *testing.T) {
		m2 := m
		m2.layout.treeHidden = true
		m2.layout.treeWidth = 0
		result, _ := m2.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		model := result.(Model)
		assert.True(t, model.layout.treeHidden)
		assert.Equal(t, 0, model.layout.treeWidth)
	})

	t.Run("status icon shows when hidden", func(t *testing.T) {
		m2 := m
		m2.layout.treeHidden = true
		icons := m2.statusModeIcons()
		assert.Contains(t, icons, "⊟")
	})
}

func TestModel_ToggleLineNumbers(t *testing.T) {
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{
		"a.go": {
			{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		},
	})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = []diff.DiffLine{{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext}}

	assert.False(t, m.modes.lineNumbers)
	result, _ := m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.True(t, m.modes.lineNumbers)
	result, _ = m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.False(t, m.modes.lineNumbers)
}

func TestModel_ComputeLineNumWidth(t *testing.T) {
	tests := []struct {
		name  string
		lines []diff.DiffLine
		want  int
	}{
		{name: "single digit", lines: []diff.DiffLine{
			{OldNum: 5, NewNum: 5, ChangeType: diff.ChangeContext},
		}, want: 1},
		{name: "two digits", lines: []diff.DiffLine{
			{OldNum: 99, NewNum: 99, ChangeType: diff.ChangeContext},
		}, want: 2},
		{name: "mixed old larger", lines: []diff.DiffLine{
			{OldNum: 100, NewNum: 5, ChangeType: diff.ChangeContext},
		}, want: 3},
		{name: "mixed new larger", lines: []diff.DiffLine{
			{OldNum: 5, NewNum: 1000, ChangeType: diff.ChangeContext},
		}, want: 4},
		{name: "empty", lines: nil, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.file.lines = tt.lines
			assert.Equal(t, tt.want, m.computeLineNumWidth())
		})
	}
}

func TestModel_LineNumbersEndToEnd(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 10, NewNum: 10, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 11, NewNum: 0, Content: "old", ChangeType: diff.ChangeRemove},
		{OldNum: 0, NewNum: 11, Content: "new", ChangeType: diff.ChangeAdd},
		{Content: "@@ -10,3 +10,3 @@", ChangeType: diff.ChangeDivider},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.layout.focus = paneDiff
	m.file.name = "a.go"
	m.file.lines = lines

	// toggle on
	result, _ := m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.True(t, m.modes.lineNumbers)
	assert.Equal(t, 2, m.file.lineNumWidth)

	rendered := m.renderDiff()
	stripped := ansi.Strip(rendered)
	assert.Contains(t, stripped, "10 10")
	assert.Contains(t, stripped, "11   ")
	assert.Contains(t, stripped, "   11")

	// toggle off
	result, _ = m.handleViewToggle(keymap.ActionToggleLineNums)
	m = result.(Model)
	assert.False(t, m.modes.lineNumbers)
	rendered = m.renderDiff()
	stripped = ansi.Strip(rendered)
	assert.NotContains(t, stripped, "10 10")
}
