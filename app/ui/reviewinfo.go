package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/review"
	"github.com/umputun/revdiff/app/ui/overlay"
)

// setReviewEntries records the per-file portion of the review-info summary
// (status histogram, snapshot for stats) at file-load time. Aggregate line
// stats are NOT computed here — they are loaded lazily on the first time the
// user opens the review-info overlay (see triggerReviewStats).
func (m *Model) setReviewEntries(entries []diff.FileEntry) {
	m.review.statusCounts = make(map[diff.FileStatus]int)
	for _, e := range entries {
		if e.Status != "" {
			m.review.statusCounts[e.Status]++
		}
	}
	// reset aggregate-line state — a reload invalidates any prior fetch.
	// Always snapshot the entries slice: the stats fetch iterates it, and the
	// footer/header read len() from the same field. Sharing the caller's
	// backing array would let a later sort/append/truncate change what we see
	// through this header. The slice is small (per-file entries) so the copy
	// is cheap, and a single code path avoids drift between stats-on and
	// stats-off modes.
	m.review.entries = append([]diff.FileEntry(nil), entries...)
	m.review.adds = 0
	m.review.removes = 0
	m.review.partial = false
	m.review.err = nil
	m.review.statsLoaded = false
	m.review.statsRequested = false
	m.review.statsLoadSeq++ // invalidate any in-flight stats fetch
}

// triggerReviewStats schedules a lazy aggregate-stats fetch the first time the
// review-info overlay is opened in this load generation. Subsequent opens read
// from cache; a reload (which calls setReviewEntries again) flips
// statsRequested back to false so the next open re-fetches. Returns nil when
// stats are not applicable (AllFiles or focused-test mode where the config is
// nil), already in flight, or opened before the file list has loaded. In the
// last case statsRequested remains true so handleFilesLoaded can schedule the
// deferred fetch once entries are known.
func (m *Model) triggerReviewStats() tea.Cmd {
	if m.review.statsRequested {
		return nil
	}
	if m.review.cfg == nil || m.review.cfg.AllFiles {
		m.review.statsLoaded = true
		m.review.statsRequested = true
		return nil
	}
	if len(m.review.entries) == 0 {
		m.review.statsRequested = true
		if m.filesLoaded {
			m.review.statsLoaded = true
		}
		return nil
	}
	m.review.statsRequested = true
	return m.loadReviewStats(m.review.entries)
}

// loadReviewStats produces the tea.Cmd that runs the aggregate-stats fetch.
// The actual computation lives in app/review.ComputeStats; this method only
// owns the bubbletea plumbing (seq capture, message wrapping) so the UI
// package stays out of the diff-fetching / filesystem business.
//
// The normalized Staged value from the review-info config is used so
// stats-loading honors the same hg/jj fallback as the popup header/footer.
// Reading m.cfg.staged directly would diverge from what the user sees.
func (m Model) loadReviewStats(entries []diff.FileEntry) tea.Cmd {
	if m.review.cfg == nil || m.review.cfg.AllFiles || len(entries) == 0 {
		return nil
	}
	seq := m.review.statsLoadSeq
	req := review.StatsRequest{
		Differ:  m.diffRenderer,
		Ref:     m.cfg.ref,
		Staged:  m.review.cfg.Staged,
		WorkDir: m.cfg.workDir,
		Entries: entries,
	}
	return func() tea.Msg {
		return reviewStatsLoadedMsg{
			seq:   seq,
			Stats: review.ComputeStats(req),
		}
	}
}

func (m Model) handleReviewStatsLoaded(msg reviewStatsLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.review.statsLoadSeq {
		return m, nil
	}
	m.review.adds = msg.Adds
	m.review.removes = msg.Removes
	m.review.partial = msg.Partial
	m.review.err = msg.Err
	m.review.statsLoaded = true
	// the info popup may be open with a stale "loading…" footer; push a fresh
	// spec so the user sees totals appear inline without dismiss/reopen.
	m.refreshInfoOverlay()
	return m, nil
}

// refreshInfoOverlay re-builds the info-popup spec and pushes it to the
// overlay when the popup is currently visible. Must be called after any
// state change that affects the rendered spec (review-stats load, commits
// load) so async results land in an open popup. UpdateInfo is a no-op
// when the active overlay is anything else, so the call site does not need
// to gate on Kind().
func (m *Model) refreshInfoOverlay() {
	if !m.overlay.Active() {
		return
	}
	m.overlay.UpdateInfo(m.buildInfoSpec())
}

