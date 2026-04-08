package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
)

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

func TestModel_ResizeInSingleFileMode(t *testing.T) {
	m := testModel(nil, nil)
	// set up single-file mode via filesLoadedMsg
	resized, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = resized.(Model)
	loaded, _ := m.Update(filesLoadedMsg{entries: []diff.FileEntry{{Path: "main.go"}}})
	m = loaded.(Model)
	require.True(t, m.singleFile)

	// resize while in single-file mode
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	model := result.(Model)

	assert.Equal(t, 0, model.treeWidth, "treeWidth stays 0 after resize in single-file mode")
	assert.Equal(t, 78, model.viewport.Width, "viewport width should be new width - 2")
}

func TestModel_StatusBarFilterIndicator(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}

	t.Run("filter icon shown when filter active", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.tree.filter = true
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "◉", "should show filter icon when filter active")
	})

	t.Run("filter icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

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
		colors := Colors{Muted: "#6c6c6c", StatusFg: "#202020"}
		m := testModel(nil, nil)
		m.styles = newStyles(colors)
		m.collapsed.enabled = true
		m.tree.filter = false
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
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.wrapMode = true
		m.width = 200

		status := m.statusBarText()
		assert.Contains(t, status, "↩", "should show wrap icon when wrap active")
	})

	t.Run("wrap icon always present", func(t *testing.T) {
		m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
		m.tree = newFileTree([]string{"a.go"})
		m.ready = true
		m.currFile = "a.go"
		m.diffLines = lines
		m.width = 200

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
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.fileAdds = 1
	m.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "a.go", "status bar should show filename")
	assert.Contains(t, status, "+1/-0", "status bar should show diff stats")
	assert.Contains(t, status, "? help", "status bar should show help hint")
}

func TestModel_StatusBarFilenameTruncation(t *testing.T) {
	longFile := "very/long/path/to/some/deeply/nested/file/in/the/project/structure.go"
	m := testModel(nil, nil)
	m.currFile = longFile
	m.fileAdds = 3
	m.fileRemoves = 1
	m.focus = paneDiff
	m.width = 40 // narrow terminal forces truncation

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
	m.currFile = wideFile
	m.fileAdds = 1
	m.fileRemoves = 0
	m.focus = paneDiff
	m.width = 42

	status := m.statusBarText()
	assert.Contains(t, status, "…", "should truncate wide-char filename with ellipsis")
	assert.Contains(t, status, "+1/-0", "should still show stats after truncation")
	assert.LessOrEqual(t, lipgloss.Width(status), m.width-2, "status text must fit within terminal width minus padding")
}

func TestModel_StatusBarModeIndicators(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff
	m.width = 200

	t.Run("both indicators when collapsed and filtered", func(t *testing.T) {
		m.collapsed.enabled = true
		m.collapsed.expandedHunks = make(map[int]bool)
		m.tree.filter = true
		status := m.statusBarText()
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
	})

	t.Run("indicators always present in default mode", func(t *testing.T) {
		m.collapsed.enabled = false
		m.tree.filter = false
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
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)
	m.tree.filter = true

	t.Run("wide terminal shows all segments", func(t *testing.T) {
		m.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "hunk 1/1")
		assert.Contains(t, status, "▼")
		assert.Contains(t, status, "◉")
		assert.Contains(t, status, "? help")
	})

	t.Run("narrow terminal drops hunk from left first", func(t *testing.T) {
		m.width = 50
		status := m.statusBarText()
		assert.Contains(t, status, "a.go")
		assert.Contains(t, status, "+1/-0")
		assert.Contains(t, status, "? help")
	})

	t.Run("very narrow terminal drops hunk info", func(t *testing.T) {
		m.width = 28
		status := m.statusBarText()
		assert.Contains(t, status, "? help")
		assert.NotContains(t, status, "hunk", "hunk should be dropped on very narrow terminal")
	})
}

func TestModel_StatusBarStatsDisplay(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "main.go"
	m.fileAdds = 10
	m.fileRemoves = 5
	m.width = 120

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
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.cursorOnAnnotation = true
	m.focus = paneDiff
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
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = "a.go"
	m.diffLines = lines

	m.focus = paneTree
	status := m.statusBarText()
	assert.Contains(t, status, "? help", "tree pane should show help hint")

	m.focus = paneDiff
	status = m.statusBarText()
	assert.Contains(t, status, "? help", "diff pane should show help hint")
}

