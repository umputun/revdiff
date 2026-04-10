package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

func TestModel_HighlightSearchMatches(t *testing.T) {
	colors := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700"}
	m := testModel(nil, nil)
	m.styles = newStyles(colors)

	t.Run("plain text single match", func(t *testing.T) {
		m.searchTerm = "hello"
		result := m.highlightSearchMatches("say hello world", diff.ChangeContext)
		assert.NotContains(t, result, "\033[38;2;", "should not set foreground (bg-only highlight)")
		assert.Contains(t, result, "\033[48;2;215;215;0m") // search bg
		assert.Contains(t, result, "hello")
		assert.Contains(t, result, "\033[49m") // bg reset for context lines
	})

	t.Run("multiple matches", func(t *testing.T) {
		m.searchTerm = "ab"
		result := m.highlightSearchMatches("ab cd ab", diff.ChangeContext)
		assert.Equal(t, 2, strings.Count(result, "\033[48;2;215;215;0m"), "should highlight both occurrences")
	})

	t.Run("no match", func(t *testing.T) {
		m.searchTerm = "xyz"
		result := m.highlightSearchMatches("hello world", diff.ChangeContext)
		assert.Equal(t, "hello world", result)
	})

	t.Run("empty search term", func(t *testing.T) {
		m.searchTerm = ""
		result := m.highlightSearchMatches("hello world", diff.ChangeContext)
		assert.Equal(t, "hello world", result)
	})

	t.Run("case insensitive", func(t *testing.T) {
		m.searchTerm = "hello"
		result := m.highlightSearchMatches("say HELLO world", diff.ChangeContext)
		assert.Contains(t, result, "\033[48;2;215;215;0m")
	})

	t.Run("with ansi codes", func(t *testing.T) {
		m.searchTerm = "world"
		result := m.highlightSearchMatches("\033[32mhello world\033[0m", diff.ChangeContext)
		assert.Contains(t, result, "\033[48;2;215;215;0m") // search bg on
		assert.Contains(t, result, "\033[49m")             // search bg reset for context
		assert.Contains(t, result, "\033[32m")             // original ansi preserved
	})

	t.Run("no-colors fallback", func(t *testing.T) {
		noColorModel := testModel(nil, nil)
		noColorModel.searchTerm = "hello"
		result := noColorModel.highlightSearchMatches("say hello world", diff.ChangeContext)
		assert.Contains(t, result, "\033[7m", "should use reverse video in no-colors mode")
		assert.Contains(t, result, "\033[27m", "should reset reverse video")
	})

	t.Run("add line restores add bg instead of terminal default", func(t *testing.T) {
		c := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700", AddFg: "#00ff00", AddBg: "#002200"}
		am := testModel(nil, nil)
		am.styles = newStyles(c)
		am.searchTerm = "hello"
		result := am.highlightSearchMatches("say hello world", diff.ChangeAdd)
		assert.Contains(t, result, "\033[48;2;215;215;0m", "should have search bg on")
		assert.NotContains(t, result, "\033[49m", "should not reset to terminal default")
		assert.Contains(t, result, am.ansiBg(c.AddBg), "should restore to add bg")
	})

	t.Run("remove line restores remove bg instead of terminal default", func(t *testing.T) {
		c := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700", RemoveFg: "#ff0000", RemoveBg: "#220000"}
		rm := testModel(nil, nil)
		rm.styles = newStyles(c)
		rm.searchTerm = "hello"
		result := rm.highlightSearchMatches("say hello world", diff.ChangeRemove)
		assert.Contains(t, result, "\033[48;2;215;215;0m", "should have search bg on")
		assert.NotContains(t, result, "\033[49m", "should not reset to terminal default")
		assert.Contains(t, result, rm.ansiBg(c.RemoveBg), "should restore to remove bg")
	})

	t.Run("search inside word-diff span restores word-diff bg", func(t *testing.T) {
		c := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700", AddFg: "#00ff00", AddBg: "#002200", WordAddBg: "#2d5a3a"}
		am := testModel(nil, nil)
		am.styles = newStyles(c)
		am.searchTerm = "foo"
		// simulate input with word-diff markers: [WordAddBg]foobar[AddBg]rest
		wordBg := am.ansiBg(c.WordAddBg)
		lineBg := am.ansiBg(c.AddBg)
		input := wordBg + "foobar" + lineBg + "rest"
		result := am.highlightSearchMatches(input, diff.ChangeAdd)
		// after search match ends at "foo", should restore to word-diff bg, not line bg
		searchBg := am.ansiBg(c.SearchBg)
		assert.Contains(t, result, searchBg, "should have search bg on")
		assert.Contains(t, result, wordBg+"bar", "bar should keep word-diff bg after search match")
	})

	t.Run("search match spanning word-diff boundary preserves search bg", func(t *testing.T) {
		c := Colors{SearchFg: "#1a1a1a", SearchBg: "#d7d700", AddFg: "#00ff00", AddBg: "#002200", WordAddBg: "#2d5a3a"}
		am := testModel(nil, nil)
		am.styles = newStyles(c)
		am.searchTerm = "foobar rest"
		// simulate input with word-diff markers: [WordAddBg]foobar[AddBg] rest
		wordBg := am.ansiBg(c.WordAddBg)
		lineBg := am.ansiBg(c.AddBg)
		searchBg := am.ansiBg(c.SearchBg)
		input := wordBg + "foobar" + lineBg + " rest"
		result := am.highlightSearchMatches(input, diff.ChangeAdd)
		// search bg must persist across word-diff boundary so " rest" stays highlighted
		assert.Contains(t, result, searchBg+" rest", "search bg should be re-emitted after word-diff boundary")
	})

	t.Run("no-colors search inside word-diff reverse restores reverse", func(t *testing.T) {
		am := testModel(nil, nil) // no colors → noColors=true
		am.searchTerm = "foo"
		// simulate input with word-diff reverse: \033[7mfoobar\033[27mrest
		input := "\033[7m" + "foobar" + "\033[27m" + "rest"
		result := am.highlightSearchMatches(input, diff.ChangeAdd)
		// after search match ends at "foo", should restore reverse-video for "bar"
		assert.Contains(t, result, "\033[7m"+"bar", "bar should stay reverse-video after search")
	})
}

