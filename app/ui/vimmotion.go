package ui

import (
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/keymap"
)

// maxVimCount caps the count prefix accumulator. Matches typical vim buffer
// sizes and keeps the repeat loop bounded regardless of user input.
const maxVimCount = 9999

// vimChordTable maps leader+second-key concatenation to the action the chord
// dispatches. Leader is the first pressed letter (g/z/Z); second is the
// pressed key that resolves the chord. Entries are static and shared across
// Model instances.
var vimChordTable = map[string]keymap.Action{
	"gg": keymap.ActionHome,
	"zz": keymap.ActionScrollCenter,
	"zt": keymap.ActionScrollTop,
	"zb": keymap.ActionScrollBottom,
	"ZZ": keymap.ActionQuit,
	"ZQ": keymap.ActionDiscardQuit,
}

// interceptVimMotion runs the vim-motion preset state machine for a single key
// event. Returns handled=true when the interceptor consumed the key; handled
// =false lets handleKey fall through to the standard keymap resolution.
//
// Priority order (first match wins):
//  1. pending letter leader — resolved via vimChordTable or hint on miss
//  2. digit accumulation — builds the count prefix
//  3. G motion — bare G jumps to end, <N>G jumps to line N (diff pane only)
//  4. count consumer keys — j/k repeat cursor motion (diff pane only)
//  5. leader entry — g/z require diff pane, Z is pane-agnostic
//
// The interceptor is a pure state-machine layer: it never calls handleModalKey
// and never resolves through the keymap. Callers must gate invocation on
// m.modes.vimMotion to keep the cost at one branch per keypress when off.
//
// Invariant: every fall-through branch (handled=false) returns cmd=nil.
// handleKey relies on this — on fall-through it keeps the returned model but
// discards cmd. If a future branch needs to emit a command, promote it to
// handled=true instead.
func (m Model) interceptVimMotion(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	keyStr := msg.String()

	// priority 1: pending letter leader
	if m.vim.leader != "" {
		return m.resolveVimLeader(keyStr)
	}

	// priority 2: digit accumulation
	if isDigit(keyStr) {
		// bare "0" with no pending count falls through so it can bind normally
		if keyStr == "0" && m.vim.count == 0 {
			return m, nil, false
		}
		m.vim.count = min(m.vim.count*10+digitValue(keyStr), maxVimCount)
		m.vim.hint = strconv.Itoa(m.vim.count)
		return m, nil, true
	}

	// priority 3: G motion (diff pane only) — bare G goes to last line
	if keyStr == "G" && m.layout.focus == paneDiff {
		n := m.vim.count
		m.vim.count = 0
		m.vim.hint = ""
		if n == 0 {
			n = len(m.file.lines) // jumpToLineN clamps
		}
		m.jumpToLineN(n)
		return m, nil, true
	}

	// priority 4: other count consumer keys
	if m.vim.count > 0 {
		switch {
		case keyStr == "j" && m.layout.focus == paneDiff:
			return m.repeatDiffAction(keymap.ActionDown, m.vim.count)
		case keyStr == "k" && m.layout.focus == paneDiff:
			return m.repeatDiffAction(keymap.ActionUp, m.vim.count)
		}
		// count is active but key is not a consumer (or wrong pane): drop count
		// silently and fall through so the key still triggers its normal action
		// (e.g. "5q" still quits, "5j" in tree pane falls through to tree nav).
		m.vim.count = 0
		m.vim.hint = ""
		return m, nil, false
	}

	// priority 5: leader entry
	if (keyStr == "g" || keyStr == "z") && m.layout.focus == paneDiff {
		m.vim.leader = keyStr
		m.vim.hint = keyStr + "…"
		return m, nil, true
	}
	if keyStr == "Z" {
		m.vim.leader = keyStr
		m.vim.hint = keyStr + "…"
		return m, nil, true
	}

	return m, nil, false
}

// resolveVimLeader handles the second-stage key after a leader (g/z/Z) is
// pending. esc cancels silently; a bound second-stage key dispatches the
// chord's action; any other key clears the leader and surfaces an
// "Unknown: <leader><key>" hint. The leader is always cleared before returning.
func (m Model) resolveVimLeader(keyStr string) (tea.Model, tea.Cmd, bool) {
	leader := m.vim.leader
	m.vim.leader = ""
	m.vim.hint = ""
	if keyStr == "esc" {
		return m, nil, true
	}
	action, ok := vimChordTable[leader+keyStr]
	if !ok {
		m.vim.hint = "Unknown: " + leader + keyStr
		return m, nil, true
	}
	model, cmd := m.dispatchAction(action)
	return model, cmd, true
}

// repeatDiffAction applies the given cursor motion n times, then syncs the
// viewport once. Used by count-prefixed j/k motion to turn "5j" into five
// cursor advances without re-rendering after each step — looping through
// handleDiffAction would call syncViewportToCursor (and therefore renderDiff)
// on every iteration, turning bounded counts like 9999 into multi-second
// hangs on large diffs.
//
// Only ActionDown and ActionUp are expected; any other action is a caller
// bug and returns without moving. Count and hint are cleared on the returned
// model so the next keypress starts fresh.
func (m Model) repeatDiffAction(action keymap.Action, n int) (tea.Model, tea.Cmd, bool) {
	m.vim.count = 0
	m.vim.hint = ""
	switch action {
	case keymap.ActionDown:
		for range n {
			m.moveDiffCursorDown()
		}
	case keymap.ActionUp:
		for range n {
			m.moveDiffCursorUp()
		}
	default:
		return m, nil, true
	}
	m.syncViewportToCursor()
	m.syncTOCActiveSection()
	return m, nil, true
}

// isDigit reports whether keyStr is a single-character string representing an
// ASCII digit (0-9). bubbletea reports these as single-rune key strings.
func isDigit(keyStr string) bool {
	if len(keyStr) != 1 {
		return false
	}
	c := keyStr[0]
	return c >= '0' && c <= '9'
}

// digitValue returns the integer value of a digit key string as returned by
// isDigit. Behavior is undefined for non-digit input.
func digitValue(keyStr string) int {
	return int(keyStr[0] - '0')
}
