package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
)

// keyMsg builds a bubbletea KeyMsg for a single rune. Matches the style of
// existing tests that construct KeyRunes messages inline.
func keyMsg(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// vimTestModel builds a loaded Model with n context lines suitable for
// exercising the vim-motion interceptor. Viewport is pre-sized so cursor
// motion does not require a WindowSizeMsg.
func vimTestModel(t *testing.T, n int) Model {
	t.Helper()
	lines := make([]diff.DiffLine, n)
	for i := range lines {
		lines[i] = diff.DiffLine{NewNum: i + 1, Content: "line", ChangeType: diff.ChangeContext}
	}
	m := testModel([]string{"a.go"}, map[string][]diff.DiffLine{"a.go": lines})
	m.tree = testNewFileTree([]string{"a.go"})
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := result.(Model)
	result, _ = model.Update(fileLoadedMsg{file: "a.go", lines: lines})
	model = result.(Model)
	model.layout.focus = paneDiff
	model.modes.vimMotion = true
	model.nav.diffCursor = 0
	return model
}

func TestIsDigit(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"0", true}, {"5", true}, {"9", true},
		{"", false}, {"a", false}, {"10", false}, {"esc", false}, {" ", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, isDigit(tc.in))
		})
	}
}

func TestDigitValue(t *testing.T) {
	for i := range 10 {
		s := string(rune('0' + i))
		assert.Equal(t, i, digitValue(s), "digitValue(%q)", s)
	}
}

func TestVimChordTable_Bindings(t *testing.T) {
	// lock in the full set of chords so changes to the table surface in tests.
	expected := map[string]keymap.Action{
		"gg": keymap.ActionHome,
		"zz": keymap.ActionScrollCenter,
		"zt": keymap.ActionScrollTop,
		"zb": keymap.ActionScrollBottom,
		"ZZ": keymap.ActionQuit,
		"ZQ": keymap.ActionDiscardQuit,
	}
	assert.Equal(t, expected, vimChordTable)
}

func TestInterceptVimMotion_LeaderPendingBoundSecond(t *testing.T) {
	m := vimTestModel(t, 50)
	m.nav.diffCursor = 10
	m.vim.leader = "g"
	m.vim.hint = "g…"

	result, _, handled := m.interceptVimMotion(keyMsg('g'))
	require.True(t, handled, "bound second key should be handled")
	model := result.(Model)
	assert.Empty(t, model.vim.leader, "leader must be cleared")
	assert.Empty(t, model.vim.hint, "hint must be cleared on bound dispatch")
	assert.Equal(t, 0, model.nav.diffCursor, "gg should jump to home")
}

func TestInterceptVimMotion_LeaderPendingPropagatesCmd(t *testing.T) {
	// ZZ dispatches ActionQuit through resolveVimLeader -> dispatchAction,
	// which returns a tea.Quit command. The interceptor must surface that cmd
	// to its caller so handleKey can forward it to bubbletea.
	m := vimTestModel(t, 50)
	m.vim.leader = "Z"
	m.vim.hint = "Z…"

	_, cmd, handled := m.interceptVimMotion(keyMsg('Z'))
	require.True(t, handled)
	require.NotNil(t, cmd, "ZZ must surface a quit command through the interceptor")
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "surfaced cmd must produce tea.QuitMsg")
}

func TestInterceptVimMotion_LeaderPendingUnboundSecond(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.leader = "g"
	m.vim.hint = "g…"

	result, _, handled := m.interceptVimMotion(keyMsg('x'))
	require.True(t, handled, "unbound second key must still be consumed (no fall-through)")
	model := result.(Model)
	assert.Empty(t, model.vim.leader, "leader must be cleared on miss")
	assert.Equal(t, "Unknown: gx", model.vim.hint, "unknown chord must surface hint")
}

func TestInterceptVimMotion_LeaderPendingEsc(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.leader = "g"
	m.vim.hint = "g…"

	escMsg := tea.KeyMsg{Type: tea.KeyEsc}
	result, _, handled := m.interceptVimMotion(escMsg)
	require.True(t, handled, "esc during pending leader must be consumed silently")
	model := result.(Model)
	assert.Empty(t, model.vim.leader, "leader must be cleared by esc")
	assert.Empty(t, model.vim.hint, "hint must be cleared (no 'Unknown' message)")
}

