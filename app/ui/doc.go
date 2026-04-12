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
//   - annotate.go — annotation input lifecycle: start, save, cancel, delete (line and file level),
//     cursor-viewport coordination, annotation key map
//   - annotlist.go — annotation list overlay for cross-file annotation browsing
//   - search.go — incremental search: input handling, match computation, navigation
//
// Intra-line word-diff algorithms and the shared highlight marker insertion engine live
// in the [worddiff] sub-package (app/ui/worddiff/). It owns the tokenizer, LCS algorithm,
// line pairing, similarity gate, and ANSI-aware highlight marker insertion used by both
// word-diff and search highlighting. Model holds the worddiff type through a consumer-side
// interface (wordDiffer) defined in model.go; concrete *worddiff.Differ is injected via
// ModelConfig.WordDiffer wired in app/main.go.
//
// Color and style management lives in the [style] sub-package (app/ui/style/).
// It owns all hex-to-ANSI conversion, lipgloss style construction, SGR state tracking,
// and HSL color math. Model holds style types through consumer-side interfaces
// (styleResolver, styleRenderer, sgrProcessor) defined in model.go; concrete
// implementations live in the style sub-package.
//
// Left-pane navigation components live in the [sidepane] sub-package (app/ui/sidepane/).
// It owns the file tree (FileTree) and markdown table-of-contents (TOC) types,
// including cursor/offset management, entry parsing, and rendering logic.
// Model holds sidepane types through consumer-side interfaces (FileTreeComponent,
// TOCComponent) defined in model.go; concrete construction is injected via
// ModelConfig.NewFileTree and ModelConfig.ParseTOC factory closures wired in app/main.go.
//
// The key interfaces consumed by Model are [Renderer] (provides changed files and diffs),
// [SyntaxHighlighter] (provides ANSI-highlighted lines), and [Blamer] (provides blame data).
// All three are defined in this package and implemented externally.
package ui
