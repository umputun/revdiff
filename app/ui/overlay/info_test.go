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

func infoRenderCtx() RenderCtx {
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

func TestInfoOverlay_RenderHeaderTextInTitle(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{HeaderText: "vs HEAD~3"})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "vs HEAD~3", "header text replaces the default 'info' title")
	// the literal " info " fallback should not appear when header text is set
	// (search for " info " surrounded by spaces to avoid matching e.g. "info popup")
	assert.NotContains(t, out, " info ─")
}

func TestInfoOverlay_RenderFooterText(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		HeaderText: "working tree",
		FooterText: "22 files · +231/-18 · A2 M20",
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "22 files")
	assert.Contains(t, out, "+231/-18")
	assert.Contains(t, out, "A2 M20")
	// footer should be on the bottom border, not the body — verify it sits
	// after any body content
	lines := strings.Split(out, "\n")
	require.Greater(t, len(lines), 2)
	bottomLine := lines[len(lines)-1]
	assert.Contains(t, bottomLine, "22 files", "footer text lives on the bottom border")
}

func TestInfoOverlay_DefaultTitleWhenNoHeaderText(t *testing.T) {
	// focused-test mode: no review-info config, popup falls back to plain " info " title.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		CommitsApplicable: true, CommitsLoaded: true,
		Commits: []diff.CommitInfo{{Hash: "abc"}},
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, " info ", "default title is used when HeaderText is empty")
}

func TestInfoOverlay_FooterGracefullyDegradesOnNarrowPopup(t *testing.T) {
	// regression: narrow terminals shouldn't crash or render a corrupted bottom
	// border. injectBorderEdgeText bails when the text wouldn't fit.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		HeaderText: "this is a deliberately long header that won't fit in a narrow popup",
		FooterText: "this is a deliberately long footer that won't fit in a narrow popup",
	})
	ctx := RenderCtx{Width: 30, Height: 10, Resolver: style.PlainResolver()}
	out := mgr.info.render(ctx, mgr)
	// must not panic; popup still renders something
	assert.NotEmpty(t, out)
}

func TestInfoOverlay_NoCommitsModeStillUseful(t *testing.T) {
	// regression for the "no commits" image Sasha flagged: when commits are
	// hidden, the popup must still convey meaningful info via header + footer
	// + (when present) detail rows. With no description, no detail rows, and
	// no commits section, header and footer alone carry the popup.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		HeaderText:        "working tree",
		FooterText:        "22 files · +231/-18 · A2 M20 · git",
		CommitsApplicable: false,
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "working tree", "header reads as the popup title")
	assert.Contains(t, out, "22 files")
	assert.Contains(t, out, "+231/-18")
	assert.NotContains(t, out, "no info available", "header+footer mean the popup is non-empty")
	assert.NotContains(t, out, "details", "details section header hidden when there are no detail rows")
}

func TestInfoOverlay_DetailRowMutedSuffixRendered(t *testing.T) {
	// MutedSuffix renders after Value with the muted-fg ANSI escape so the
	// secondary token (full path, etc.) reads as contextual rather than
	// primary. Empty MutedSuffix preserves the legacy single-value layout.
	mgr := NewManager()
	c := style.Colors{Accent: "#5f87ff", Muted: "#888888", Normal: "#d0d0d0", Border: "#585858"}
	resolver := style.NewResolver(c)
	mgr.OpenInfo(InfoSpec{
		HeaderText: "info",
		Rows: []InfoRow{
			{Label: "project", Value: "revdiff", MutedSuffix: "/home/sasha/Developer/revdiff"},
		},
	})
	out := mgr.info.render(RenderCtx{Width: 100, Height: 12, Resolver: resolver}, mgr)

	assert.Contains(t, out, "revdiff", "primary value rendered")
	assert.Contains(t, out, "/home/sasha/Developer/revdiff", "muted suffix appended after value")
	assert.Contains(t, out, style.AnsiFg("#888888"), "muted fg escape present for the suffix")
	// the suffix must come AFTER the value, not before — verify by index
	valueIdx := strings.Index(out, "revdiff")
	suffixIdx := strings.Index(out, "/home/sasha")
	require.Positive(t, valueIdx)
	require.Positive(t, suffixIdx)
	assert.Less(t, valueIdx, suffixIdx, "MutedSuffix renders after the primary value")
}