func TestInterceptVimMotion_DigitZeroFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	// count == 0 and key == "0" must fall through so "0" can keep its normal binding.
	result, _, handled := m.interceptVimMotion(keyMsg('0'))
	require.False(t, handled, "bare 0 with no pending count must fall through")
	model := result.(Model)
	assert.Equal(t, 0, model.vim.count, "count must stay 0")
	assert.Empty(t, model.vim.hint, "hint must stay empty")
}

func TestInterceptVimMotion_DigitAccumulation(t *testing.T) {
	m := vimTestModel(t, 50)

	result, _, handled := m.interceptVimMotion(keyMsg('5'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, 5, model.vim.count)
	assert.Equal(t, "5", model.vim.hint)

	// subsequent digit compounds into count
	result, _, handled = model.interceptVimMotion(keyMsg('3'))
	require.True(t, handled)
	model = result.(Model)
	assert.Equal(t, 53, model.vim.count)
	assert.Equal(t, "53", model.vim.hint)

	// 0 after a non-zero count accumulates (no fall-through)
	result, _, handled = model.interceptVimMotion(keyMsg('0'))
	require.True(t, handled)
	model = result.(Model)
	assert.Equal(t, 530, model.vim.count)
	assert.Equal(t, "530", model.vim.hint)
}

func TestInterceptVimMotion_DigitClampsAt9999(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.count = 9999

	result, _, handled := m.interceptVimMotion(keyMsg('9'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, maxVimCount, model.vim.count, "count must clamp at maxVimCount")
	assert.Equal(t, "9999", model.vim.hint)
}

func TestInterceptVimMotion_CountJDiffPane(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.count = 5
	m.nav.diffCursor = 0

	result, _, handled := m.interceptVimMotion(keyMsg('j'))
	require.True(t, handled, "count + j in diff pane must be consumed")
	model := result.(Model)
	assert.Equal(t, 5, model.nav.diffCursor, "cursor must advance by count")
	assert.Equal(t, 0, model.vim.count, "count must be cleared")
	assert.Empty(t, model.vim.hint)
}

func TestInterceptVimMotion_CountKDiffPane(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.count = 3
	m.nav.diffCursor = 10

	result, _, handled := m.interceptVimMotion(keyMsg('k'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, 7, model.nav.diffCursor, "cursor must retreat by count")
	assert.Equal(t, 0, model.vim.count)
}

func TestInterceptVimMotion_CountJFocusTreeFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	m.layout.focus = paneTree
	m.vim.count = 5

	result, _, handled := m.interceptVimMotion(keyMsg('j'))
	require.False(t, handled, "j outside diff pane must fall through")
	model := result.(Model)
	assert.Equal(t, 0, model.vim.count, "count must be cleared on fall-through")
	assert.Empty(t, model.vim.hint)
}

func TestInterceptVimMotion_GWithCount(t *testing.T) {
	m := vimTestModel(t, 100)
	m.vim.count = 5
	m.nav.diffCursor = 0

	result, _, handled := m.interceptVimMotion(keyMsg('G'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, 4, model.nav.diffCursor, "5G jumps to line 5 (0-indexed: 4)")
	assert.Equal(t, 0, model.vim.count)
}

func TestInterceptVimMotion_GBareGoesToLastLine(t *testing.T) {
	m := vimTestModel(t, 50)
	m.nav.diffCursor = 0

	result, _, handled := m.interceptVimMotion(keyMsg('G'))
	require.True(t, handled, "bare G in diff pane must be consumed")
	model := result.(Model)
	assert.Equal(t, 49, model.nav.diffCursor, "bare G must jump to last line")
}

func TestInterceptVimMotion_GTreePaneFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	m.layout.focus = paneTree

	result, _, handled := m.interceptVimMotion(keyMsg('G'))
	require.False(t, handled, "G in tree pane must fall through (diff-only)")
	model := result.(Model)
	assert.Equal(t, 0, model.vim.count)
}

func TestInterceptVimMotion_CountUnrelatedKeyFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.count = 5

	// "q" with a pending count must drop the count silently and fall through
	// so the key still performs its normal action (ActionQuit).
	result, _, handled := m.interceptVimMotion(keyMsg('q'))
	require.False(t, handled, "unrelated key after count must fall through")
	model := result.(Model)
	assert.Equal(t, 0, model.vim.count, "count must be cleared silently")
	assert.Empty(t, model.vim.hint)
}

func TestInterceptVimMotion_LeaderEntryG(t *testing.T) {
	m := vimTestModel(t, 50)

	result, _, handled := m.interceptVimMotion(keyMsg('g'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, "g", model.vim.leader)
	assert.Equal(t, "g…", model.vim.hint)
}

func TestInterceptVimMotion_LeaderEntryGTreeFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	m.layout.focus = paneTree

	result, _, handled := m.interceptVimMotion(keyMsg('g'))
	require.False(t, handled, "g in tree pane must fall through")
	model := result.(Model)
	assert.Empty(t, model.vim.leader)
}

func TestInterceptVimMotion_LeaderEntryZ(t *testing.T) {
	m := vimTestModel(t, 50)

	result, _, handled := m.interceptVimMotion(keyMsg('z'))
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, "z", model.vim.leader)
	assert.Equal(t, "z…", model.vim.hint)
}