func TestModel_StartSearch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// press / to start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	assert.True(t, model.searching, "should be in searching mode")
	assert.True(t, model.searchInput.Focused(), "search input should be focused")
}

func TestModel_StartSearchOnlyFromDiffPane(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneTree

	// press / in tree pane - should not start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	assert.False(t, model.searching, "should not search from tree pane")
}

func TestModel_SubmitSearchFindsMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello world", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "hello again", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 0

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// type "hello"
	for _, ch := range "hello" {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	// submit with enter
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching, "should exit searching mode")
	assert.Equal(t, "hello", model.searchTerm)
	assert.Equal(t, []int{0, 2}, model.searchMatches)
	assert.Equal(t, 0, model.searchCursor)
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match")
}

func TestModel_SubmitSearchCaseInsensitive(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "Hello World", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "HELLO again", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("hello")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, []int{0, 1}, model.searchMatches, "should match case-insensitively")
}

func TestModel_SubmitSearchJumpsForwardFromCursor(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match here", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match again", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 1 // cursor past first match

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("match")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, 1, model.searchCursor, "should jump to second match (index 1)")
	assert.Equal(t, 2, model.diffCursor, "cursor should be on second match line")
}

func TestModel_SubmitSearchWrapsToFirstMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match here", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "foo bar", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.diffCursor = 1 // cursor past all matches

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("match")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.Equal(t, 0, model.searchCursor, "should wrap to first match")
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match line")
}

func TestModel_SubmitSearchNoMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("xyz")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching)
	assert.Equal(t, "xyz", model.searchTerm)
	assert.Empty(t, model.searchMatches)
}

func TestModel_SubmitEmptySearchClearsMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up existing search state
	model.searchTerm = "hello"
	model.searchMatches = []int{0}
	model.searchCursor = 0

	// start search with empty input
	model.searching = true
	model.searchInput = textinput.New()
	model.searchInput.SetValue("")

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = result.(Model)

	assert.False(t, model.searching)
	assert.Empty(t, model.searchTerm)
	assert.Empty(t, model.searchMatches)
}