func TestInfoOverlay_DetailRowsRenderWithSectionHeader(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		HeaderText: "working tree",
		FooterText: "1 file · +0/-0",
		Rows: []InfoRow{
			{Label: "only", Value: "main.go"},
			{Label: "project", Value: "repo", MutedSuffix: "/home/x/repo"},
		},
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "details", "section header aligns with description and commits")
	assert.Contains(t, out, "only")
	assert.Contains(t, out, "main.go")
	assert.Contains(t, out, "project")
	assert.Contains(t, out, "repo")
	assert.Contains(t, out, "/home/x/repo", "MutedSuffix renders the full path next to the basename")
}

func TestInfoOverlay_RenderCommitsHiddenWhenNotApplicable(t *testing.T) {
	// regression: in the merged overlay the commits section is HIDDEN entirely
	// when CommitsApplicable=false (the session section already explains the
	// mode, so a "no commits in this mode" stub would be redundant).
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: false})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.NotContains(t, out, "commits", "commits section must be hidden when not applicable")
	assert.NotContains(t, out, "no commits in this mode", "old dead-end message must be gone")
}

func TestInfoOverlay_RenderSessionRowsAlwaysShownEvenWithoutCommits(t *testing.T) {
	// the popup should be useful in every mode — including those that hide
	// the commits section. Session rows render on their own; the title in
	// the top border becomes the only "commits" visible reference.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		CommitsApplicable: false,
		Rows: []InfoRow{
			{Label: "mode", Value: "stdin scratch buffer"},
			{Label: "scope", Value: "--stdin patch.diff"},
		},
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "stdin scratch buffer")
	assert.Contains(t, out, "--stdin patch.diff")
	assert.NotContains(t, out, "commits", "commits section hidden")
	assert.NotContains(t, out, "no info available", "session rows mean popup is non-empty")
}

func TestInfoOverlay_RenderCommitsLoadingState(t *testing.T) {
	// regression: in the merged overlay, opening the popup before commits
	// finish loading must show "loading commits…" inside the section instead
	// of "no commits in range" (which would be wrong) or "(0)" in the header
	// (which would mislead).
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: false})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "loading commits")
	assert.NotContains(t, out, "commits (0)", "header must not show a count while loading")
	assert.NotContains(t, out, "no commits in range")
}

func TestInfoOverlay_RenderDescriptionSection(t *testing.T) {
	// description section renders prose at the top of the popup, above session
	// info and above commits. Empty description hides the section entirely.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		Description:       "This change refactors the auth middleware.\n\nSee comment for context.",
		CommitsApplicable: false,
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "description")
	assert.Contains(t, out, "refactors the auth middleware")
	assert.Contains(t, out, "See comment for context")
}

func TestInfoOverlay_DescriptionSectionHiddenWhenEmpty(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		Description:       "   ",
		CommitsApplicable: true, CommitsLoaded: true,
		Rows: []InfoRow{{Label: "mode", Value: "x"}},
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.NotContains(t, out, "description", "whitespace-only description must hide the section header")
}

func TestInfoOverlay_SectionOrder(t *testing.T) {
	// description → session → commits, each separated by a blank line.
	commit := makeCommit("abc123def456", "alice", "subject", "", time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		Description:       "agent says hi",
		Rows:              []InfoRow{{Label: "mode", Value: "stdin scratch buffer"}},
		CommitsApplicable: true, CommitsLoaded: true,
		Commits: []diff.CommitInfo{commit},
	})
	out := mgr.info.render(infoRenderCtx(), mgr)

	descIdx := strings.Index(out, "agent says hi")
	sessionIdx := strings.Index(out, "stdin scratch buffer")
	commitsIdx := strings.Index(out, "abc123def456")

	require.Positive(t, descIdx, "description must render")
	require.Positive(t, sessionIdx, "session must render")
	require.Positive(t, commitsIdx, "commits must render")
	assert.Less(t, descIdx, sessionIdx, "description must come before session")
	assert.Less(t, sessionIdx, commitsIdx, "session must come before commits")
}

func TestInfoOverlay_RenderEmptyList(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "no commits in range")
}

func TestInfoOverlay_RenderError(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, CommitsErr: errors.New("git log failed")})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "git log failed")
	assert.Contains(t, out, ansiItalicOn, "error text rendered italic")
}

func TestInfoOverlay_RenderErrorFlattensMultilineMessage(t *testing.T) {
	// runVCS embeds raw stderr into errors which may contain newlines and tabs;
	// buildContent must flatten before centering or the popup layout breaks.
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		CommitsApplicable: true, CommitsLoaded: true,
		CommitsErr: errors.New("git log failed:\nfatal: bad revision\n\ttry a different ref"),
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	// flattened: whitespace runs (newlines, tabs) collapse to single spaces
	assert.Contains(t, out, "git log failed: fatal: bad revision try a different ref")
	// raw newline/tab sequences from the error must not survive
	assert.NotContains(t, out, "failed:\nfatal")
	assert.NotContains(t, out, "revision\n\ttry")
}