func TestInterceptVimMotion_LeaderEntryZTreeFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)
	m.layout.focus = paneTree

	result, _, handled := m.interceptVimMotion(keyMsg('z'))
	require.False(t, handled, "z in tree pane must fall through")
	model := result.(Model)
	assert.Empty(t, model.vim.leader)
}

func TestInterceptVimMotion_LeaderEntryCapitalZPaneAgnostic(t *testing.T) {
	// Z (for ZZ/ZQ quit aliases) must activate in any pane
	panes := []pane{paneDiff, paneTree}
	for _, p := range panes {
		t.Run(paneName(p), func(t *testing.T) {
			m := vimTestModel(t, 50)
			m.layout.focus = p

			result, _, handled := m.interceptVimMotion(keyMsg('Z'))
			require.True(t, handled, "Z must activate in any pane")
			model := result.(Model)
			assert.Equal(t, "Z", model.vim.leader)
			assert.Equal(t, "Z…", model.vim.hint)
		})
	}
}

func TestInterceptVimMotion_NonVimKeyFallsThrough(t *testing.T) {
	m := vimTestModel(t, 50)

	result, _, handled := m.interceptVimMotion(keyMsg('q'))
	require.False(t, handled, "non-vim key with no pending state must fall through")
	model := result.(Model)
	assert.Equal(t, 0, model.vim.count)
	assert.Empty(t, model.vim.leader)
	assert.Empty(t, model.vim.hint)
}

func TestResolveVimLeader_AllChordTableEntries(t *testing.T) {
	// for each chord, set the leader and resolve the second key. verify the
	// leader is cleared and the returned model reflects the dispatched action.
	tests := []struct {
		name   string
		leader string
		second string
		check  func(t *testing.T, m Model, cmd tea.Cmd)
	}{
		{
			name: "gg -> home", leader: "g", second: "g",
			check: func(t *testing.T, m Model, cmd tea.Cmd) {
				assert.Equal(t, 0, m.nav.diffCursor, "gg must jump to home")
			},
		},
		{
			name: "zz -> center", leader: "z", second: "z",
			check: func(t *testing.T, m Model, cmd tea.Cmd) {
				// centerViewportOnCursor reshapes viewport offset; we only verify no error
				// and the leader was cleared (checked below).
				_ = m
			},
		},
		{
			name: "zt -> top", leader: "z", second: "t",
			check: func(t *testing.T, m Model, cmd tea.Cmd) { _ = m },
		},
		{
			name: "zb -> bottom", leader: "z", second: "b",
			check: func(t *testing.T, m Model, cmd tea.Cmd) { _ = m },
		},
		{
			name: "ZZ -> quit", leader: "Z", second: "Z",
			check: func(t *testing.T, m Model, cmd tea.Cmd) {
				require.NotNil(t, cmd, "ZZ must produce a quit command")
				msg := cmd()
				_, ok := msg.(tea.QuitMsg)
				assert.True(t, ok, "ZZ must emit tea.QuitMsg")
			},
		},
		{
			name: "ZQ -> discard_quit", leader: "Z", second: "Q",
			check: func(t *testing.T, m Model, cmd tea.Cmd) {
				assert.True(t, m.discarded, "ZQ must set discarded flag")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := vimTestModel(t, 50)
			m.nav.diffCursor = 20
			m.vim.leader = tc.leader
			m.vim.hint = tc.leader + "…"

			result, cmd, handled := m.resolveVimLeader(tc.second)
			require.True(t, handled, "chord resolution must always be handled")
			model := result.(Model)
			assert.Empty(t, model.vim.leader, "leader must be cleared")
			tc.check(t, model, cmd)
		})
	}
}

