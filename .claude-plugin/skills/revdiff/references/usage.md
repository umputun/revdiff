# Usage

```
revdiff [OPTIONS] [ref]
```

## Examples

```bash
revdiff              # review uncommitted changes
revdiff main         # review changes against a branch
revdiff --staged     # review staged changes
revdiff HEAD~1       # review last commit
```

## Single-File Mode

When a diff contains exactly one file, revdiff automatically hides the file tree pane and gives full terminal width to the diff view. Pane-switching keys (`Tab`, `h/l`, `n/p`, `f`) become no-ops. Search navigation (`n`/`N`) still works normally.

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
| `n/p` | Next/previous changed file (n = next match when search active) |
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
| `d` | Delete annotation under cursor |
| `Esc` | Cancel annotation input |

**View:**

| Key | Action |
|-----|--------|
| `v` | Toggle collapsed diff mode (shows final text with change markers) |
| `w` | Toggle word wrap (long lines wrap with `↪` continuation markers) |
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
