package ui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/annotation"
	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/review"
	"github.com/umputun/revdiff/app/ui/mocks"
	"github.com/umputun/revdiff/app/ui/overlay"
	"github.com/umputun/revdiff/app/ui/style"
)

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
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: &ReviewInfoConfig{}})
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
	m.review.cfg = &ReviewInfoConfig{AllFiles: true}
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
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: &ReviewInfoConfig{}})

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
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: &ReviewInfoConfig{}})

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
	m.review.cfg = &ReviewInfoConfig{}
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
	m.review.cfg = &ReviewInfoConfig{}
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
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: &ReviewInfoConfig{}})
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
	m := testNewModel(t, r, annotation.NewStore(), noopHighlighter(), ModelConfig{ReviewInfo: &ReviewInfoConfig{}})
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
		ReviewInfo: &ReviewInfoConfig{},
	})
	m.setReviewEntries(entries)

	stats := m.loadReviewStats(entries)().(reviewStatsLoadedMsg)
	assert.True(t, stats.Partial, "path-escape must mark partial")
	assert.Equal(t, 0, stats.Adds)
	assert.Equal(t, 0, stats.Removes)
}

func TestReviewLinesText_NoChangedLines(t *testing.T) {
	m := testModel(nil, nil)
	m.review.cfg = &ReviewInfoConfig{}
	m.review.statsLoaded = true
	assert.Equal(t, "no changed lines", m.reviewLinesText())

	m.review.partial = true
	assert.Equal(t, "no changed lines (partial)", m.reviewLinesText())
}

func TestReviewHeaderText(t *testing.T) {
	tests := []struct {
		name string
		cfg  *ReviewInfoConfig
		want string
	}{
		{name: "nil config returns empty string", cfg: nil, want: ""},
		{name: "stdin with name", cfg: &ReviewInfoConfig{Stdin: true, StdinName: "patch.diff"}, want: "stdin: patch.diff"},
		{name: "stdin without name", cfg: &ReviewInfoConfig{Stdin: true}, want: "stdin scratch buffer"},
		{name: "all-files", cfg: &ReviewInfoConfig{AllFiles: true}, want: "all tracked files"},
		{name: "staged", cfg: &ReviewInfoConfig{Staged: true}, want: "staged changes"},
		{name: "working tree", cfg: &ReviewInfoConfig{}, want: "working tree changes"},
		{name: "ref range", cfg: &ReviewInfoConfig{Ref: "main..feature"}, want: "ref range: main..feature"},
		{name: "single ref", cfg: &ReviewInfoConfig{Ref: "HEAD~3"}, want: "changes against HEAD~3"},
		{name: "no-VCS file-only review", cfg: &ReviewInfoConfig{VCS: "none"}, want: "standalone files"},
		{name: "compare mode", cfg: &ReviewInfoConfig{Compare: true, VCS: "none"}, want: "two-file diff"},
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
		m.review.cfg = &ReviewInfoConfig{VCS: "git"}
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
		m.review.cfg = &ReviewInfoConfig{AllFiles: true, VCS: "git"}
		m.review.entries = make([]diff.FileEntry, 12)
		got := m.reviewFooterText()
		assert.Contains(t, got, "12 tracked files")
		assert.NotContains(t, got, "+0/-0", "lines segment must be hidden in --all-files mode")
		assert.NotContains(t, got, "loading")
	})

	t.Run("stdin mode drops vcs segment (it's already in header)", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = &ReviewInfoConfig{Stdin: true, VCS: "stdin"}
		m.review.entries = make([]diff.FileEntry, 1)
		m.review.statsLoaded = true
		got := m.reviewFooterText()
		assert.NotContains(t, got, "stdin", "vcs segment is suppressed when it would just say 'stdin'")
	})

	t.Run("empty config returns empty", func(t *testing.T) {
		m := testModel(nil, nil)
		m.review.cfg = nil
		assert.Empty(t, m.reviewFooterText())
	})

	t.Run("disabled config with files loaded still returns empty", func(t *testing.T) {
		// Enabled=false is the off-switch for the entire review-info subsystem;
		// once files load the footer must STILL stay empty so it cannot get
		// stuck on "loading…" (triggerReviewStats short-circuits when disabled,
		// so stats never resolve in this state).
		m := testModel(nil, nil)
		m.review.cfg = nil
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
		ReviewInfo: &ReviewInfoConfig{},
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
		m.review.cfg = nil
		assert.Empty(t, m.review.descriptionHighlighted)
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
		}, ModelConfig{ReviewInfo: &ReviewInfoConfig{Description: "# Heading\n\nbody text\n\n```go\ncode block\n```"}})

		got := m.review.descriptionHighlighted
		assert.True(t, called, "highlighter must be invoked for non-empty description")
		assert.Equal(t, "description.md", seenFile, "filename hint must be .md so chroma picks the markdown lexer")
		assert.Equal(t, 7, seenLineCount, "line count must match raw newline-split")
		assert.Contains(t, got, "[hl]# Heading")
		assert.Contains(t, got, "[hl]```go")
	})

	t.Run("disabled highlighter falls back to sanitized plain text", func(t *testing.T) {
		m := testNewModel(t, &mocks.RendererMock{}, annotation.NewStore(), &mocks.SyntaxHighlighterMock{
			HighlightLinesFunc: func(_ string, _ []diff.DiffLine) []string {
				return nil
			},
			SetStyleFunc:  func(string) bool { return true },
			StyleNameFunc: func() string { return "test" },
		}, ModelConfig{ReviewInfo: &ReviewInfoConfig{Description: "plain\ntext\x1b[31m"}})

		assert.Equal(t, "plain\ntext", m.review.descriptionHighlighted)
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
		}, ModelConfig{ReviewInfo: &ReviewInfoConfig{Description: "agent prose"}})

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
		}, ModelConfig{ReviewInfo: &ReviewInfoConfig{
			// embed: ESC, BEL, NUL, CRLF, lone CR, plus benign tab + newline
			Description: "safe\x1b[31mred\x1b[0m\a\x00line\r\nwindows\rmac\tindented\nend",
		}})

		got := m.review.descriptionHighlighted
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