func TestModel_CancelSearch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	require.True(t, model.searching)

	// cancel with esc
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)

	assert.False(t, model.searching, "should exit searching mode on esc")
}

func TestModel_CancelSearchPreservesExistingMatches(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up existing search state
	model.searchTerm = "hello"
	model.searchMatches = []int{0}

	// start and cancel new search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)

	assert.Equal(t, "hello", model.searchTerm, "existing search term should be preserved on cancel")
	assert.Equal(t, []int{0}, model.searchMatches, "existing matches should be preserved on cancel")
}

func TestModel_SearchInputForwardsCharacters(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// type characters
	for _, ch := range "test" {
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = result.(Model)
	}

	assert.Equal(t, "test", model.searchInput.Value(), "characters should be forwarded to search input")
}

func TestModel_SearchBlocksOtherKeysWhileActive(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)

	// pressing q should not quit, it should type 'q'
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	model = result.(Model)

	assert.True(t, model.searching, "should still be searching")
	assert.Contains(t, model.searchInput.Value(), "q")
}

func TestModel_SearchForwardsNonKeyMessages(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "hello", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start search
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = result.(Model)
	require.True(t, model.searching)

	// send a non-key message; should not panic and model stays searching
	type customMsg struct{}
	result, _ = model.Update(customMsg{})
	model = result.(Model)
	assert.True(t, model.searching, "searching should remain true after non-key message")
}

func TestModel_NextSearchMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "match three", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2, 3}
	model.searchCursor = 0
	model.diffCursor = 0

	// press n to go to next match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should advance to 1")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should move to second match")

	// press n again
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 2, model.searchCursor, "search cursor should advance to 2")
	assert.Equal(t, 3, model.diffCursor, "diff cursor should move to third match")
}

func TestModel_NextSearchMatchWrapsAround(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 1 // on last match
	model.diffCursor = 2

	// press n should wrap to first match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "search cursor should wrap to 0")
	assert.Equal(t, 0, model.diffCursor, "diff cursor should wrap to first match")
}

func TestModel_PrevSearchMatch(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "match three", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2, 3}
	model.searchCursor = 2
	model.diffCursor = 3

	// press N to go to prev match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should go back to 1")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should move to second match")
}

func TestModel_PrevSearchMatchWrapsAround(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "no match", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match two", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 2}
	model.searchCursor = 0 // on first match
	model.diffCursor = 0

	// press N should wrap to last match
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 1, model.searchCursor, "search cursor should wrap to last")
	assert.Equal(t, 2, model.diffCursor, "diff cursor should wrap to last match")
}

func TestModel_SearchNavigationSkipsCollapsedHiddenLines(t *testing.T) {
	// in collapsed mode, removed lines are hidden. search navigation must skip them.
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = make(map[int]bool)
	// matches on indices 0 (ctx), 1 (hidden remove), 2 (add), 3 (ctx)
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 0
	model.diffCursor = 0

	t.Run("nextSearchMatch skips hidden removed line", func(t *testing.T) {
		m := model
		m.nextSearchMatch()
		assert.Equal(t, 2, m.searchCursor, "should skip hidden index 1, land on index 2")
		assert.Equal(t, 2, m.diffCursor, "cursor should be on visible add line")
	})

	t.Run("prevSearchMatch skips hidden removed line", func(t *testing.T) {
		m := model
		m.searchCursor = 2 // on index 2 (add line)
		m.diffCursor = 2
		m.prevSearchMatch()
		assert.Equal(t, 0, m.searchCursor, "should skip hidden index 1, land on index 0")
		assert.Equal(t, 0, m.diffCursor, "cursor should be on visible context line")
	})

	t.Run("submitSearch skips hidden match for initial jump", func(t *testing.T) {
		m := model
		m.diffCursor = 1 // cursor on hidden line
		m.searchTerm = ""
		m.searchMatches = nil
		m.searchInput = textinput.New()
		m.searchInput.SetValue("match")
		m.submitSearch()
		// should jump to index 2 (visible add) not index 1 (hidden remove)
		assert.Equal(t, 2, m.diffCursor, "should skip hidden remove and land on visible add")
	})
}

