# Usage

```
revdiff [OPTIONS] [base] [against]
```

## Examples

```bash
revdiff              # review uncommitted changes
revdiff main         # review changes against a branch
revdiff --staged     # review staged changes
revdiff HEAD~1       # review last commit
revdiff main feature # diff between two refs
revdiff main..feature  # same as above, git dot-dot syntax
revdiff main...feature # changes since feature diverged from main
revdiff --only=model.go              # review only files matching model.go
revdiff --only=ui/model.go --only=README.md  # review specific files
revdiff --all-files                  # browse all tracked files (git or jj)
revdiff --all-files --exclude vendor # browse all files, excluding vendor directory
revdiff --include src                # include only src/ files
revdiff --include src --exclude src/vendor  # include src/ but exclude src/vendor/
revdiff main --exclude vendor        # diff against main, excluding vendor
revdiff --only=/tmp/plan.md          # review a file outside a repo (context-only)
revdiff --only=docs/notes.txt        # review a file with no VCS changes (context-only)
printf '# Plan\n\nBody\n' | revdiff --stdin --stdin-name plan.md  # review piped text as markdown
some-command | revdiff --stdin --output /tmp/annotations.txt      # annotate generated output
```

## Single-File Mode

When a diff contains exactly one file, revdiff automatically hides the file tree pane and gives full terminal width to the diff view. Pane-switching keys (`Tab`, `h/l`, `n/p`, `f`) become no-ops, except when markdown TOC is active (see below). Search navigation (`n`/`N`) still works normally.

## Markdown TOC Navigation

When reviewing a single markdown file in context-only mode (e.g., `revdiff --only=README.md`), a table-of-contents pane appears on the left listing all markdown headers with indentation by level. Use `Tab` to switch between TOC and diff, `j`/`k` to navigate headers, `n`/`p` to jump to next/prev header from either pane, `Enter` to jump to a header. The TOC highlights the current section as you scroll. Headers inside fenced code blocks are excluded.

## All-Files Mode

Use `--all-files` (`-A`) to browse all tracked files, not just diffs. Turns revdiff into a general-purpose code annotation tool. All files shown in context-only mode with full annotation and syntax highlighting support.

- Requires a git or jj repository (uses `git ls-files` / `jj file list` for file discovery; Mercurial is not supported)
- Mutually exclusive with refs, `--staged`, and `--only`
- Combine with `--include` (`-I`) to narrow to specific paths and `--exclude` (`-X`) to filter out unwanted paths

```bash
revdiff --all-files                          # all tracked files
revdiff --all-files --include src            # only src/ files
revdiff --all-files --include src --exclude src/vendor  # src/ minus src/vendor/
revdiff --all-files --exclude vendor         # skip vendor/
revdiff --all-files --exclude vendor --exclude mocks  # skip both
revdiff main --exclude vendor                # normal diff, excluding vendor
```

`--include` and `--exclude` can be persisted in config file (`include = src`, `exclude = vendor`) or via env vars (`REVDIFF_INCLUDE=src`, `REVDIFF_EXCLUDE=vendor,mocks`). Include narrows first, then exclude removes from the included set.

## Context-Only File Review

When `--only` specifies a file that has no VCS changes (or when no repo exists), revdiff shows the file in context-only mode: all lines displayed without `+`/`-` markers, with full annotation and syntax highlighting support.

- **Inside a repo (git/hg/jj)**: `--only` files not in the diff are read from disk alongside changed files
- **Outside a repo**: `--only` is required; files are read directly from disk

## Scratch-Buffer Review

Use `--stdin` to review arbitrary piped or redirected text as one synthetic file. All lines are treated as context, so single-file mode, inline annotations, file-level notes, search, wrap, collapsed mode, and structured output all work unchanged.

- `--stdin` is explicit and requires piped or redirected input
- `--stdin-name` sets the synthetic filename used in annotations and syntax highlighting
- `--stdin` conflicts with refs, `--staged`, `--only`, `--all-files`, `--include`, and `--exclude`