func TestModel_StatusBarNoFilenameWithoutFile(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.currFile = ""
	m.focus = paneTree

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
	m.diffLines = lines
	m.diffCursor = 1
	m.currFile = "a.go"
	m.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/2")

	m.diffCursor = 3
	status = m.statusBarText()
	assert.Contains(t, status, "hunk 2/2")
}

func TestModel_StatusBarNoHunkIndicatorWithoutChanges(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "ctx2", ChangeType: diff.ChangeContext},
	}

	m := testModel(nil, nil)
	m.diffLines = lines
	m.diffCursor = 0
	m.currFile = "a.go"
	m.focus = paneDiff

	status := m.statusBarText()
	assert.NotContains(t, status, "hunk", "should not show hunk when cursor on context line")
}

func TestModel_StatusBarHunkOnlyInDiffPane(t *testing.T) {
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
	m.diffCursor = 0
	m.focus = paneDiff

	status := m.statusBarText()
	assert.Contains(t, status, "hunk 1/1")

	// tree pane should not show hunk
	m.focus = paneTree
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
		m.diffLines = lines
		m.diffCursor = 0
		m.currFile = "a.go"
		m.focus = paneDiff

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
		m.diffLines = lines
		m.diffCursor = 0
		m.currFile = "a.go"
		m.focus = paneDiff

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
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 0
	m.fileAdds = 0
	m.focus = paneDiff
	m.width = 200

	status := m.statusBarText()
	assert.Contains(t, status, "L:10/11", "status bar should show line number")
}

func TestModel_StatusBarLineNumberAfterHunk(t *testing.T) {
	lines := []diff.DiffLine{
		{OldNum: 1, NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 0, NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
	}
	m := testModel(nil, nil)
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff
	m.width = 200

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
	m.currFile = "a.go"
	m.diffLines = lines
	m.diffCursor = 1
	m.fileAdds = 1
	m.focus = paneDiff

	t.Run("wide terminal shows line number", func(t *testing.T) {
		m.width = 200
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
		m.currFile = "a.go"
		m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.fileAdds = 1
		m.diffCursor = 0
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "no full ANSI reset in separator")
	})

	t.Run("with colors", func(t *testing.T) {
		colors := Colors{Muted: "#6c6c6c", StatusFg: "#202020", StatusBg: "#C5794F", Normal: "#d0d0d0"}
		m := testModel(nil, nil)
		m.styles = newStyles(colors)
		m.currFile = "a.go"
		m.diffLines = []diff.DiffLine{{NewNum: 1, Content: "add", ChangeType: diff.ChangeAdd}}
		m.fileAdds = 1
		m.diffCursor = 0
		m.focus = paneDiff

		status := m.statusBarText()
		assert.Contains(t, status, "|", "separator pipe must be present")
		assert.NotContains(t, status, "\033[0m", "separator must use raw ANSI fg, not lipgloss Render")
		assert.Contains(t, status, "\033[38;2;108;108;108m", "separator should have muted fg ANSI sequence")
		assert.Contains(t, status, "\033[38;2;32;32;32m", "separator should restore status fg after pipe")
	})
}

func TestModel_ViewNoStatusBar(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.tree = newFileTree([]string{"a.go"})
	m.noStatusBar = true
	m.ready = true
	m.width = 120
	m.height = 40
	m.treeWidth = 24
	view := m.View()
	assert.NotContains(t, view, "quit", "status bar should be hidden")
	assert.Contains(t, view, "a.go", "tree content should still appear")
}

func TestModel_QKeyNoStatusBarSkipsConfirmation(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.noStatusBar = true
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
	m.width = 120

	t.Run("tree pane has no shortcut hints", func(t *testing.T) {
		m.focus = paneTree
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[j/k]")
		assert.Contains(t, status, "? help")
	})

	t.Run("diff pane has no shortcut hints", func(t *testing.T) {
		m.focus = paneDiff
		status := m.statusBarText()
		assert.NotContains(t, status, "[Q]")
		assert.NotContains(t, status, "[q]")
		assert.NotContains(t, status, "[enter/a]")
		assert.Contains(t, status, "? help")
	})
}

