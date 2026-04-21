// Package ui implements the bubbletea TUI for revdiff — a diff reviewer with inline annotations.
//
// The package centers on a single [Model] struct that implements bubbletea's Model interface.
// Model methods are split across multiple files by concern area, all operating on the same struct:
//
//   - model.go — struct definition, sub-state structs, [NewModel], Init, Update, handleKey
//     dispatch, view toggles, consumer-side interfaces
//   - view.go — View rendering, status bar, ANSI color helpers
//   - handlers.go — modal key handlers: enter/esc dispatch, discard confirmation,
//     filter toggle, mark-reviewed, file-or-search navigation, help spec building
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
//   - annotlist.go — annotation list spec building, cross-file jump logic
//   - editor.go — external $EDITOR handoff for multi-line annotations: [ExternalEditor]
//     interface, [editorFinishedMsg], openEditor (wraps the injected editor in
//     tea.ExecProcess and captures target state at spawn time), handleEditorFinished
//     (save / cancel / error-preserve routing)
//   - themeselect.go — theme selector operations: open, preview/confirm/cancel, apply theme
//     (delegates to injected [ThemeCatalog] for discovery and persistence)
//   - search.go — incremental search: input handling, match computation, navigation
//
// Model mutable state is organized into explicit sub-structs by concern:
//
//   - modelConfigState (m.cfg) — immutable session config (ref, staged, noColors, tabSpaces, etc.)
//   - layoutState (m.layout) — viewport, pane focus, dimensions, scroll offset
//   - loadedFileState (m.file) — current file's lines, highlighted content, intra-line ranges,
//     blame data, line numbering, TOC, and single-file flag. groups parallel arrays and derived
//     metadata to make the synchronization invariant explicit
//   - modeState (m.modes) — user-togglable view modes (wrap, collapsed, lineNumbers, wordDiff,
//     showBlame, showUntracked, compact, compactContext)
//   - navigationState (m.nav) — diff cursor position and pending hunk jump
//   - searchState (m.search) — search input, term, matches, cursor, match set
//   - annotationState (m.annot) — annotation input lifecycle (annotating, file-level, cursor state)
//
// Methods remain on Model — sub-structs group state for clarity, not to create mini-models.
//
// Theme discovery and persistence are accessed through the [ThemeCatalog] interface defined
// in model.go. This package does not import app/theme or app/fsutil — the concrete adapter
// is wired in app/themes.go, composing theme.Catalog with config file persistence.
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
// Layered popup UI lives in the [overlay] sub-package (app/ui/overlay/).
// It owns help, annotation list, and theme selector overlays — all popup state
// (cursor, offset, filter text, items, active kind), rendering (box layout,
// item formatting, border title injection, ANSI-aware centered compositing),
// and key dispatch (navigation, confirm, cancel, filter input). A Manager
// coordinator enforces mutual exclusivity (one overlay at a time) and routes
// key events and compose calls to the active overlay. Model holds the Manager
// through a consumer-side interface (overlayManager) defined in model.go;
// concrete *overlay.Manager is injected via ModelConfig.Overlay wired in app/main.go.
//
// The key interfaces consumed by Model are [Renderer] (provides changed files and diffs),
// [SyntaxHighlighter] (provides ANSI-highlighted lines), [Blamer] (provides blame data),
// [ThemeCatalog] (provides theme discovery, resolution, and persistence), and
// [ExternalEditor] (provides $EDITOR invocation for multi-line annotation input).
// All are defined in this package and implemented externally.
package ui