## Key Bindings

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` | Switch between file tree and diff pane |
| left/right | Horizontal scroll in diff pane (truncated lines show `«` / `»` overflow indicators at the edges) |
| `Tab` | Switch between file tree and diff pane |
| `PgDown/PgUp` | Page scroll in file tree and diff pane |
| `Ctrl+d/Ctrl+u` | Half-page scroll in file tree and diff pane |
| `Home/End` | Jump to first/last item |
| `Enter` | Switch to diff pane (tree) / start annotation (diff pane) |
| `n/p` | Next/previous changed file; next/prev header in markdown TOC mode (n = next match when search active) |
| `[` / `]` | Jump to previous/next change hunk in diff |

**Search:**

| Key | Action |
|-----|--------|
| `/` | Start search in diff pane |
| `n` | Next search match (overrides next file when search active) |
| `N` | Previous search match |
| `Esc` | Cancel search input / clear search results |

**Annotations:**

| Key | Action |
|-----|--------|
| `a` or `Enter` (diff pane) | Annotate current diff line |
| `A` | Add file-level annotation (stored at top of diff) |
| `@` | Toggle annotation list popup (navigate and jump to any annotation) |
| `d` | Delete annotation under cursor |
| `Ctrl+E` (during annotation input) | Open `$EDITOR` for multi-line annotation |
| `Esc` | Cancel annotation input |

While the annotation input is active, press `Ctrl+E` to hand off the current text to an external editor for multi-line comments. Editor resolution: `$EDITOR` → `$VISUAL` → `vi`. Values with arguments work (e.g. `EDITOR="code --wait"`). On editor save and quit, the full file contents (including newlines) become the annotation. Quitting the editor with an empty file cancels the annotation and preserves any previously stored note on that line. Multi-line annotations are rendered line-by-line in the diff view, shown flattened in the annotation list popup (`@`), and emitted with embedded newlines in the structured output.

**View:**

| Key | Action |
|-----|--------|
| `v` | Toggle collapsed diff mode (shows final text with change markers) |
| `C` | Toggle compact diff view (small context around changes, re-fetches current file) |
| `w` | Toggle word wrap (long lines wrap with `↪` continuation markers) |
| `t` | Toggle tree/TOC pane visibility (gives diff full terminal width) |
| `L` | Toggle line numbers (side-by-side old/new for diffs, single column for full-context files) |
| `B` | Toggle blame gutter (author name + commit age per line) |
| `W` | Toggle intra-line word-diff highlighting for paired add/remove lines |
| `.` | Expand/collapse individual hunk under cursor (collapsed mode only) |
| `T` | Open theme selector with live preview |
| `f` | Toggle filter: all files / annotated only |
| `?` | Toggle help overlay showing all keybindings |
| `i` | Toggle commit info popup (subject + body of commits in the current ref range; git/hg/jj only) |
| `R` | Reload diff from VCS (warns if annotations exist) |
| `q` | Quit, output annotations to stdout |
| `Q` | Discard all annotations and quit (confirms if annotations exist) |

## Status Bar Icons

The status bar shows a fixed row of mode indicators on the right side. All slots are always rendered — active modes use the status bar foreground color, inactive modes use muted gray, so the row occupies the same width regardless of what's toggled on.

| Icon | Toggle | Meaning |
|------|--------|---------|
| `▼` | `v` | Collapsed diff mode |
| `⊂` | `C` | Compact diff mode (small context around changes) |
| `◉` | `f` | Filter: annotated files only |
| `↩` | `w` | Word wrap mode |
| `≋` | `/` | Search active |
| `⊟` | `t` | Tree/TOC pane hidden (diff uses full width) |
| `#` | `L` | Line numbers visible in gutter |
| `b` | `B` | Blame gutter visible |
| `±` | `W` | Intra-line word-diff highlighting |
| `✓` | `Space` | Reviewed count (increments when a file is marked reviewed) |
| `∅` | `u` | Untracked files visible in tree |