func TestModel_HelpOverlaySections(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify section headers are present
	assert.Contains(t, help, "Navigation")
	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "Annotations")
	assert.Contains(t, help, "View")
	assert.Contains(t, help, "Quit")
}

func TestModel_HelpOverlayKeyListings(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	// verify key listings are present (dynamic rendering uses display names)
	keys := []string{
		"Tab", "PgDn", "PgUp", "Ctrl+d", "Ctrl+u", "Home", "End",
		"j", "k", "n", "N", "h", "l",
		"/", "Enter", "A", "d", "@", "f", "v", "w", ".", "L", "t",
		"q", "Q", "?", "Esc",
	}
	for _, k := range keys {
		assert.Contains(t, help, k, "help overlay should contain key: %s", k)
	}
}

func TestModel_HelpOverlayInView(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	m.tree = newFileTree([]string{"a.go"})
	m.ready = true
	m.width = 100
	m.height = 80

	// without help, view should not contain help sections
	m.showHelp = false
	view := m.View()
	assert.NotContains(t, view, "Navigation")
	assert.NotContains(t, view, "Annotations")

	// with help, view should contain help sections overlaid on top of content
	m.showHelp = true
	view = m.View()
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Annotations")
	assert.Contains(t, view, "View")
	assert.Contains(t, view, "Quit")
	// overlay should preserve background content (tree pane visible on edges)
	assert.Contains(t, view, "a.go", "tree pane should be visible behind help overlay")
}

func TestModel_HelpOverlayContainsWordWrap(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()
	assert.Contains(t, help, "toggle word wrap")
	assert.Contains(t, help, "w")
}

func TestModel_HelpOverlayContainsSearchKeys(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	assert.Contains(t, help, "Search")
	assert.Contains(t, help, "search in diff")
	// n/N for next/prev search match is shown via File/Hunk section's "next file / search match"
	assert.Contains(t, help, "next file / search match")
	assert.Contains(t, help, "prev file / search match")
}