func TestInfoOverlay_RenderErrorTruncatesHugeMessage(t *testing.T) {
	// raw VCS stderr can be megabytes (verbose tools, hostile branches);
	// the error renderer must cap length so popup rendering stays bounded.
	huge := strings.Repeat("x", infoErrMaxLen*5)
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{
		CommitsApplicable: true, CommitsLoaded: true,
		CommitsErr: errors.New(huge),
	})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "(truncated)")
	// total visible 'x' run must not exceed the cap (slack allowed for borders/padding)
	assert.Less(t, strings.Count(out, "x"), infoErrMaxLen+50)
}

func TestInfoOverlay_RenderSingleCommit(t *testing.T) {
	date := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	commit := makeCommit("abcdef1234567890", "Alice <alice@example.com>", "Short subject", "Body line one.\nBody line two.", date)
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(infoRenderCtx(), mgr)

	assert.Contains(t, out, "abcdef123456", "short hash rendered")
	assert.NotContains(t, out, "abcdef1234567890", "full hash should be truncated to short form")
	assert.Contains(t, out, "Alice <alice@example.com>")
	assert.Contains(t, out, "2026-04-17")
	assert.Contains(t, out, "Short subject")
	assert.Contains(t, out, "Body line one.")
	assert.Contains(t, out, "Body line two.")
	assert.Contains(t, out, "commits (1)")
}

func TestInfoOverlay_RenderTruncatedTitle(t *testing.T) {
	commit := makeCommit("aaaa111", "a", "s", "", time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Truncated: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "truncated")
}

func TestInfoOverlay_RenderSkipsEmptyBody(t *testing.T) {
	commit := makeCommit("1234567890ab", "me", "just a subject", "   ", time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "just a subject")
	// whitespace-only body should produce zero indented body rows
	lines := strings.Split(out, "\n")
	indent := infoBodyIndent
	bodyRows := 0
	for _, l := range lines {
		trimmed := strings.TrimLeft(l, "│ ")
		if strings.HasPrefix(trimmed, indent) && strings.TrimSpace(trimmed) != "" {
			bodyRows++
		}
	}
	assert.Zero(t, bodyRows, "whitespace-only body should not emit indented body rows")
}

func TestInfoOverlay_RenderLongBodyWraps(t *testing.T) {
	longBody := strings.Repeat("word ", 60) // ~300 chars, will wrap at 80-wide popup
	commit := makeCommit("1234567890ab", "me", "subject", longBody, time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(infoRenderCtx(), mgr)
	// content exists on multiple visible rows
	visibleRows := strings.Count(out, "word")
	assert.Greater(t, visibleRows, 2, "long body should span multiple wrapped rows")
}

func TestInfoOverlay_ScrollJK(t *testing.T) {
	// build many commits so content exceeds viewport
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	// force a render to set height
	_ = mgr.info.render(infoRenderCtx(), mgr)

	// j -> offset +1
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 1, mgr.info.offset)

	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}, keymap.ActionDown)
	assert.Equal(t, 2, mgr.info.offset)

	// k -> offset -1
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 1, mgr.info.offset)

	// k at 0 -> stays at 0
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}, keymap.ActionUp)
	assert.Equal(t, 0, mgr.info.offset)
}

func TestInfoOverlay_ScrollPageUpDown(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	ctx := infoRenderCtx()
	_ = mgr.info.render(ctx, mgr)

	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyPgDown}, keymap.ActionPageDown)
	assert.Positive(t, mgr.info.offset, "pgdown advances offset by viewport height")

	firstPageOffset := mgr.info.offset
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyPgUp}, keymap.ActionPageUp)
	assert.Less(t, mgr.info.offset, firstPageOffset, "pgup rewinds offset")
}

func TestInfoOverlay_ScrollHalfPageUpDown(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	_ = mgr.info.render(infoRenderCtx(), mgr)

	mgr.info.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageDown)
	assert.Positive(t, mgr.info.offset, "half-pgdown advances offset by viewport height")

	firstOffset := mgr.info.offset
	mgr.info.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageUp)
	assert.Less(t, mgr.info.offset, firstOffset, "half-pgup rewinds offset")

	// half-pgup below zero should clamp to zero
	mgr.info.offset = 1
	mgr.info.handleKey(tea.KeyMsg{}, keymap.ActionHalfPageUp)
	assert.Equal(t, 0, mgr.info.offset, "half-pgup clamps to zero")
}