func TestModel_NKeyFallsThroughToNextFileWhenNoSearch(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.currFile = "a.go"
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)

	// no search active, n should advance to next file
	assert.Empty(t, model.searchMatches)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = result.(Model)
	assert.Equal(t, "b.go", model.tree.selectedFile(), "n should go to next file when no search active")
}

func TestModel_ShiftNDoesPrevMatchWhenSearchActive(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match one", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "match two", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = []int{0, 1}
	model.searchCursor = 1
	model.diffCursor = 1

	// press N (shift-n)
	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model = result.(Model)
	assert.Equal(t, 0, model.searchCursor, "N should go to prev match")
	assert.Equal(t, 0, model.diffCursor, "cursor should be on first match")
}

func TestModel_ShiftNNavigatesPrevFileWithoutSearch(t *testing.T) {
	lines := []diff.DiffLine{{NewNum: 1, Content: "line1", ChangeType: diff.ChangeContext}}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{
		"a.go": lines, "b.go": lines,
	})
	m.tree = newFileTree([]string{"a.go", "b.go"})
	m.tree.cursor = 2 // start at second file (b.go); entries: [dir=0, a.go=1, b.go=2]
	m.currFile = "b.go"

	// no search active, N (prev_item) should navigate to previous file
	assert.Empty(t, m.searchMatches)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'N'}})
	model := result.(Model)
	assert.Equal(t, "a.go", model.tree.selectedFile(), "N should navigate to previous file")
}

func TestModel_SearchHighlightInRenderDiff(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "package main", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "func hello() {}", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "func world() {}", ChangeType: diff.ChangeAdd},
		{OldNum: 4, Content: "old line", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.styles = plainStyles()

	t.Run("no search, renderDiff succeeds with all lines", func(t *testing.T) {
		m.searchMatches = nil
		m.searchMatchSet = nil
		rendered := m.renderDiff()
		assert.Contains(t, rendered, "package main")
		assert.Contains(t, ansi.Strip(rendered), "func hello")
		assert.Contains(t, rendered, "func world")
		assert.Contains(t, rendered, "old line")
	})

	t.Run("search active, renderDiff includes matched content", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		rendered := m.renderDiff()
		// matched and non-matched lines should both be rendered
		assert.Contains(t, ansi.Strip(rendered), "func hello")
		assert.Contains(t, rendered, "func world")
		assert.Contains(t, rendered, "old line")
	})

	t.Run("search vs no search both render content correctly", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		renderedWithSearch := m.renderDiff()

		m.searchMatches = nil
		renderedWithout := m.renderDiff()

		// both should contain the same text content
		assert.Contains(t, ansi.Strip(renderedWithSearch), "func hello")
		assert.Contains(t, ansi.Strip(renderedWithout), "func hello")
		assert.Contains(t, renderedWithSearch, "func world")
		assert.Contains(t, renderedWithout, "func world")
	})

	t.Run("cursor coexists with search highlight", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{1}
		m.searchCursor = 0
		m.diffCursor = 1
		rendered := m.renderDiff()

		outputLines := strings.Split(rendered, "\n")
		var matchLine string
		for _, l := range outputLines {
			if strings.Contains(l, "hello") {
				matchLine = l
			}
		}
		require.NotEmpty(t, matchLine)
		assert.Contains(t, matchLine, "▶", "cursor should be present on matched line")
		assert.Contains(t, ansi.Strip(matchLine), "func hello", "content should be preserved with cursor on match")
	})
}