func TestModel_ViewSingleFileMode(t *testing.T) {
	t.Run("single-file mode renders full-width diff without tree pane", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.tree = newFileTree([]string{"main.go"})
		m.singleFile = true
		m.treeWidth = 0
		m.focus = paneDiff
		m.currFile = "main.go"
		m.noStatusBar = true
		m.ready = true

		view := m.View()
		assert.Contains(t, view, "main.go")

		// every rendered line must be full terminal width (diff pane uses m.width - 2 + 2 border = m.width)
		lines := strings.Split(view, "\n")
		for i, line := range lines {
			w := lipgloss.Width(line)
			if w == 0 {
				continue // skip empty trailing lines
			}
			assert.Equal(t, m.width, w, "line %d should be full width (%d), got %d", i, m.width, w)
		}

		// single-file mode must not contain adjacent pane borders (││) from JoinHorizontal
		stripped := ansi.Strip(view)
		assert.NotContains(t, stripped, "││", "single-file mode should not have two adjacent pane borders")
	})

	t.Run("multi-file mode renders tree and diff panes side by side", func(t *testing.T) {
		m := testModel([]string{"internal/a.go", "internal/b.go"}, nil)
		m.tree = newFileTree([]string{"internal/a.go", "internal/b.go"})
		m.singleFile = false
		m.focus = paneTree
		m.noStatusBar = true
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
		m.singleFile = true
		m.width = 100
		m.treeWidth = 0
		assert.Equal(t, 96, m.diffContentWidth()) // width - 4 (borders + cursor bar + right padding)
	})

	t.Run("multi-file mode", func(t *testing.T) {
		m := testModel([]string{"a.go", "b.go"}, nil)
		m.singleFile = false
		m.width = 120
		m.treeWidth = 36
		assert.Equal(t, 78, m.diffContentWidth()) // 120 - 36 - 4 - 2
	})

	t.Run("single-file mode minimum width", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.width = 5
		m.treeWidth = 0
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
		m.tree = newFileTree([]string{"main.go"})
		m.singleFile = true
		m.focus = paneDiff
		m.currFile = "main.go"
		m.diffLines = lines
		m.highlightedLines = noopHighlighter().HighlightLines("main.go", lines)
		m.styles = plainStyles()
		return m
	}

	t.Run("tab is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "tab should not switch pane in single-file mode")
	})

	t.Run("h is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "h should not switch to tree in single-file mode")
	})

	t.Run("f is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		m.store.Add(annotation.Annotation{File: "main.go", Line: 1, Type: "+", Comment: "test"})
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		model := result.(Model)
		assert.False(t, model.tree.filter, "f should not toggle filter in single-file mode")
	})

	t.Run("p is no-op in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.selectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.selectedFile(), "p should not change file in single-file mode")
	})

	t.Run("n is no-op for file nav in single-file mode", func(t *testing.T) {
		m := setup()
		selected := m.tree.selectedFile()
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, selected, model.tree.selectedFile(), "n should not advance file in single-file mode")
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
	model.singleFile = true
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 0
	model.diffCursor = 0

	// n should navigate to next search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "n should advance search cursor in single-file mode")
	assert.Equal(t, 2, model.diffCursor, "cursor should move to second match")

	// n should navigate to previous search match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "N should go back in single-file mode")
	assert.Equal(t, 0, model.diffCursor, "cursor should return to first match")
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
	model.singleFile = true
	model.focus = paneDiff

	assert.False(t, model.wrapMode, "wrap should be off initially")

	// toggle wrap on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.True(t, model.wrapMode, "w should toggle wrap on in single-file mode")
	assert.Equal(t, 0, model.scrollX, "wrap should reset horizontal scroll")

	// toggle wrap off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	model = result.(Model)
	assert.False(t, model.wrapMode, "w should toggle wrap off in single-file mode")
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
	model.singleFile = true
	model.focus = paneDiff

	assert.False(t, model.collapsed.enabled, "collapsed should be off initially")

	// toggle collapsed on
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.True(t, model.collapsed.enabled, "v should toggle collapsed on in single-file mode")

	// toggle collapsed off
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	model = result.(Model)
	assert.False(t, model.collapsed.enabled, "v should toggle collapsed off in single-file mode")
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

	assert.False(t, model.singleFile, "multi-file should not be in single-file mode")
	assert.Equal(t, paneTree, model.focus, "multi-file should start on tree pane")

	// tab should switch panes
	model.focus = paneTree
	model.currFile = "a.go"
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneDiff, model.focus, "tab should switch to diff pane in multi-file mode")

	// tab back to tree
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneTree, model.focus, "tab should switch back to tree in multi-file mode")

	// f should toggle filter (with annotations present)
	model.store.Add(annotation.Annotation{File: "a.go", Line: 1, Type: "+", Comment: "note"})
	model.focus = paneDiff
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = result.(Model)
	assert.True(t, model.tree.filter, "f should toggle filter in multi-file mode")
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
		assert.Len(t, model.mdTOC.entries, 3) // top + 2 headers
		assert.Equal(t, "README.md", model.mdTOC.entries[0].title)
		assert.Equal(t, "Title", model.mdTOC.entries[1].title)
		assert.Equal(t, "Section", model.mdTOC.entries[2].title)
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
}

func TestModel_ResizeWithTOCActive(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "## Section", ChangeType: diff.ChangeContext},
	}

	t.Run("resize preserves treeWidth when TOC active", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)

		result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		model := result.(Model)

		expectedTreeWidth := max(minTreeWidth, 100*model.treeWidthRatio/10)
		assert.Equal(t, expectedTreeWidth, model.treeWidth, "treeWidth should be ratio-based when TOC is active")
		assert.Equal(t, 100-expectedTreeWidth-4, model.viewport.Width, "viewport width accounts for TOC pane")
	})

	t.Run("resize sets treeWidth=0 when single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.mdTOC = nil

		result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		model := result.(Model)

		assert.Equal(t, 0, model.treeWidth, "treeWidth should be 0 for single-file without TOC")
		assert.Equal(t, 78, model.viewport.Width, "viewport width should be width - 2")
	})
}