// buildInfoSpec assembles the unified info-popup spec from current model
// state. The description prose (issue #130) is run through the same chroma
// markdown-highlighting path used for in-diff .md files so headings and
// fenced code blocks render with theme-consistent color, no extra deps.
//
// Header text (rendered in the popup's top border) and footer text (bottom
// border) carry the most-glanced session metadata — mode/scope summary on
// top, aggregate stats on the bottom — so the popup body stays compact and
// the "no commits" case still presents useful info via the borders alone.
//
// The Rows slice carries only the residual session details that don't fit
// in the borders (filters, --compact flags, workDir root); see reviewRows.
// The commits section is gated by m.commits.applicable and reads from the
// eagerly-fetched commit cache; CommitsLoaded=false signals the overlay to
// render a "loading commits…" line inside the section.
func (m Model) buildInfoSpec() overlay.InfoSpec {
	return overlay.InfoSpec{
		HeaderText:        m.reviewHeaderText(),
		FooterText:        m.reviewFooterText(),
		Description:       m.review.descriptionHighlighted,
		Rows:              m.reviewRows(),
		Commits:           m.commits.list,
		CommitsApplicable: m.commits.applicable && m.commits.source != nil,
		CommitsLoaded:     m.commits.loaded,
		Truncated:         m.commits.truncated,
		CommitsErr:        m.commits.err,
	}
}

// precomputeDescriptionHighlight runs the description prose through the
// project's markdown-highlighting pipeline (the same path used for .md files
// in the diff view, so the colors match the rest of the TUI). Empty
// description short-circuits to "" so the description section stays hidden.
// Newlines are preserved as paragraph breaks; the highlighter operates
// per-line. Raw input is first run through diff.SanitizeCommitText to strip
// ANSI CSI sequences, stray ESC bytes, C0/DEL/C1 controls (including CR), and
// invalid UTF-8 — without this, a malicious --description-file could inject
// escape sequences that the highlighter would pass through verbatim. CR is
// dropped by the sanitizer, so a Windows CRLF reduces to LF and lone CR
// disappears; lines inside the description never carry stray \r bytes.
// Called once from NewModel; the result is stored on reviewInfoState.
func precomputeDescriptionHighlight(h SyntaxHighlighter, desc string) string {
	if desc == "" || h == nil {
		return ""
	}
	desc = diff.SanitizeCommitText(desc)
	rawLines := strings.Split(desc, "\n")
	diffLines := make([]diff.DiffLine, len(rawLines))
	for i, line := range rawLines {
		diffLines[i] = diff.DiffLine{Content: line, ChangeType: diff.ChangeContext}
	}
	highlighted := h.HighlightLines("description.md", diffLines)
	if highlighted == nil {
		return strings.Join(rawLines, "\n")
	}
	return strings.Join(highlighted, "\n")
}

// reviewHeaderText returns the mode-language caption used as the popup's
// top-border title. Phrasing matches the language the user sees in CLI
// help so the title is unambiguous even when the popup body is otherwise
// minimal:
//
//	working tree changes        — uncommitted edits in the work tree
//	staged changes              — --staged
//	changes against HEAD~3      — single ref
//	ref range: main..feature    — explicit range syntax
//	stdin: patch.diff           — --stdin <name>
//	stdin scratch buffer        — --stdin without a name
//	two-file diff               — --compare-old/--compare-new
//	all tracked files           — --all-files
//	standalone files            — --only without a VCS (file-only review)
//
// Returns "" in focused-test mode (cfg == nil); the overlay then falls back
// to a plain " info " title.
func (m Model) reviewHeaderText() string {
	cfg := m.review.cfg
	if cfg == nil {
		return ""
	}
	switch {
	case cfg.Stdin:
		if cfg.StdinName != "" {
			return "stdin: " + cfg.StdinName
		}
		return "stdin scratch buffer"
	case cfg.Compare:
		return "two-file diff"
	case cfg.AllFiles:
		return "all tracked files"
	case cfg.Staged:
		return "staged changes"
	case cfg.VCS == "none":
		// no-VCS file-only review: there is no working tree or ref scope, so
		// the default "working tree changes" phrasing would mislead.
		return "standalone files"
	case cfg.Ref == "":
		return "working tree changes"
	case strings.Contains(cfg.Ref, ".."):
		return "ref range: " + cfg.Ref
	default:
		return "changes against " + cfg.Ref
	}
}