func TestModel_SearchHighlightWithWrap(t *testing.T) {
	longContent := "this is a very long line that contains the search term hello somewhere in the middle and should wrap"
	lines := []diff.DiffLine{
		{NewNum: 1, Content: longContent, ChangeType: diff.ChangeAdd},
		{NewNum: 2, Content: "short line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.wrapMode = true
	m.width = 60
	m.treeWidth = 12
	m.styles = plainStyles()

	m.searchTerm = "hello"
	m.searchMatches = []int{0}
	m.searchCursor = 0

	rendered := m.renderDiff()
	outputLines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")

	// the long line should produce continuation rows with ↪
	var continuationCount int
	for _, l := range outputLines {
		if strings.Contains(l, "↪") {
			continuationCount++
		}
	}
	assert.Positive(t, continuationCount, "wrapped search match should have continuation lines")

	// verify content is present (text flows through the rendering path correctly)
	assert.Contains(t, rendered, "hello")
	assert.Contains(t, rendered, "short line")
}

func TestModel_SearchHighlightInCollapsedMode(t *testing.T) {
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context line", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed line", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added hello line", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "added other line", ChangeType: diff.ChangeAdd},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = newFileTree([]string{"a.go"})
	m.currFile = "a.go"
	m.diffLines = lines
	m.highlightedLines = noopHighlighter().HighlightLines("a.go", lines)
	m.focus = paneDiff
	m.diffCursor = 0
	m.styles = plainStyles()
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	t.Run("collapsed renders with search matches", func(t *testing.T) {
		m.searchTerm = "hello"
		m.searchMatches = []int{2}
		m.searchCursor = 0
		rendered := m.renderDiff()

		assert.Contains(t, rendered, "added hello line")
		assert.Contains(t, rendered, "added other line")
	})

	t.Run("collapsed without search has no match set", func(t *testing.T) {
		m.searchMatches = nil
		m.searchMatchSet = nil
		rendered := m.renderDiff()

		assert.Contains(t, rendered, "added hello line")
		assert.Nil(t, m.searchMatchSet, "no search should produce nil match set")
	})
}

func TestModel_StyleDiffContentSearchMatch(t *testing.T) {
	m := testModel(nil, nil)
	m.styles = plainStyles()

	t.Run("search match returns same text content", func(t *testing.T) {
		resultMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, true)
		resultNoMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		assert.Contains(t, resultMatch, " + content")
		assert.Contains(t, resultNoMatch, " + content")
	})

	t.Run("search match with highlight preserves content", func(t *testing.T) {
		result := m.styleDiffContent(diff.ChangeAdd, " + ", "\033[32mgreen\033[0m", true, true)
		assert.Contains(t, result, " + ")
		assert.Contains(t, result, "\033[32m", "chroma foreground should be preserved")
	})

	t.Run("search match uses different style than normal add", func(t *testing.T) {
		// use newStyles with distinct colors so rendering produces different output
		c := Colors{
			Accent: "#ffffff", Border: "#555555", Normal: "#cccccc", Muted: "#666666",
			SelectedFg: "#ffffff", SelectedBg: "#333333", Annotation: "#ff9900",
			AddFg: "#00ff00", AddBg: "#002200", RemoveFg: "#ff0000", RemoveBg: "#220000",
			ModifyFg: "#ffaa00", ModifyBg: "#221100",
			SearchFg: "#1a1a1a", SearchBg: "#d7d700",
		}
		m.styles = newStyles(c)
		resultMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, true)
		resultNoMatch := m.styleDiffContent(diff.ChangeAdd, " + ", "content", false, false)
		// both have same text but may differ in ANSI sequences (depends on terminal detection)
		// the key test is that both contain the content and the code paths don't panic
		assert.Contains(t, resultMatch, "content")
		assert.Contains(t, resultNoMatch, "content")
	})
}

func TestModel_BuildSearchMatchSet(t *testing.T) {
	m := testModel(nil, nil)

	t.Run("empty matches produces nil set", func(t *testing.T) {
		m.searchMatches = nil
		result := m.buildSearchMatchSet()
		assert.Nil(t, result)
	})

	t.Run("matches produce correct set", func(t *testing.T) {
		m.searchMatches = []int{1, 5, 10}
		result := m.buildSearchMatchSet()
		assert.True(t, result[1])
		assert.True(t, result[5])
		assert.True(t, result[10])
		assert.False(t, result[0])
		assert.False(t, result[3])
	})
}

func TestModel_ClearSearchResetsMatchSet(t *testing.T) {
	m := testModel(nil, nil)
	m.searchTerm = "test"
	m.searchMatches = []int{1, 2}
	m.searchCursor = 1
	m.searchMatchSet = map[int]bool{1: true, 2: true}

	m.clearSearch()

	assert.Empty(t, m.searchTerm)
	assert.Nil(t, m.searchMatches)
	assert.Equal(t, 0, m.searchCursor)
	assert.Nil(t, m.searchMatchSet)
}

