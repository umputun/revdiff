---
worth: later
where: app/ui/view.go:statusBarText
added: 2026-11-02
---
# status bar right side never degrades on narrow panes

`statusBarText` reserves the whole right block (`minRight`) and the degradation ladder only drops
left-side segments; `statusModeIcons` always shows every icon. When the right block alone is wider than
the pane, `Width(...).Render` wraps the status line to two rows while `statusBarHeight()` still returns 1,
so the frame is one row taller than the terminal and mouse rows are off by one.

On master it already overflows at 44 columns or less, and much wider once annotation or reviewed counts
are present (about 62 columns with two annotations). Each new mode icon moves the threshold up by two
cells; the fixture in `TestModel_StatusBarFilenameTruncationWideChars` has been widened for that twice
(616ebd0 for the word-diff icon). Surfaced reviewing PR #425, whose new icon would push it to 46.

Fix: let the right side degrade when the minimal left side still does not fit (drop or compact inactive
icons first), then move the test fixture back to a narrow width.
