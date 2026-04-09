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
revdiff --all-files                  # browse all git-tracked files in a project
revdiff --all-files --exclude vendor # browse all files, excluding vendor directory
revdiff main --exclude vendor        # diff against main, excluding vendor
revdiff --only=/tmp/plan.md          # review a file outside a git repo (context-only)
revdiff --only=docs/notes.txt        # review a file with no git changes (context-only)
printf '# Plan\n\nBody\n' | revdiff --stdin --stdin-name plan.md  # review piped text as markdown
some-command | revdiff --stdin --output /tmp/annotations.txt      # annotate generated output
```

## Single-File Mode

When a diff contains exactly one file, revdiff automatically hides the file tree pane and gives full terminal width to the diff view. Pane-switching keys (`Tab`, `h/l`, `n/p`, `f`) become no-ops, except when markdown TOC is active (see below). Search navigation (`n`/`N`) still works normally.

## Markdown TOC Navigation

When reviewing a single markdown file in context-only mode (e.g., `revdiff --only=README.md`), a table-of-contents pane appears on the left listing all markdown headers with indentation by level. Use `Tab` to switch between TOC and diff, `j`/`k` to navigate headers, `n`/`p` to jump to next/prev header from either pane, `Enter` to jump to a header. The TOC highlights the current section as you scroll. Headers inside fenced code blocks are excluded.

## All-Files Mode

Use `--all-files` (`-A`) to browse all git-tracked files, not just diffs. Turns revdiff into a general-purpose code annotation tool. All files shown in context-only mode with full annotation and syntax highlighting support.

- Requires a git repository (uses `git ls-files` for file discovery)
- Mutually exclusive with refs, `--staged`, and `--only`
- Combine with `--exclude` (`-X`) to filter out paths by prefix matching

```bash
revdiff --all-files                          # all tracked files
revdiff --all-files --exclude vendor         # skip vendor/
revdiff --all-files --exclude vendor --exclude mocks  # skip both
revdiff main --exclude vendor                # normal diff, excluding vendor
```

`--exclude` can be persisted in config file (`exclude = vendor`) or via env var (`REVDIFF_EXCLUDE=vendor,mocks`).

## Context-Only File Review

When `--only` specifies a file that has no git changes (or when no git repo exists), revdiff shows the file in context-only mode: all lines displayed without `+`/`-` markers, with full annotation and syntax highlighting support.

- **Inside a git repo**: `--only` files not in the diff are read from disk alongside changed files
- **Outside a git repo**: `--only` is required; files are read directly from disk

## Scratch-Buffer Review

Use `--stdin` to review arbitrary piped or redirected text as one synthetic file. All lines are treated as context, so single-file mode, inline annotations, file-level notes, search, wrap, collapsed mode, and structured output all work unchanged.

- `--stdin` is explicit and requires piped or redirected input
- `--stdin-name` sets the synthetic filename used in annotations and syntax highlighting
- `--stdin` conflicts with refs, `--staged`, `--only`, `--all-files`, and `--exclude`

## Key Bindings

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` | Switch between file tree and diff pane |
| left/right | Horizontal scroll in diff pane |
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
| `Esc` | Cancel annotation input |

**View:**

| Key | Action |
|-----|--------|
| `v` | Toggle collapsed diff mode (shows final text with change markers) |
| `w` | Toggle word wrap (long lines wrap with `↪` continuation markers) |
| `t` | Toggle tree/TOC pane visibility (gives diff full terminal width) |
| `L` | Toggle line numbers (side-by-side old/new numbers in gutter) |
| `B` | Toggle git blame gutter (author name + commit age per line) |
| `.` | Expand/collapse individual hunk under cursor (collapsed mode only) |
| `T` | Open theme selector with live preview |
| `f` | Toggle filter: all files / annotated only |
| `?` | Toggle help overlay showing all keybindings |
| `q` | Quit, output annotations to stdout |
| `Q` | Discard all annotations and quit (confirms if annotations exist) |

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

Use `--output` / `-o` flag to write annotations to a file instead of stdout.

## Review History

When you quit with annotations (`q`), revdiff automatically saves a copy of the review session to `~/.config/revdiff/history/<repo-name>/<timestamp>.md`. This is a safety net — if annotations are lost (process crash, agent fails to capture stdout), the history file preserves them.

Each history file contains:
- Header with path, git refs, and commit hash
- Full annotation output (same format as stdout)
- Raw git diff for annotated files only

History auto-save is always on and silent — errors are logged to stderr, never fail the process. No history is saved on discard quit (`Q`) or when there are no annotations. For `--stdin` mode, files are saved under `stdin/` subdirectory; for `--only` without git, the parent directory name is used instead of a repo name.

Override the history directory with `--history-dir`, `REVDIFF_HISTORY_DIR` env var, or `history-dir` in the config file.
