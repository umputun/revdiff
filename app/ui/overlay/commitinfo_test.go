package overlay

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/keymap"
	"github.com/umputun/revdiff/app/ui/style"
)

func commitInfoRenderCtx() RenderCtx {
	return RenderCtx{Width: 100, Height: 30, Resolver: style.PlainResolver()}
}

func makeCommit(hash, author, subject, body string, date time.Time) diff.CommitInfo {
	return diff.CommitInfo{
		Hash:    hash,
		Author:  author,
		Date:    date,
		Subject: subject,
		Body:    body,
	}
}

func TestCommitInfoOverlay_RenderApplicableFalse(t *testing.T) {
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: false})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "no commits in this mode")
	assert.Contains(t, out, "commits (0)", "title reflects empty list")
}

func TestCommitInfoOverlay_RenderEmptyList(t *testing.T) {
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "no commits in range")
}

func TestCommitInfoOverlay_RenderError(t *testing.T) {
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Err: errors.New("git log failed")})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "git log failed")
	assert.Contains(t, out, ansiItalicOn, "error text rendered italic")
}

func TestCommitInfoOverlay_RenderErrorFlattensMultilineMessage(t *testing.T) {
	// runVCS embeds raw stderr into errors which may contain newlines and tabs;
	// buildContent must flatten before centering or the popup layout breaks.
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{
		Applicable: true,
		Err:        errors.New("git log failed:\nfatal: bad revision\n\ttry a different ref"),
	})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	// flattened: whitespace runs (newlines, tabs) collapse to single spaces
	assert.Contains(t, out, "git log failed: fatal: bad revision try a different ref")
	// raw newline/tab sequences from the error must not survive
	assert.NotContains(t, out, "failed:\nfatal")
	assert.NotContains(t, out, "revision\n\ttry")
}

func TestCommitInfoOverlay_RenderSingleCommit(t *testing.T) {
	date := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	commit := makeCommit("abcdef1234567890", "Alice <alice@example.com>", "Short subject", "Body line one.\nBody line two.", date)
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)

	assert.Contains(t, out, "abcdef123456", "short hash rendered")
	assert.NotContains(t, out, "abcdef1234567890", "full hash should be truncated to short form")
	assert.Contains(t, out, "Alice <alice@example.com>")
	assert.Contains(t, out, "2026-04-17")
	assert.Contains(t, out, "Short subject")
	assert.Contains(t, out, "Body line one.")
	assert.Contains(t, out, "Body line two.")
	assert.Contains(t, out, "commits (1)")
}

func TestCommitInfoOverlay_RenderTruncatedTitle(t *testing.T) {
	commit := makeCommit("aaaa111", "a", "s", "", time.Time{})
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Truncated: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "truncated")
}

func TestCommitInfoOverlay_RenderSkipsEmptyBody(t *testing.T) {
	commit := makeCommit("1234567890ab", "me", "just a subject", "   ", time.Time{})
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "just a subject")
	// whitespace-only body should produce zero indented body rows
	lines := strings.Split(out, "\n")
	indent := commitInfoBodyIndent
	bodyRows := 0
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, "│ ")
		if strings.HasPrefix(trimmed, indent) && strings.TrimSpace(trimmed) != "" {
			bodyRows++
		}
	}
	assert.Zero(t, bodyRows, "whitespace-only body should not emit indented body rows")
}

func TestCommitInfoOverlay_RenderLongBodyWraps(t *testing.T) {
	longBody := strings.Repeat("word ", 60) // ~300 chars, will wrap at 80-wide popup
	commit := makeCommit("1234567890ab", "me", "subject", longBody, time.Time{})
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	// content exists on multiple visible rows
	visibleRows := strings.Count(out, "word")
	assert.Greater(t, visibleRows, 2, "long body should span multiple wrapped rows")
}

func TestCommitInfoOverlay_ScrollJK(t *testing.T) {
	// build many commits so content exceeds viewport
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	// force a render to set height
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)

	// j -> offset +1
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 1, mgr.commitInfo.offset)

	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 2, mgr.commitInfo.offset)

	// k -> offset -1
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 1, mgr.commitInfo.offset)

	// k at 0 -> stays at 0
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 0, mgr.commitInfo.offset)
}

func TestCommitInfoOverlay_ScrollPageUpDown(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	ctx := commitInfoRenderCtx()
	_ = mgr.commitInfo.render(ctx, mgr)

	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyPgDown}, keymap.ActionPageDown)
	assert.Positive(t, mgr.commitInfo.offset, "pgdown advances offset by viewport height")

	firstPageOffset := mgr.commitInfo.offset
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyPgUp}, keymap.ActionPageUp)
	assert.Less(t, mgr.commitInfo.offset, firstPageOffset, "pgup rewinds offset")
}