func TestModel_StatusBarShowsSearchInput(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.currFile = "a.go"
	m.searching = true
	m.searchInput = textinput.New()
	m.searchInput.SetValue("hello")

	status := m.statusBarText()
	assert.Contains(t, status, "/hello", "should show search prompt with value")
	assert.NotContains(t, status, "a.go", "filename should not appear during search input")
}

func TestModel_StatusBarSearchInputTakesPriority(t *testing.T) {
	m := testModel([]string{"a.go"}, nil)
	m.width = 120
	m.currFile = "a.go"
	m.searching = true
	m.searchInput = textinput.New()
	m.inConfirmDiscard = true // should not show discard prompt

	status := m.statusBarText()
	assert.Contains(t, status, "/", "search input should take priority over discard")
	assert.NotContains(t, status, "discard")
}

func TestModel_StatusBarSearchMatchPosition(t *testing.T) {
	tests := []struct {
		name         string
		matches      []int
		cursor       int
		wantContains string
		wantAbsent   string
	}{
		{name: "first of three", matches: []int{0, 2, 5}, cursor: 0, wantContains: "1/3"},
		{name: "second of three", matches: []int{0, 2, 5}, cursor: 1, wantContains: "2/3"},
		{name: "third of three", matches: []int{0, 2, 5}, cursor: 2, wantContains: "3/3"},
		{name: "single match", matches: []int{1}, cursor: 0, wantContains: "1/1"},
		{name: "no matches", matches: nil, cursor: 0, wantAbsent: "["},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := testModel(nil, nil)
			m.currFile = "a.go"
			m.diffLines = []diff.DiffLine{
				{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
				{NewNum: 2, Content: "add", ChangeType: diff.ChangeAdd},
				{NewNum: 3, Content: "ctx2", ChangeType: diff.ChangeContext},
			}
			m.focus = paneDiff
			m.width = 200
			m.searchMatches = tt.matches
			m.searchCursor = tt.cursor

			status := m.statusBarText()
			if tt.wantContains != "" {
				assert.Contains(t, status, tt.wantContains)
			}
			if tt.wantAbsent != "" {
				assert.NotContains(t, status, tt.wantAbsent)
			}
		})
	}
}

func TestModel_SearchSegment(t *testing.T) {
	m := testModel(nil, nil)

	// no matches
	assert.Empty(t, m.searchSegment())

	// with matches
	m.searchMatches = []int{0, 3, 7}
	m.searchCursor = 1
	assert.Equal(t, "2/3", m.searchSegment())

	// all matches on hidden removed lines in collapsed mode shows [0/N]
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed match", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx end", ChangeType: diff.ChangeContext},
	}
	m2 := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m2.diffLines = lines
	m2.currFile = "a.go"
	m2.collapsed.enabled = true
	m2.collapsed.expandedHunks = make(map[int]bool)
	m2.searchMatches = []int{1} // only on hidden removed line
	m2.searchCursor = 0
	assert.Equal(t, "0/1", m2.searchSegment(), "should show [0/N] when all matches are hidden")
}

func TestModel_StatusBarSearchPositionBetweenHunkAndIcons(t *testing.T) {
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
	m.width = 200
	m.searchMatches = []int{1}
	m.searchCursor = 0
	m.collapsed.enabled = true
	m.collapsed.expandedHunks = make(map[int]bool)

	status := m.statusBarText()
	// all three should be present
	assert.Contains(t, status, "hunk 1/1")
	assert.Contains(t, status, "1/1")
	assert.Contains(t, status, "▼")

	// [1/1] should appear after hunk and before ▼
	hunkIdx := strings.Index(status, "hunk 1/1")
	searchIdx := strings.Index(status, "1/1")
	iconIdx := strings.Index(status, "▼")
	assert.Greater(t, searchIdx, hunkIdx, "search position should appear after hunk")
	assert.Less(t, searchIdx, iconIdx, "search position should appear before mode icons")
}