// reviewFooterText returns the aggregate-stats summary rendered in the
// popup's bottom border. Pieces are joined with " · " in this order:
// file count, line totals, file-status histogram, vcs name. Pieces that
// don't apply in the current mode (e.g. line totals while in --all-files
// mode) are dropped silently. Empty when m.review.cfg is nil regardless of
// file count, matching reviewHeaderText and triggerReviewStats: cfg == nil
// is the off-switch for the entire review-info subsystem, so every derived
// render path must honor it consistently or "loading…" can stick on the
// footer forever (triggerReviewStats short-circuits there).
func (m Model) reviewFooterText() string {
	if m.review.cfg == nil {
		return ""
	}
	parts := []string{m.reviewFilesText()}
	if !m.review.cfg.AllFiles {
		parts = append(parts, m.reviewLinesText())
	}
	if s := m.reviewStatusText(); s != "" {
		parts = append(parts, s)
	}
	if v := m.review.cfg.VCS; v != "" && v != "stdin" && v != "none" {
		parts = append(parts, v)
	}
	return strings.Join(parts, " · ")
}

// reviewRows builds the residual detail rows shown in the popup body, after
// the most common metadata has been hoisted to the popup's borders. Only rows
// that genuinely add information beyond "what mode am I in?" and "how big is
// this review?" appear here: the active filter flags (--only, --include,
// --exclude), the --compact options when set, and the working-directory
// root. Common case (no filters, no compact, no workDir): one row for root,
// or none at all in stdin mode where workDir is empty.
func (m Model) reviewRows() []overlay.InfoRow {
	cfg := m.review.cfg
	if cfg == nil {
		return nil
	}
	var rows []overlay.InfoRow
	if f := m.reviewListFlag(cfg.Only); f != "" {
		rows = append(rows, overlay.InfoRow{Label: "only", Value: f})
	}
	if f := m.reviewListFlag(cfg.Include); f != "" {
		rows = append(rows, overlay.InfoRow{Label: "include", Value: f})
	}
	if f := m.reviewListFlag(cfg.Exclude); f != "" {
		rows = append(rows, overlay.InfoRow{Label: "exclude", Value: f})
	}
	if cfg.Compact {
		rows = append(rows, overlay.InfoRow{Label: "options", Value: fmt.Sprintf("--compact --compact-context=%d", cfg.CompactContext)})
	}
	if m.review.err != nil {
		rows = append(rows, overlay.InfoRow{Label: "stats", Value: "unavailable: " + m.review.err.Error()})
	}
	if cfg.WorkDir != "" {
		rows = append(rows, overlay.InfoRow{
			Label:       "project",
			Value:       filepath.Base(cfg.WorkDir),
			MutedSuffix: cfg.WorkDir,
		})
	}
	return rows
}

func (m Model) reviewFilesText() string {
	n := len(m.review.entries)
	allFiles := m.review.cfg != nil && m.review.cfg.AllFiles
	if n == 1 {
		if allFiles {
			return "1 tracked file"
		}
		return "1 file"
	}
	if allFiles {
		return fmt.Sprintf("%d tracked files", n)
	}
	return fmt.Sprintf("%d files", n)
}

func (m Model) reviewLinesText() string {
	switch {
	case m.review.cfg != nil && m.review.cfg.AllFiles:
		return "not calculated in all-files mode"
	case !m.review.statsLoaded:
		return "loading…"
	case m.review.err != nil:
		return "stats unavailable"
	case m.review.adds == 0 && m.review.removes == 0:
		if m.review.partial {
			return "no changed lines (partial)"
		}
		return "no changed lines"
	case m.review.partial:
		return fmt.Sprintf("+%d/-%d (partial)", m.review.adds, m.review.removes)
	default:
		return fmt.Sprintf("+%d/-%d", m.review.adds, m.review.removes)
	}
}

func (m Model) reviewStatusText() string {
	if len(m.review.statusCounts) == 0 {
		return ""
	}
	order := []diff.FileStatus{diff.FileAdded, diff.FileModified, diff.FileDeleted, diff.FileRenamed, diff.FileUntracked}
	seen := make(map[diff.FileStatus]bool, len(order))
	var parts []string
	for _, st := range order {
		seen[st] = true
		if n := m.review.statusCounts[st]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s%d", st, n))
		}
	}
	var rest []string
	for st, n := range m.review.statusCounts {
		if !seen[st] && n > 0 {
			rest = append(rest, fmt.Sprintf("%s%d", st, n))
		}
	}
	sort.Strings(rest)
	parts = append(parts, rest...)
	return strings.Join(parts, " ")
}

func (m Model) reviewListFlag(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return strings.Join(values, ", ")
}
