// Package ui implements the bubbletea TUI for revdiff — a diff reviewer with inline annotations.
//
// The package centers on a single [Model] struct that implements bubbletea's Model interface.
// Model methods are split across multiple files by concern area, all operating on the same struct:
//
//   - model.go — struct definition, [NewModel], Init, Update, handleKey dispatch, view toggles
//   - view.go — View rendering, status bar, ANSI color helpers
//   - handlers.go — modal key handlers: help overlay, enter/esc dispatch, discard confirmation,
//     filter toggle, mark-reviewed, file-or-search navigation
//   - loaders.go — async file/blame loading (tea.Cmd producers), loaded-message handlers,
//     data preparation (stats, filter, skip-dividers)
//   - diffview.go — diff content rendering: line styling, gutters (line numbers, blame),
//     syntax-highlight integration, search-match highlighting, annotation rendering within diff
//   - diffnav.go — navigation dispatchers for diff/tree/TOC panes, cursor movement,
//     hunk navigation (including cross-file), viewport synchronization, horizontal scroll
//   - collapsed.go — collapsed diff mode: hides removed lines, shows modified markers,
//     per-hunk expansion, delete-only placeholders
//   - filetree.go — file tree sidebar component with cursor, scroll, and selection
//   - annotate.go — annotation input lifecycle: start, save, cancel, delete (line and file level),
//     cursor-viewport coordination, annotation key map
//   - annotlist.go — annotation list overlay for cross-file annotation browsing
//   - mdtoc.go — markdown table-of-contents sidebar for single-file markdown review
//   - search.go — incremental search: input handling, match computation, navigation
//   - worddiff.go — intra-line word-diff: tokenizer, LCS, line pairing, range computation
//   - colorutil.go — HSL color utilities for auto-deriving word-diff background colors
//   - styles.go — lipgloss style definitions, theme color integration
//
// The key interfaces consumed by Model are [Renderer] (provides changed files and diffs),
// [SyntaxHighlighter] (provides ANSI-highlighted lines), and [Blamer] (provides git blame data).
// All three are defined in this package and implemented externally.
package ui