func TestCommitInfoOverlay_ScrollHalfPageUpDown(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)

	mgr.commitInfo.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageDown)
	assert.Positive(t, mgr.commitInfo.offset, "half-pgdown advances offset by viewport height")

	firstOffset := mgr.commitInfo.offset
	mgr.commitInfo.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageUp)
	assert.Less(t, mgr.commitInfo.offset, firstOffset, "half-pgup rewinds offset")

	// half-pgup below zero should clamp to zero
	mgr.commitInfo.offset = 1
	mgr.commitInfo.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageUp)
	assert.Equal(t, 0, mgr.commitInfo.offset, "half-pgup clamps to zero")
}

func TestCommitInfoOverlay_ScrollGG(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)

	// G -> jump to bottom (offset clamped in render)
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, keymap.Action(""))
	assert.Equal(t, scrollEndSentinel, mgr.commitInfo.offset, "G sets offset to end sentinel")

	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Less(t, mgr.commitInfo.offset, scrollEndSentinel, "render clamps sentinel to max valid offset")
	assert.Positive(t, mgr.commitInfo.offset)

	// g -> jump to top
	mgr.commitInfo.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, keymap.Action(""))
	assert.Equal(t, 0, mgr.commitInfo.offset)
}

func TestCommitInfoOverlay_ScrollHomeEnd(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)

	mgr.commitInfo.handleKey(tea.KeyMsg{}, keymap.ActionEnd)
	assert.Equal(t, scrollEndSentinel, mgr.commitInfo.offset)

	mgr.commitInfo.handleKey(tea.KeyMsg{}, keymap.ActionHome)
	assert.Equal(t, 0, mgr.commitInfo.offset)
}

func TestCommitInfoOverlay_OffsetClamping(t *testing.T) {
	// single short commit - no scrolling possible
	commit := makeCommit("1234567890ab", "a", "subject", "", time.Time{})
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})

	// set offset artificially high
	mgr.commitInfo.offset = 100
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Equal(t, 0, mgr.commitInfo.offset, "offset clamped to 0 when content fits in viewport")

	mgr.commitInfo.offset = -5
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Equal(t, 0, mgr.commitInfo.offset, "negative offset clamped to 0")
}

func TestCommitInfoOverlay_HandleKeyClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})

	t.Run("esc closes", func(t *testing.T) {
		mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})

	t.Run("commit-info action closes", func(t *testing.T) {
		mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}, keymap.ActionCommitInfo)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})

	t.Run("q closes", func(t *testing.T) {
		mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})
}

func TestCommitInfoOverlay_HandleMouse_WheelScrollsOffset(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 20)
	for range 20 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	// render once so viewport height + content are realized; offset stays 0
	_ = mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	require.Equal(t, 0, mgr.commitInfo.offset)

	t.Run("wheel down advances offset by WheelStep", func(t *testing.T) {
		mgr.commitInfo.offset = 0
		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeNone, out.Kind)
		assert.Equal(t, WheelStep, mgr.commitInfo.offset)
	})

	t.Run("wheel up decreases offset and clamps at 0", func(t *testing.T) {
		mgr.commitInfo.offset = 1
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		assert.Equal(t, 0, mgr.commitInfo.offset, "clamped to zero when step exceeds offset")
	})

	t.Run("shift+wheel uses half viewport step", func(t *testing.T) {
		mgr.commitInfo.offset = 0
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true})
		want := max(mgr.commitInfo.viewportHeight(mgr.commitInfo.height)/2, 1)
		assert.Equal(t, want, mgr.commitInfo.offset)
	})

	t.Run("non-press wheel ignored", func(t *testing.T) {
		mgr.commitInfo.offset = 5
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
		assert.Equal(t, 5, mgr.commitInfo.offset, "release action must not scroll")
	})

	t.Run("non-wheel button ignored", func(t *testing.T) {
		mgr.commitInfo.offset = 5
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		assert.Equal(t, 5, mgr.commitInfo.offset, "left click does not scroll")
	})

	t.Run("overlay stays open after wheel", func(t *testing.T) {
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.True(t, mgr.Active())
	})
}

func TestCommitInfoOverlay_UnhandledKey(t *testing.T) {
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.Action(""))
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.True(t, mgr.Active(), "unrecognized key does not close overlay")
}

func TestManager_OpenCommitInfoClosesHelp(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(HelpSpec{})
	require.Equal(t, KindHelp, mgr.Kind())
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true})
	assert.Equal(t, KindCommitInfo, mgr.Kind(), "opening commit info closes existing help overlay")
}

