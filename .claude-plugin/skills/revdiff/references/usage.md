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
revdiff --only=/tmp/plan.md          # review a file outside a git repo (context-only)
revdiff --only=docs/notes.txt        # review a file with no git changes (context-only)
```

## Single-File Mode

When a diff contains exactly one file, revdiff automatically hides the file tree pane and gives full terminal width to the diff view. Pane-switching keys (`Tab`, `h/l`, `n/p`, `f`) become no-ops, except when markdown TOC is active (see below). Search navigation (`n`/`N`) still works normally.

## Markdown TOC Navigation

When reviewing a single markdown file in context-only mode (e.g., `revdiff --only=README.md`), a table-of-contents pane appears on the left listing all markdown headers with indentation by level. Use `Tab` to switch between TOC and diff, `j`/`k` to navigate headers, `n`/`p` to jump to next/prev header from either pane, `Enter` to jump to a header. The TOC highlights the current section as you scroll. Headers inside fenced code blocks are excluded.

## Context-Only File Review

When `--only` specifies a file that has no git changes (or when no git repo exists), revdiff shows the file in context-only mode: all lines displayed without `+`/`-` markers, with full annotation and syntax highlighting support.

- **Inside a git repo**: `--only` files not in the diff are read from disk alongside changed files
- **Outside a git repo**: `--only` is required; files are read directly from disk

## Key Bindings

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` | Switch between file tree and diff pane |
| left/right | Horizontal scroll in diff pane |
| `Tab` | Switch between file tree and diff pane |
| `PgDown/PgUp` | Page scroll in file tree and diff pane |
| `Ctrl+d/Ctrl+u` | Page scroll in file tree and diff pane |
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
| `Esc` | Cancel annotation input |

**View:**

| Key | Action |
|-----|--------|
| `v` | Toggle collapsed diff mode (shows final text with change markers) |
| `w` | Toggle word wrap (long lines wrap with `↪` continuation markers) |
| `t` | Toggle tree/TOC pane visibility (gives diff full terminal width) |
| `.` | Expand/collapse individual hunk under cursor (collapsed mode only) |
| `f` | Toggle filter: all files / annotated only |
| `?` | Toggle help overlay showing all keybindings |
| `q` | Quit, output annotations to stdout |
| `Q` | Discard all annotations and quit (confirms if annotations exist) |

## Output Format

On quit, revdiff outputs annotations to stdout:

```
## handler.go (file-level)
consider splitting this file into smaller modules

## handler.go:43 (+)
use errors.Is() instead of direct comparison

## store.go:18 (-)
don't remove this validation
```

Each annotation block: `## filename:line (type)` where type is `(+)` added, `(-)` removed, or `(file-level)`.

Use `--output` / `-o` flag to write annotations to a file instead of stdout.