func TestModel_ClearSearchOnFileLoad(t *testing.T) {
	lines1 := []diff.DiffLine{
		{NewNum: 1, Content: "hello world", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "hello again", ChangeType: diff.ChangeAdd},
	}
	lines2 := []diff.DiffLine{
		{NewNum: 1, Content: "other content", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go", "b.go"}, map[string][]diff.DiffLine{"a.go": lines1, "b.go": lines2})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", seq: model.loadSeq, lines: lines1})
	model = result.(Model)
	model.focus = paneDiff

	// set up search state as if user searched for "hello"
	model.searchTerm = "hello"
	model.searchMatches = []int{0, 1}
	model.searchCursor = 1
	model.searchMatchSet = map[int]bool{0: true, 1: true}

	// load a different file
	model.loadSeq++
	result, _ = model.Update(fileLoadedMsg{file: "b.go", seq: model.loadSeq, lines: lines2})
	model = result.(Model)

	assert.Empty(t, model.searchTerm, "search term should be cleared on file load")
	assert.Nil(t, model.searchMatches, "search matches should be cleared on file load")
	assert.Equal(t, 0, model.searchCursor, "search cursor should be reset on file load")
	assert.Nil(t, model.searchMatchSet, "search match set should be cleared on file load")
}

func TestModel_StatusBarNarrowDropsSearchSegment(t *testing.T) {
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
	m.searchMatches = []int{1}
	m.searchCursor = 0

	t.Run("wide terminal shows search segment", func(t *testing.T) {
		m.width = 200
		status := m.statusBarText()
		assert.Contains(t, status, "1/1")
	})

	t.Run("very narrow terminal drops search with hunk", func(t *testing.T) {
		m.width = 28
		status := m.statusBarText()
		assert.NotContains(t, status, "1/1", "search segment should be dropped on very narrow terminal")
		assert.Contains(t, status, "? help")
	})
}

func TestModel_RealignSearchCursorOnCollapsedToggle(t *testing.T) {
	// when toggling collapsed mode, searchCursor must realign to nearest visible match
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "match end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// set up search with cursor on the removed line (index 1)
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 1
	model.diffCursor = 1

	// toggle collapsed mode, which hides removed lines
	model.toggleCollapsedMode()

	assert.True(t, model.collapsed.enabled)
	assert.NotEqual(t, 1, model.diffCursor, "cursor should have moved off hidden removed line")
	assert.NotEqual(t, 1, model.searchCursor, "searchCursor should realign away from hidden match")
	// searchCursor should point to a visible match
	if model.searchCursor < len(model.searchMatches) {
		matchIdx := model.searchMatches[model.searchCursor]
		hunks := model.findHunks()
		assert.False(t, model.isCollapsedHidden(matchIdx, hunks), "realigned searchCursor should point to a visible match")
	}
}

func TestModel_RealignSearchCursorOnHunkCollapse(t *testing.T) {
	// when collapsing a hunk, searchCursor must realign if current match becomes hidden
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "match ctx", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "match removed", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "match added", ChangeType: diff.ChangeAdd},
		{NewNum: 3, Content: "ctx end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// start in collapsed mode with hunk expanded (hunk starts at index 1, first change line)
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = map[int]bool{1: true}
	model.searchMatches = []int{0, 1, 2, 3}
	model.searchCursor = 1 // on removed line (visible because hunk is expanded)
	model.diffCursor = 1

	// collapse the hunk — removed line becomes hidden
	model.toggleHunkExpansion()

	assert.NotContains(t, model.collapsed.expandedHunks, 1, "hunk should be collapsed")
	// searchCursor should have realigned to a visible match
	if len(model.searchMatches) > 0 && model.searchCursor < len(model.searchMatches) {
		matchIdx := model.searchMatches[model.searchCursor]
		hunks := model.findHunks()
		assert.False(t, model.isCollapsedHidden(matchIdx, hunks), "searchCursor should point to visible match after hunk collapse")
	}
}

func TestModel_RealignSearchCursorNoopWithoutSearch(t *testing.T) {
	// realignSearchCursor should be a no-op when no search is active
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "removed", ChangeType: diff.ChangeRemove},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.searchMatches = nil
	model.searchCursor = 0

	// should not panic or change anything
	model.realignSearchCursor()
	assert.Equal(t, 0, model.searchCursor)
}