func TestModel_DiffContentWidthWithTOC(t *testing.T) {
	m := testModel([]string{"README.md"}, nil)
	m.singleFile = true
	m.width = 100
	m.treeWidth = 30
	m.mdTOC = &mdTOC{entries: []tocEntry{{title: "Title", level: 1, lineIdx: 0}}, activeSection: -1}

	// with TOC active, should use multi-file formula: width - treeWidth - 4 - 2
	assert.Equal(t, 64, m.diffContentWidth()) // 100 - 30 - 4 - 2
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

func TestModel_ViewWithTOCPane(t *testing.T) {
	t.Run("markdown single-file with TOC renders two-pane layout", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.tree = newFileTree([]string{"README.md"})
		m.singleFile = true
		m.treeWidth = 25
		m.focus = paneDiff
		m.currFile = "README.md"
		m.noStatusBar = true
		m.ready = true
		m.mdTOC = &mdTOC{entries: []tocEntry{
			{title: "Title", level: 1, lineIdx: 0},
			{title: "Section", level: 2, lineIdx: 5},
		}, cursor: 0, activeSection: 0}

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
		m.tree = newFileTree([]string{"main.go"})
		m.singleFile = true
		m.treeWidth = 0
		m.focus = paneDiff
		m.currFile = "main.go"
		m.noStatusBar = true
		m.ready = true

		view := m.View()
		stripped := ansi.Strip(view)
		assert.Contains(t, stripped, "main.go")

		// single-file mode without TOC must not have adjacent pane borders
		assert.NotContains(t, stripped, "││", "single-file without TOC should not have two pane borders")
	})

	t.Run("TOC pane uses active style when focused", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.tree = newFileTree([]string{"README.md"})
		m.singleFile = true
		m.treeWidth = 25
		m.focus = paneTree // TOC pane focused
		m.currFile = "README.md"
		m.noStatusBar = true
		m.ready = true
		m.mdTOC = &mdTOC{entries: []tocEntry{
			{title: "Title", level: 1, lineIdx: 0},
		}, cursor: 0, activeSection: -1}

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
		m.singleFile = true
		m.width = 100
		m.treeWidth = 0
		assert.Equal(t, 96, m.diffContentWidth()) // width - 4
	})

	t.Run("single-file with TOC uses multi-file formula", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.singleFile = true
		m.width = 100
		m.treeWidth = 30
		m.mdTOC = &mdTOC{entries: []tocEntry{{title: "T", level: 1, lineIdx: 0}}, activeSection: -1}
		assert.Equal(t, 64, m.diffContentWidth()) // 100 - 30 - 4 - 2
	})

	t.Run("minimum width enforced", func(t *testing.T) {
		m := testModel([]string{"README.md"}, nil)
		m.singleFile = true
		m.width = 20
		m.treeWidth = 15
		m.mdTOC = &mdTOC{entries: []tocEntry{{title: "T", level: 1, lineIdx: 0}}, activeSection: -1}
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
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.focus = paneDiff

		// tab from diff -> TOC (paneTree)
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus)

		// tab from TOC -> diff
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		model = result.(Model)
		assert.Equal(t, paneDiff, model.focus)
	})

	t.Run("tab no-op in single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.mdTOC = nil
		m.currFile = "main.go"
		m.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "tab should be no-op without TOC in single-file mode")
	})
}