func TestInfoOverlay_ScrollGG(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	_ = mgr.info.render(infoRenderCtx(), mgr)

	// G -> jump to bottom (offset clamped in render)
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}, keymap.Action(""))
	assert.Equal(t, scrollEndSentinel, mgr.info.offset, "G sets offset to end sentinel")

	_ = mgr.info.render(infoRenderCtx(), mgr)
	assert.Less(t, mgr.info.offset, scrollEndSentinel, "render clamps sentinel to max valid offset")
	assert.Positive(t, mgr.info.offset)

	// g -> jump to top
	mgr.info.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}, keymap.Action(""))
	assert.Equal(t, 0, mgr.info.offset)
}

func TestInfoOverlay_ScrollHomeEnd(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 30)
	for range 30 {
		commits = append(commits, makeCommit("hash", "a", "subject", "", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	_ = mgr.info.render(infoRenderCtx(), mgr)

	mgr.info.handleKey(tea.KeyMsg{}, keymap.ActionEnd)
	assert.Equal(t, scrollEndSentinel, mgr.info.offset)

	mgr.info.handleKey(tea.KeyMsg{}, keymap.ActionHome)
	assert.Equal(t, 0, mgr.info.offset)
}

func TestInfoOverlay_OffsetClamping(t *testing.T) {
	// single short commit - no scrolling possible
	commit := makeCommit("1234567890ab", "a", "subject", "", time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})

	// set offset artificially high
	mgr.info.offset = 100
	_ = mgr.info.render(infoRenderCtx(), mgr)
	assert.Equal(t, 0, mgr.info.offset, "offset clamped to 0 when content fits in viewport")

	mgr.info.offset = -5
	_ = mgr.info.render(infoRenderCtx(), mgr)
	assert.Equal(t, 0, mgr.info.offset, "negative offset clamped to 0")
}

func TestInfoOverlay_HandleKeyClose(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})

	t.Run("esc closes", func(t *testing.T) {
		mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyEsc}, keymap.ActionDismiss)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})

	t.Run("info action closes", func(t *testing.T) {
		mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}, keymap.ActionInfo)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})

	t.Run("q closes", func(t *testing.T) {
		mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
		out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}, keymap.ActionQuit)
		assert.Equal(t, OutcomeClosed, out.Kind)
		assert.False(t, mgr.Active())
	})
}

func TestInfoOverlay_HandleMouse_WheelScrollsOffset(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 20)
	for range 20 {
		commits = append(commits, makeCommit("hash", "a", "subject", "body", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	// render once so viewport height + content are realized; offset stays 0
	_ = mgr.info.render(infoRenderCtx(), mgr)
	require.Equal(t, 0, mgr.info.offset)

	t.Run("wheel down advances offset by WheelStep", func(t *testing.T) {
		mgr.info.offset = 0
		out := mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.Equal(t, OutcomeNone, out.Kind)
		assert.Equal(t, WheelStep, mgr.info.offset)
	})

	t.Run("wheel up decreases offset and clamps at 0", func(t *testing.T) {
		mgr.info.offset = 1
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
		assert.Equal(t, 0, mgr.info.offset, "clamped to zero when step exceeds offset")
	})

	t.Run("shift+wheel uses half viewport step", func(t *testing.T) {
		mgr.info.offset = 0
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress, Shift: true})
		want := max(mgr.info.viewportHeight(mgr.info.height)/2, 1)
		assert.Equal(t, want, mgr.info.offset)
	})

	t.Run("non-press wheel ignored", func(t *testing.T) {
		mgr.info.offset = 5
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionRelease})
		assert.Equal(t, 5, mgr.info.offset, "release action must not scroll")
	})

	t.Run("non-wheel button ignored", func(t *testing.T) {
		mgr.info.offset = 5
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		assert.Equal(t, 5, mgr.info.offset, "left click does not scroll")
	})

	t.Run("overlay stays open after wheel", func(t *testing.T) {
		mgr.HandleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
		assert.True(t, mgr.Active())
	})
}

func TestInfoOverlay_UnhandledKey(t *testing.T) {
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
	out := mgr.HandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, keymap.Action(""))
	assert.Equal(t, OutcomeNone, out.Kind)
	assert.True(t, mgr.Active(), "unrecognized key does not close overlay")
}

func TestManager_OpenInfoClosesHelp(t *testing.T) {
	mgr := NewManager()
	mgr.OpenHelp(HelpSpec{})
	require.Equal(t, KindHelp, mgr.Kind())
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true})
	assert.Equal(t, KindInfo, mgr.Kind(), "opening info closes existing help overlay")
}