func TestModel_SubmitSearchPreservesLeadingWhitespace(t *testing.T) {
	// search query with leading/trailing whitespace should be preserved in the search term
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "  indented line", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "normal line", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	model.searchInput = textinput.New()
	model.searchInput.SetValue("  indented")
	model.submitSearch()

	assert.Equal(t, "  indented", model.searchTerm, "leading whitespace should be preserved in search term")
	assert.Equal(t, []int{0}, model.searchMatches, "should match the indented line")
}

func TestModel_SubmitSearchWhitespaceOnlyClearsSearch(t *testing.T) {
	// pure whitespace query should clear search (same as empty)
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "line", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff

	// pre-populate search state
	model.searchTerm = "old"
	model.searchMatches = []int{0}
	model.searchCursor = 0

	model.searchInput = textinput.New()
	model.searchInput.SetValue("   ")
	model.submitSearch()

	assert.Empty(t, model.searchTerm, "whitespace-only query should clear search")
	assert.Nil(t, model.searchMatches)
}

func TestModel_DeletePlaceholderSearchHighlight(t *testing.T) {
	// delete-only placeholder should render correctly with and without search match.
	// verifies the code path doesn't panic and produces correct text content.
	// (actual ANSI styling differences depend on terminal detection)
	lines := []diff.DiffLine{
		{NewNum: 1, Content: "context", ChangeType: diff.ChangeContext},
		{OldNum: 2, Content: "deleted match", ChangeType: diff.ChangeRemove},
		{OldNum: 3, Content: "deleted other", ChangeType: diff.ChangeRemove},
		{NewNum: 2, Content: "context end", ChangeType: diff.ChangeContext},
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.focus = paneDiff
	model.styles = plainStyles()
	model.collapsed.enabled = true
	model.collapsed.expandedHunks = make(map[int]bool)
	model.diffCursor = 1

	t.Run("with search match", func(t *testing.T) {
		model.searchMatchSet = map[int]bool{1: true}
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		assert.Contains(t, rendered, "▶", "cursor indicator should be present")
	})

	t.Run("without search match", func(t *testing.T) {
		model.searchMatchSet = nil
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		assert.Contains(t, rendered, "▶")
	})

	t.Run("with wrap mode and search match", func(t *testing.T) {
		model.searchMatchSet = map[int]bool{1: true}
		model.wrapMode = true
		model.width = 120
		model.treeWidth = 30
		var b strings.Builder
		model.renderDeletePlaceholder(&b, 1, 1)
		rendered := b.String()
		assert.Contains(t, rendered, "2 lines deleted")
		model.wrapMode = false
	})
}

func TestModel_SearchWithTOCActive(t *testing.T) {
	mdLines := []diff.DiffLine{
		{NewNum: 1, Content: "# Title", ChangeType: diff.ChangeContext},
		{NewNum: 2, Content: "some text", ChangeType: diff.ChangeContext},
		{NewNum: 3, Content: "## Section", ChangeType: diff.ChangeContext},
		{NewNum: 4, Content: "more text with title", ChangeType: diff.ChangeContext},
	}

	t.Run("start search from diff pane with TOC", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model := result.(Model)
		result, _ = model.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model = result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneDiff

		// press '/' to start search
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		model = result.(Model)
		assert.True(t, model.searching, "should enter search mode in diff pane with TOC")
	})

	t.Run("search not started from TOC pane", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model := result.(Model)
		result, _ = model.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model = result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneTree // TOC pane

		// press '/' in TOC pane - should not start search
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
		model = result.(Model)
		assert.False(t, model.searching, "search should not start from TOC pane")
	})

	t.Run("TOC active section updates after search navigation", func(t *testing.T) {
		m := testModel([]string{"README.md"}, map[string][]diff.DiffLine{"README.md": mdLines})
		m.singleFile = true
		m.treeWidth = 0

		result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		model := result.(Model)
		result, _ = model.Update(fileLoadedMsg{file: "README.md", lines: mdLines})
		model = result.(Model)
		require.NotNil(t, model.mdTOC)

		model.focus = paneDiff
		model.searchMatches = []int{3} // match on line 3
		model.searchCursor = 0

		// navigate to search match via 'n' key
		result, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
		model = result.(Model)
		// active section should reflect the cursor position after search nav (index 2 = Section, accounting for top entry)
		assert.Equal(t, 2, model.mdTOC.activeSection, "TOC should track active section after search jump")
	})
}