func TestResolveVimLeader_UnknownChordSetsHint(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.leader = "z"
	m.vim.hint = "z…"

	result, _, handled := m.resolveVimLeader("x")
	require.True(t, handled)
	model := result.(Model)
	assert.Empty(t, model.vim.leader)
	assert.Equal(t, "Unknown: zx", model.vim.hint)
}

func TestResolveVimLeader_EscCancelsSilently(t *testing.T) {
	m := vimTestModel(t, 50)
	m.vim.leader = "g"
	m.vim.hint = "g…"

	result, _, handled := m.resolveVimLeader("esc")
	require.True(t, handled)
	model := result.(Model)
	assert.Empty(t, model.vim.leader)
	assert.Empty(t, model.vim.hint, "esc must not leave any hint")
}

func TestRepeatDiffAction_MultipleIterations(t *testing.T) {
	m := vimTestModel(t, 50)
	m.nav.diffCursor = 0

	result, _, handled := m.repeatDiffAction(keymap.ActionDown, 5)
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, 5, model.nav.diffCursor)
	assert.Equal(t, 0, model.vim.count, "count must be cleared after repeat")
	assert.Empty(t, model.vim.hint)
}

func TestRepeatDiffAction_ClampsAtBoundary(t *testing.T) {
	m := vimTestModel(t, 20)
	m.nav.diffCursor = 15

	result, _, handled := m.repeatDiffAction(keymap.ActionDown, 9999)
	require.True(t, handled)
	model := result.(Model)
	assert.Equal(t, 19, model.nav.diffCursor, "cursor must clamp at last line (handleDiffAction clamps internally)")
	assert.Equal(t, 0, model.vim.count)
}

func TestRepeatDiffAction_ZeroCountNoMotion(t *testing.T) {
	m := vimTestModel(t, 50)
	m.nav.diffCursor = 10

	result, _, handled := m.repeatDiffAction(keymap.ActionDown, 0)
	require.True(t, handled, "repeatDiffAction always returns handled=true")
	model := result.(Model)
	assert.Equal(t, 10, model.nav.diffCursor, "zero iterations leaves cursor alone")
}

// paneName returns a human-readable name for a pane constant, used in test
// subtest names. Keeps test output readable without exporting pane.
func paneName(p pane) string {
	switch p {
	case paneDiff:
		return "diff"
	case paneTree:
		return "tree"
	default:
		return "unknown"
	}
}

// full-flow integration tests: drive the vim-motion interceptor through
// Model.Update to exercise the handleKey pipeline end-to-end (hint clear,
// pending-reload guard, chord-second guard, handleModalKey, interceptor,
// keymap.Resolve, dispatchAction). Each test simulates a realistic key
// sequence and asserts the final model state.

func TestVimMotion_FullFlow_5j(t *testing.T) {
	m := vimTestModel(t, 100)

	result, _ := m.Update(keyMsg('5'))
	model := result.(Model)
	require.Equal(t, 5, model.vim.count, "digit accumulates into vim.count")
	require.Equal(t, "5", model.vim.hint)

	result, _ = model.Update(keyMsg('j'))
	model = result.(Model)
	assert.Equal(t, 5, model.nav.diffCursor, "5j moves cursor 5 lines down")
	assert.Equal(t, 0, model.vim.count, "count cleared after consuming motion")
	assert.Empty(t, model.vim.hint, "hint cleared after dispatch")
}