On narrow terminals, the left-hand segments are dropped before the icons: search position first, then line and hunk info, then the filename truncates. The icon row on the right stays put.

## Mouse Support

revdiff enables mouse tracking by default so the scroll wheel and left-click work consistently across terminals.

- **Scroll wheel** — scrolls whichever pane the cursor is over (tree/TOC or diff). Three lines per notch.
- **Shift+scroll** — half-page scroll in whichever pane the cursor is over.
- **Left-click in the tree** — focuses the tree and selects/loads the clicked entry. Clicking a directory row moves the cursor but does not load a file.
- **Left-click in the diff** — focuses the diff and moves the cursor to the clicked line. Enables a "click, then `a`" annotation flow.
- **Left-click in the TOC pane** (single-file markdown) — focuses the TOC and selects the clicked header.

Horizontal wheel, right-click, middle-click, drag selection, and clicks on the status bar or diff header are intentionally ignored. Modal states (annotation input, search input, confirm discard, reload confirm, open overlay) swallow mouse events so they cannot interfere with text entry or popups.

**Text selection trade-off** — once mouse tracking is on, plain drag is captured by revdiff. For terminal-native text selection:

- **kitty**: hold `Ctrl+Shift` while dragging
- **iTerm2**: hold `Option` while dragging
- **most other terminals**: hold `Shift` while dragging

Because the tree pane is rendered alongside the diff on the same rows, multi-line Shift+drag will include tree content. For clean copies of diff text, use your terminal's block-select mode (Option+drag in iTerm2, Ctrl+Shift+drag in kitty) or run with `--no-mouse` to disable mouse capture entirely.

Opt out with `--no-mouse`, `REVDIFF_NO_MOUSE=true`, or `no-mouse = true` in the config file.

## Custom Keybindings

All keybindings can be customized via `~/.config/revdiff/keybindings` (override path with `--keys` or `REVDIFF_KEYS`).

```
# map <key> <action> — bind a key
# unmap <key> — remove a default binding
map x quit
unmap q
map ctrl+d half_page_down
```

Generate a template with all defaults: `revdiff --dump-keys > ~/.config/revdiff/keybindings`

See the [configuration reference](config.md) for the full list of available actions.

## Output Format

On quit, revdiff outputs annotations to stdout:

```
## handler.go (file-level)
consider splitting this file into smaller modules

## handler.go:43 (+)
use errors.Is() instead of direct comparison

## handler.go:43-67 (+)
refactor this hunk to reduce nesting

## store.go:18 (-)
don't remove this validation
```

Each annotation block: `## filename:line[-end] (type)` where type is `(+)` added, `(-)` removed, or `(file-level)`. The `-end` suffix is included when the annotation covers a line range.

When annotation text contains the keyword "hunk" (case-insensitive, whole word), the output header automatically expands to include the full hunk line range (e.g., `handler.go:43-67 (+)` instead of `handler.go:43 (+)`). This gives AI consumers the range context without any extra steps.

Comment body lines starting with `## ` (the record-header form) are prefixed with a single space on output so parsers that split on `## ` record headers cannot confuse a multi-line comment for a new record.

Use `--output` / `-o` flag to write annotations to a file instead of stdout.

## Review History

When you quit with annotations (`q`), revdiff automatically saves a copy of the review session to `~/.config/revdiff/history/<repo-name>/<timestamp>.md`. This is a safety net — if annotations are lost (process crash, agent fails to capture stdout), the history file preserves them.

Each history file contains:
- Header with path, refs, and (git only) a short commit hash
- Full annotation output (same format as stdout)
- Raw git diff for annotated files only

History auto-save is always on and silent — errors are logged to stderr, never fail the process. No history is saved on discard quit (`Q`) or when there are no annotations. For `--stdin` mode, files are saved under `stdin/` subdirectory; for `--only` without git, the parent directory name is used instead of a repo name.

Override the history directory with `--history-dir`, `REVDIFF_HISTORY_DIR` env var, or `history-dir` in the config file.