func TestModel_HKeySwitchesToTOC(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "## Section", ChangeType: diff.ChangeContext},
	}

	t.Run("h key in diff pane switches to TOC", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.diffLines = mdLines
		m.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus, "h key should switch to TOC pane")
	})

	t.Run("h key no-op in single-file without TOC", func(t *testing.T) {
		m := testModel([]string{"main.go"}, nil)
		m.singleFile = true
		m.mdTOC = nil
		m.currFile = "main.go"
		m.focus = paneDiff

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus, "h key should be no-op without TOC")
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
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.diffLines = mdLines
		m.focus = paneTree
		return m
	}

	// TOC entries: [0]=⌂ top (lineIdx=0), [1]=First (lineIdx=0), [2]=Second (lineIdx=2), [3]=Third (lineIdx=4)

	t.Run("j moves cursor down in TOC and auto-jumps diff", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, m.mdTOC.cursor) // starts on "top" entry

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 1, model.mdTOC.cursor)
		assert.Equal(t, 0, model.diffCursor, "diff cursor should jump to # First at index 0")
		assert.Equal(t, paneTree, model.focus, "focus should stay on TOC pane")
	})

	t.Run("k moves cursor up in TOC and auto-jumps diff", func(t *testing.T) {
		m := setup(t)
		m.mdTOC.cursor = 3 // on "Third"

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 2, model.mdTOC.cursor) // on "Second"
		assert.Equal(t, 2, model.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneTree, model.focus, "focus should stay on TOC pane")
	})

	t.Run("j clamped at last entry", func(t *testing.T) {
		m := setup(t)
		m.mdTOC.cursor = 3 // last entry (Third)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model := result.(Model)
		assert.Equal(t, 3, model.mdTOC.cursor)
	})

	t.Run("k clamped at first entry", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, m.mdTOC.cursor)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
		model := result.(Model)
		assert.Equal(t, 0, model.mdTOC.cursor)
	})

	t.Run("home moves to first entry", func(t *testing.T) {
		m := setup(t)
		m.mdTOC.cursor = 3

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
		model := result.(Model)
		assert.Equal(t, 0, model.mdTOC.cursor)
	})

	t.Run("end moves to last entry", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, m.mdTOC.cursor)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
		model := result.(Model)
		assert.Equal(t, 3, model.mdTOC.cursor)
	})

	t.Run("l switches to diff pane", func(t *testing.T) {
		m := setup(t)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		model := result.(Model)
		assert.Equal(t, paneDiff, model.focus)
	})

	t.Run("pgdn moves cursor by page size", func(t *testing.T) {
		m := setup(t)
		assert.Equal(t, 0, m.mdTOC.cursor)

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		model := result.(Model)
		assert.Equal(t, 3, model.mdTOC.cursor, "pgdn should move to last entry (4 entries including top)")
	})

	t.Run("pgup moves cursor by page size", func(t *testing.T) {
		m := setup(t)
		m.mdTOC.cursor = 3

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
		model := result.(Model)
		assert.Equal(t, 0, model.mdTOC.cursor, "pgup should move to first entry")
	})

	t.Run("tab back to TOC syncs cursor to active section", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 0        // cursor was on top entry
		m.mdTOC.activeSection = 3 // but diff scrolled to Third section
		m.diffCursor = 4          // cursor on Third header line

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus)
		assert.Equal(t, 3, model.mdTOC.cursor, "TOC cursor should sync to active section on tab back")
	})

	t.Run("h key syncs TOC cursor to active section", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 0
		m.mdTOC.activeSection = 2 // second section

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
		model := result.(Model)
		assert.Equal(t, paneTree, model.focus)
		assert.Equal(t, 2, model.mdTOC.cursor, "TOC cursor should sync to active section on h key")
	})

	t.Run("n jumps to next TOC entry from diff pane", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 1 // on First
		m.mdTOC.activeSection = 1

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, 2, model.mdTOC.cursor, "n should advance TOC cursor to Second")
		assert.Equal(t, 2, model.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneDiff, model.focus, "focus should stay on diff pane")
	})

	t.Run("p jumps to prev TOC entry from diff pane", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 3 // on Third
		m.mdTOC.activeSection = 3

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, 2, model.mdTOC.cursor, "p should move TOC cursor to Second")
		assert.Equal(t, 2, model.diffCursor, "diff cursor should jump to ## Second at index 2")
		assert.Equal(t, paneDiff, model.focus, "focus should stay on diff pane")
	})

	t.Run("n clamped at last TOC entry", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 3 // last entry
		m.mdTOC.activeSection = 3

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model := result.(Model)
		assert.Equal(t, 3, model.mdTOC.cursor)
	})

	t.Run("p clamped at first TOC entry", func(t *testing.T) {
		m := setup(t)
		m.focus = paneDiff
		m.mdTOC.cursor = 0
		m.mdTOC.activeSection = 0

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
		model := result.(Model)
		assert.Equal(t, 0, model.mdTOC.cursor)
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
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.diffLines = mdLines
		m.highlightedLines = make([]string, len(mdLines))
		m.focus = paneTree

		// move cursor to third entry (## Second at lineIdx=3), accounting for top entry at [0]
		m.mdTOC.cursor = 2
		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)

		assert.Equal(t, 3, model.diffCursor, "diffCursor should jump to Second header at index 3")
		assert.Equal(t, paneDiff, model.focus, "focus should switch to diff pane after enter")
		assert.Equal(t, 2, model.mdTOC.activeSection, "active section should track jumped entry")
	})

	t.Run("enter on last TOC entry", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.mdTOC = parseTOC(mdLines, "README.md")
		require.NotNil(t, m.mdTOC)
		m.currFile = "README.md"
		m.diffLines = mdLines
		m.highlightedLines = make([]string, len(mdLines))
		m.focus = paneTree
		m.mdTOC.cursor = 3 // ### Third at lineIdx=5, accounting for top entry at [0]

		result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		model := result.(Model)

		assert.Equal(t, 5, model.diffCursor, "diffCursor should jump to Third header at index 5")
		assert.Equal(t, paneDiff, model.focus)
	})
}