func TestVimMotion_FullFlow_gg(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 50

	result, _ := m.Update(keyMsg('g'))
	model := result.(Model)
	require.Equal(t, "g", model.vim.leader, "first g enters leader state")
	require.Equal(t, "g…", model.vim.hint)

	result, _ = model.Update(keyMsg('g'))
	model = result.(Model)
	assert.Equal(t, 0, model.nav.diffCursor, "gg jumps to first line")
	assert.Empty(t, model.vim.leader, "leader cleared after chord resolves")
	assert.Empty(t, model.vim.hint)
}

func TestVimMotion_FullFlow_G_Bare(t *testing.T) {
	m := vimTestModel(t, 100)
	require.Equal(t, 0, m.vim.count, "precondition: no count pending")

	result, _ := m.Update(keyMsg('G'))
	model := result.(Model)
	assert.Equal(t, 99, model.nav.diffCursor, "bare G jumps to last line (index 99 for 100 lines)")
	assert.Equal(t, 0, model.vim.count, "count still zero")
	assert.Empty(t, model.vim.hint)
}

func TestVimMotion_FullFlow_5G(t *testing.T) {
	m := vimTestModel(t, 100)

	result, _ := m.Update(keyMsg('5'))
	model := result.(Model)
	require.Equal(t, 5, model.vim.count)

	result, _ = model.Update(keyMsg('G'))
	model = result.(Model)
	assert.Equal(t, 4, model.nav.diffCursor, "5G jumps to line 5 (0-indexed: 4)")
	assert.Equal(t, 0, model.vim.count, "count cleared after jump")
	assert.Empty(t, model.vim.hint)
}

func TestVimMotion_FullFlow_zz(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 50
	m.layout.viewport.SetYOffset(0)

	result, _ := m.Update(keyMsg('z'))
	model := result.(Model)
	require.Equal(t, "z", model.vim.leader)

	result, _ = model.Update(keyMsg('z'))
	model = result.(Model)
	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)
	expected := max(0, model.cursorViewportY()-pageHeight/2)
	assert.Equal(t, expected, model.layout.viewport.YOffset, "zz centers viewport on cursor")
	assert.Equal(t, 50, model.nav.diffCursor, "zz does not move cursor")
	assert.Empty(t, model.vim.leader)
	assert.Empty(t, model.vim.hint)
}

func TestVimMotion_FullFlow_zt(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 50
	m.layout.viewport.SetYOffset(0)

	result, _ := m.Update(keyMsg('z'))
	model := result.(Model)
	result, _ = model.Update(keyMsg('t'))
	model = result.(Model)

	cursorY := model.cursorViewportY()
	assert.Equal(t, max(0, cursorY), model.layout.viewport.YOffset, "zt places cursor at top of viewport")
	assert.Equal(t, 50, model.nav.diffCursor, "zt does not move cursor")
	assert.Empty(t, model.vim.leader)
}

func TestVimMotion_FullFlow_zb(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 50
	m.layout.viewport.SetYOffset(0)

	result, _ := m.Update(keyMsg('z'))
	model := result.(Model)
	result, _ = model.Update(keyMsg('b'))
	model = result.(Model)

	pageHeight := model.layout.viewport.Height
	require.Positive(t, pageHeight)
	cursorY := model.cursorViewportY()
	expected := max(0, cursorY-pageHeight+1)
	assert.Equal(t, expected, model.layout.viewport.YOffset, "zb places cursor on last visible row")
	assert.Equal(t, 50, model.nav.diffCursor, "zb does not move cursor")
	assert.Empty(t, model.vim.leader)
}

func TestVimMotion_FullFlow_ZZ(t *testing.T) {
	m := vimTestModel(t, 100)

	result, _ := m.Update(keyMsg('Z'))
	model := result.(Model)
	require.Equal(t, "Z", model.vim.leader, "first Z enters leader state")
	require.Equal(t, "Z…", model.vim.hint)

	result, cmd := model.Update(keyMsg('Z'))
	model = result.(Model)
	require.NotNil(t, cmd, "ZZ must dispatch a quit command")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "ZZ emits tea.QuitMsg")
	assert.False(t, model.discarded, "ZZ does not set discarded flag")
	assert.Empty(t, model.vim.leader)
}