func TestCommitInfoOverlay_ColorsDeliveredViaResolver(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Border: "#585858"}
	resolver := style.NewResolver(c)
	ctx := RenderCtx{Width: 100, Height: 30, Resolver: resolver}

	date := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	commit := makeCommit("1234567890abcdef", "Alice", "Subject", "body", date)
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(ctx, mgr)

	assert.Contains(t, out, style.AnsiFg("#5f87ff"), "accent color escape present for hash")
	assert.Contains(t, out, style.AnsiFg("#6c6c6c"), "muted color escape present for author/date")
	assert.Contains(t, out, ansiBoldOn, "bold on present for subject")
	assert.Contains(t, out, ansiBoldOff, "bold off present")
}

func TestCommitInfoOverlay_MetaWrapReemitsSGRState(t *testing.T) {
	// long author email forces the meta line to wrap inside a muted-colored span;
	// wrapLine must re-emit the muted escape on the continuation line, otherwise
	// the continuation renders with default fg and looks inconsistent.
	c := style.Colors{Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Border: "#585858"}
	resolver := style.NewResolver(c)
	ctx := RenderCtx{Width: 60, Height: 30, Resolver: resolver} // narrow to force wrap

	longAuthor := "Alice Verylongname <alice.verylongname@example-with-long-domain.com>"
	date := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	commit := makeCommit("abcdef1234567890", longAuthor, "subject", "", date)

	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(ctx, mgr)

	muted := style.AnsiFg("#6c6c6c")
	// the muted escape must appear at least twice: once for the original span
	// and at least once more reemitted on a wrapped continuation line.
	// without Reemit, a wrapped continuation would render with no fg escape.
	require.GreaterOrEqual(t, strings.Count(out, muted), 2,
		"muted escape should appear on the original meta span and be reemitted on at least one wrapped continuation line")
}

func TestCommitInfoOverlay_PopupWidthClamping(t *testing.T) {
	c := &commitInfoOverlay{}
	assert.Equal(t, commitInfoPopupMaxWidth, c.popupWidth(200), "clamps to max for wide terms")
	assert.Equal(t, commitInfoPopupMinWidth, c.popupWidth(10), "floors at min for narrow terms")
	// 80-col wide term * 0.9 = 72 (between min and max)
	assert.Equal(t, 72, c.popupWidth(80))
}

func TestCommitInfoOverlay_ViewportHeightFloor(t *testing.T) {
	c := &commitInfoOverlay{}
	// pathological small terminal — always returns at least 1
	assert.GreaterOrEqual(t, c.viewportHeight(1), 1)
	assert.GreaterOrEqual(t, c.viewportHeight(5), 1)
}

func TestCommitInfoOverlay_ManyCommitsRender(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 10)
	for range 10 {
		commits = append(commits, makeCommit("hash", "a", "subject line", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: commits})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "commits (10)")
}

func TestCommitInfoOverlay_AnsiInjectionRendersLiteral(t *testing.T) {
	// Parser (tasks 1-3) strips \x1b; overlay receives pre-stripped input and
	// treats any brackets as literal text, not escape sequences.
	commit := makeCommit("abc", "me", "[31mfake red[0m", "[32mfake green[0m", time.Time{})
	mgr := NewManager()
	mgr.OpenCommitInfo(CommitInfoSpec{Applicable: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.commitInfo.render(commitInfoRenderCtx(), mgr)
	assert.Contains(t, out, "[31mfake red[0m", "bracketed payload rendered literally")
	assert.Contains(t, out, "[32mfake green[0m")
}

func TestSplitBodyLines(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: []string{}},
		{name: "single line", in: "one", want: []string{"one"}},
		{name: "lf", in: "one\ntwo", want: []string{"one", "two"}},
		{name: "crlf", in: "one\r\ntwo", want: []string{"one", "two"}},
		{name: "cr only", in: "one\rtwo", want: []string{"one", "two"}},
		{name: "mixed cr crlf", in: "one\r\ntwo\rthree", want: []string{"one", "two", "three"}},
		{name: "trailing blank lines dropped", in: "one\n\n  \n", want: []string{"one"}},
		{name: "all blank body", in: "\n \n\t\n", want: []string{}},
	}
	c := &commitInfoOverlay{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := c.splitBodyLines(tc.in)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestShortHash(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "shorter than limit", in: "abc", want: "abc"},
		{name: "exactly limit", in: strings.Repeat("a", commitInfoShortHashLen), want: strings.Repeat("a", commitInfoShortHashLen)},
		{name: "longer than limit", in: "abcdef1234567890deadbeef", want: "abcdef1234567890deadbeef"[:commitInfoShortHashLen]},
		{name: "unicode runes", in: "αβγδεζηθικλμνξοπρστυφ", want: string([]rune("αβγδεζηθικλμνξοπρστυφ")[:commitInfoShortHashLen])},
	}
	c := &commitInfoOverlay{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, c.shortHash(tc.in))
		})
	}
}