func TestModel_HelpOverlayContainsTOCSection(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	help := m.helpOverlay()

	assert.Contains(t, help, "Markdown TOC")
	assert.Contains(t, help, "switch between TOC and diff")
	assert.Contains(t, help, "navigate TOC entries")
	assert.Contains(t, help, "jump to header in diff")
}

func TestModel_MarkdownNoHeadersFallback(t *testing.T) {
	noHeaders := []diff.DiffLine{
		{NewNum: 1, Content: "just text", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "more text", ChangeType: diff.ChangeContext},
	}

	m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": noHeaders})
	m.singleFile = true
	m.treeWidth = 0
	m.focus = paneDiff

	result, _ := m.Update(fileLoadedMsg{file: "README.md", lines: noHeaders})
	model := result.(Model)

	assert.Nil(t, model.mdTOC, "mdTOC should be nil when no headers")
	assert.Equal(t, 0, model.treeWidth, "treeWidth should be 0 in fallback mode")

	// tab should be no-op in single-file mode without TOC
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = result.(Model)
	assert.Equal(t, paneDiff, model.focus, "tab should be no-op without TOC")
}

func TestModel_StatusModeIconsLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.lineNumbers = true
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "#")
}

func TestModel_StatusModeIconsBlame(t *testing.T) {
	m := testModel(nil, nil)
	m.showBlame = true
	icons := m.statusModeIcons()
	assert.Contains(t, icons, "b")
	assert.NotContains(t, icons, "@")
}

func TestModel_HelpOverlayContainsLineNumbers(t *testing.T) {
	m := testModel(nil, nil)
	m.width = 120
	m.height = 40
	help := m.helpOverlay()
	assert.Contains(t, help, "L")
	assert.Contains(t, help, "line numbers")
}

func TestModel_HelpOverlayCustomBinding(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	m.keymap.Bind("x", keymap.ActionQuit)
	help := m.helpOverlay()

	// custom binding should appear alongside default
	assert.Contains(t, help, "x")
	assert.Contains(t, help, "q")
	assert.Contains(t, help, "quit")
}

func TestModel_HelpOverlayUnmappedAction(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.styles = plainStyles()
	// unbind all keys for search action
	m.keymap.Unbind("/")
	help := m.helpOverlay()

	// search section should still exist but "search in diff" description should be gone
	assert.NotContains(t, help, "search in diff")
}

func TestModel_ReviewedStatusBar(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go", "c.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines, "c.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go", "c.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.width = 200

	// no reviewed count when nothing reviewed
	status := m.statusBarText()
	assert.NotContains(t, status, "✓ 0")

	// mark one file as reviewed
	m.tree.toggleReviewed("a.go")
	status = m.statusBarText()
	assert.Contains(t, status, "✓ 1/3", "status bar should show reviewed progress")

	// mark another
	m.tree.toggleReviewed("b.go")
	status = m.statusBarText()
	assert.Contains(t, status, "✓ 2/3")
}

func TestModel_ReviewedModeIcon(t *testing.T) {
	m := testModel(nil, nil)
	m.tree = newFileTree([]string{"a.go"})

	icons := m.statusModeIcons()
	assert.Contains(t, icons, "✓", "reviewed icon should always be present")

	// when no files reviewed, icon should use muted color
	m.tree.toggleReviewed("a.go")
	icons = m.statusModeIcons()
	assert.Contains(t, icons, "✓", "reviewed icon should be present when files reviewed")
}