func TestVimMotion_FullFlow_ZQ(t *testing.T) {
	// with an empty annotation store, handleDiscardQuit quits immediately
	// without the confirm dialog — matches the common flow at exit time.
	m := vimTestModel(t, 100)

	result, _ := m.Update(keyMsg('Z'))
	model := result.(Model)
	require.Equal(t, "Z", model.vim.leader)

	result, cmd := model.Update(keyMsg('Q'))
	model = result.(Model)
	require.NotNil(t, cmd, "ZQ must dispatch a quit command")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "ZQ emits tea.QuitMsg")
	assert.True(t, model.discarded, "ZQ sets discarded flag (annotations dropped)")
	assert.Empty(t, model.vim.leader)
}

func TestVimMotion_LeaderCancelled(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 10

	result, _ := m.Update(keyMsg('g'))
	model := result.(Model)
	require.Equal(t, "g", model.vim.leader)

	result, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = result.(Model)
	assert.Empty(t, model.vim.leader, "esc cancels leader silently")
	assert.Empty(t, model.vim.hint, "esc leaves no hint")
	assert.Equal(t, 10, model.nav.diffCursor, "esc during leader does not move cursor")
}

func TestVimMotion_UnknownChord(t *testing.T) {
	m := vimTestModel(t, 100)
	m.nav.diffCursor = 10

	result, _ := m.Update(keyMsg('g'))
	model := result.(Model)
	require.Equal(t, "g", model.vim.leader)

	result, _ = model.Update(keyMsg('x'))
	model = result.(Model)
	assert.Empty(t, model.vim.leader, "unknown chord clears leader")
	assert.Equal(t, "Unknown: gx", model.vim.hint, "unknown chord surfaces hint")
	assert.Equal(t, 10, model.nav.diffCursor, "unknown chord does not move cursor")
}

func TestVimMotion_CountThenUnrelatedKey(t *testing.T) {
	// 5 accumulates into count; q is not a count consumer so the interceptor
	// drops the count silently and falls through — q then dispatches ActionQuit
	// through normal keymap resolution.
	m := vimTestModel(t, 100)

	result, _ := m.Update(keyMsg('5'))
	model := result.(Model)
	require.Equal(t, 5, model.vim.count)

	result, cmd := model.Update(keyMsg('q'))
	model = result.(Model)
	assert.Equal(t, 0, model.vim.count, "count dropped silently")
	assert.Empty(t, model.vim.hint)
	require.NotNil(t, cmd, "q still dispatches ActionQuit")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "q fires tea.QuitMsg even after dropped count")
}

func TestVimMotion_MotionInTreePane(t *testing.T) {
	// vim motion (count-prefixed j/k/G) is diff-pane-only. In tree pane, digit
	// accumulation still happens (priority 2 is pane-agnostic), but the
	// subsequent j/k consumer drops the count silently and falls through so
	// the tree pane nav handles the key as a normal single-step move.
	m := vimTestModel(t, 100)
	m.layout.focus = paneTree

	result, _ := m.Update(keyMsg('5'))
	model := result.(Model)
	require.Equal(t, 5, model.vim.count, "digit accumulation works in any pane")

	result, _ = model.Update(keyMsg('j'))
	model = result.(Model)
	assert.Equal(t, 0, model.vim.count, "count dropped when j falls through in tree pane")
	assert.Empty(t, model.vim.hint)
	assert.Equal(t, 0, model.nav.diffCursor, "diff cursor unchanged — motion did not reach diff pane")
}

func TestVimMotion_ZQuitFromTreePane(t *testing.T) {
	// ZZ is pane-agnostic: the user can quit from any pane.
	m := vimTestModel(t, 100)
	m.layout.focus = paneTree

	result, _ := m.Update(keyMsg('Z'))
	model := result.(Model)
	require.Equal(t, "Z", model.vim.leader, "Z activates leader in tree pane")

	result, cmd := model.Update(keyMsg('Z'))
	model = result.(Model)
	require.NotNil(t, cmd, "ZZ from tree pane must dispatch quit")
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "ZZ from tree pane emits tea.QuitMsg")
	assert.Empty(t, model.vim.leader)
}