func TestInfoOverlay_ColorsDeliveredViaResolver(t *testing.T) {
	c := style.Colors{Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Border: "#585858"}
	resolver := style.NewResolver(c)
	ctx := RenderCtx{Width: 100, Height: 30, Resolver: resolver}

	date := time.Date(2026, 4, 17, 12, 0, 0, 0, time.UTC)
	commit := makeCommit("1234567890abcdef", "Alice", "Subject", "body", date)
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(ctx, mgr)

	assert.Contains(t, out, style.AnsiFg("#5f87ff"), "accent color escape present for hash")
	assert.Contains(t, out, style.AnsiFg("#6c6c6c"), "muted color escape present for author/date")
	assert.Contains(t, out, ansiBoldOn, "bold on present for subject")
	assert.Contains(t, out, ansiBoldOff, "bold off present")
}

func TestInfoOverlay_MetaWrapReemitsSGRState(t *testing.T) {
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
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(ctx, mgr)

	muted := style.AnsiFg("#6c6c6c")
	// the muted escape must appear at least twice: once for the original span
	// and at least once more reemitted on a wrapped continuation line.
	// without Reemit, a wrapped continuation would render with no fg escape.
	require.GreaterOrEqual(t, strings.Count(out, muted), 2,
		"muted escape should appear on the original meta span and be reemitted on at least one wrapped continuation line")
}

func TestInfoOverlay_PopupWidthClamping(t *testing.T) {
	c := &infoOverlay{}
	assert.Equal(t, infoPopupMaxWidth, c.popupWidth(200), "clamps to max for wide terms")
	assert.Equal(t, infoPopupMinWidth, c.popupWidth(10), "floors at min for narrow terms")
	// 80-col wide term * 0.9 = 72 (between min and max)
	assert.Equal(t, 72, c.popupWidth(80))
}

func TestInfoOverlay_ViewportHeightFloor(t *testing.T) {
	c := &infoOverlay{}
	// pathological small terminal — always returns at least 1
	assert.GreaterOrEqual(t, c.viewportHeight(1), 1)
	assert.GreaterOrEqual(t, c.viewportHeight(5), 1)
}

func TestInfoOverlay_ManyCommitsRender(t *testing.T) {
	commits := make([]diff.CommitInfo, 0, 10)
	for range 10 {
		commits = append(commits, makeCommit("hash", "a", "subject line", "body line", time.Time{}))
	}
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: commits})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "commits (10)")
}

func TestInfoOverlay_AnsiInjectionRendersLiteral(t *testing.T) {
	// Parser (tasks 1-3) strips \x1b; overlay receives pre-stripped input and
	// treats any brackets as literal text, not escape sequences.
	commit := makeCommit("abc", "me", "[31mfake red[0m", "[32mfake green[0m", time.Time{})
	mgr := NewManager()
	mgr.OpenInfo(InfoSpec{CommitsApplicable: true, CommitsLoaded: true, Commits: []diff.CommitInfo{commit}})
	out := mgr.info.render(infoRenderCtx(), mgr)
	assert.Contains(t, out, "[31mfake red[0m", "bracketed payload rendered literally")
	assert.Contains(t, out, "[32mfake green[0m")
}

func TestSanitizeInfoText_stripsFullCSISequences(t *testing.T) {
	// Earlier sanitizer dropped only the ESC byte and left "[31m...[0m" payload
	// visible. After routing through diff.SanitizeCommitText, the entire CSI
	// sequence is removed.
	c := &infoOverlay{}
	got := c.sanitizeInfoText("\x1b[31mhello\x1b[0m world")
	assert.Equal(t, "hello world", got)
	assert.NotContains(t, got, "[31m")
	assert.NotContains(t, got, "[0m")
}

func TestSanitizeInfoText_mapsLFAndTABToSpaces(t *testing.T) {
	// Each surviving LF or TAB after sanitization is mapped to a single space
	// so values with embedded newlines render on one row. Runs of LF/TAB
	// expand to runs of spaces (no whitespace-run collapsing).
	c := &infoOverlay{}
	got := c.sanitizeInfoText("foo\nbar\tbaz")
	assert.Equal(t, "foo bar baz", got)
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
	c := &infoOverlay{}
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
		{name: "exactly limit", in: strings.Repeat("a", infoShortHashLen), want: strings.Repeat("a", infoShortHashLen)},
		{name: "longer than limit", in: "abcdef1234567890deadbeef", want: "abcdef1234567890deadbeef"[:infoShortHashLen]},
		{name: "unicode runes", in: "αβγδεζηθικλμνξοπρστυφ", want: string([]rune("αβγδεζηθικλμνξοπρστυφ")[:infoShortHashLen])},
	}
	c := &infoOverlay{}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, c.shortHash(tc.in))
		})
	}
}
