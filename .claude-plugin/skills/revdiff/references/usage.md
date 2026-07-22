# Usage

```
revdiff [OPTIONS] [base] [against]
```

## Examples

```bash
revdiff              # review uncommitted changes
revdiff main         # review changes against a branch
revdiff --staged     # review staged changes
revdiff --untracked  # show untracked files in the tree
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
revdiff --compare-old=/tmp/plan-old.md --compare-new=docs/plans/plan.md  # diff two arbitrary files (no VCS needed)
printf '# Plan\n\nBody\n' | revdiff --stdin --stdin-name plan.md  # review piped text as markdown
some-command | revdiff --stdin --output /tmp/annotations.txt      # annotate generated output
```

## Single-File Mode

When a diff contains exactly one file, revdiff automatically hides the file tree pane and gives full terminal width to the diff view. Pane-switching keys (`Tab`, `h/l`, `n/p`, `f`, `F`) become no-ops, except when markdown TOC is active (see below). Search navigation (`n`/`N`) still works normally.

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

## Review Description

When launching revdiff for a user (auto-open after a refactor, code review, etc.), include `--description` (or `--description-file=path.md`) to attach prose context to the review. The description appears at the top of the info popup (`i` key) and is rendered as markdown:

- Headings (`#`, `##`) for sections
- Code fences (```` ``` ````) for snippets and commands
- Body prose for the *why* — what the agent did, what the user should look at, open questions

Keep it concise (a few sentences to a small markdown doc). Long content is easier to manage in a file:

```bash
revdiff HEAD~3 --description="# Refactor auth middleware

Drop session-token storage to meet new compliance requirements.
See ticket SEC-441 for context."

# longer markdown lives in a file
revdiff HEAD~3 --description-file=/tmp/review-notes.md
```

`--description` and `--description-file` are mutually exclusive. Both are optional — omit when there's no useful context to add.

## Two-File Diff

Use `--compare-old=<path>` together with `--compare-new=<path>` to diff two arbitrary files on disk using `git diff --no-index`. No VCS repo needed — works anywhere `git` is installed.

- `--compare-old` and `--compare-new` must be used together; both are mutually exclusive with refs, `--staged`, `--only`, `--all-files`, `--stdin`, `--include`, `--exclude`, and `--annotations`
- All standard features work: word-diff, compact mode, syntax highlighting, scrollbar, and inline annotations

```bash
revdiff --compare-old=/tmp/plan-old.md --compare-new=docs/plans/plan.md
revdiff --compare-old=a.txt --compare-new=b.txt
```

## Scratch-Buffer Review

Use `--stdin` to review arbitrary piped or redirected text. revdiff sniffs the input for a git unified-diff signature: when a line beginning with `diff --git a/` is found near the start, the input is parsed as a real multi-file diff (one tree entry per file, with `+`/`-` markers, hunk navigation, word-diff, compact mode, per-file annotations). Otherwise the input is shown as a single context-only buffer — single-file mode, inline annotations, file-level notes, search, wrap, collapsed mode, and structured output all work unchanged.

- `--stdin` is explicit and requires piped or redirected input
- `--stdin-name` sets the synthetic filename used by the context-only buffer (ignored in multi-file diff mode, where real paths are shown)
- `--stdin` conflicts with refs, `--staged`, `--only`, `--all-files`, `--include`, and `--exclude`
- Any per-section parse failure falls the whole input back to raw-text mode so a malformed patch never silently drops files
- Input is capped at 64 MiB

Examples piping a real diff:
- `gh pr diff 123 | revdiff --stdin` — review a GitHub PR end-to-end
- `git format-patch -1 --stdout | revdiff --stdin` — review the latest commit as a multi-file diff

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
| `J/K` | Scroll diff viewport (works from either pane) |
| `Home/End` | Jump to first/last item |
| `Enter` | Switch to diff pane (tree) / start annotation (diff pane) |
| `n/p` | Next/previous changed file; next/prev header in markdown TOC mode (n = next match when search active) |
| `Ctrl+P` | Open the file picker (recalls previous history while the search prompt is active) |
| `[` / `]` | Jump to previous/next change hunk in diff |
| `e` | Open focused file in `$EDITOR` |

The file picker lists paths currently visible in the sidebar, preserving annotated-only and unreviewed-only filters. Type to filter full relative paths, use the configured up/down keys or the mouse wheel to move, and press `Enter` or left-click to jump. `Backspace` edits the filter, `Esc` clears it before closing, and `Ctrl+P` closes the picker.

**Search:**

| Key | Action |
|-----|--------|
| `/` | Start search in diff pane |
| `n` | Next search match (overrides next file when search active) |
| `N` | Previous search match |
| `↑` / `Ctrl+P` | Recall previous search query (in search prompt) |
| `↓` / `Ctrl+N` | Recall next search query / clear (in search prompt) |
| `Esc` | Cancel search input / clear search results |

**Annotations:**

| Key | Action |
|-----|--------|
| `a` or `Enter` (diff pane) | Annotate current diff line |
| `A` | Add file-level annotation (stored at top of diff) |
| `@` | Toggle annotation list popup (navigate and jump to any annotation) |
| `}` / `{` | Jump to next/previous annotation (always crosses file boundaries; silent no-op at the first/last annotation) |
| `d` | Delete annotation under cursor |
| `O` | Flush annotations to the `--output` file without exiting (requires `-o`) |
| `Ctrl+E` (during annotation input) | Open `$EDITOR` for multi-line annotation (`open_editor` — rebindable) |
| `Esc` | Cancel annotation input |

While the annotation input is active, press `Ctrl+E` (or whatever key is bound to `open_editor`) to hand off the current text to an external editor for multi-line comments. Editor resolution: `$EDITOR` → `$VISUAL` → `vi`. Values with arguments work (e.g. `EDITOR="code --wait"`). On editor save and quit, the full file contents (including newlines) become the annotation. Quitting the editor with an empty file cancels the annotation and preserves any previously stored note on that line. Multi-line annotations are rendered line-by-line in the diff view, shown flattened in the annotation list popup (`@`), and emitted with embedded newlines in the structured output.

Press `e` in the diff pane to open the focused file in `$EDITOR` (`open_file_in_editor` — rebindable) when revdiff has a stable source path. Editor resolution is the same `$EDITOR` → `$VISUAL` → `vi` chain. Known editors receive either `$EDITOR +N path` or `$EDITOR --goto path:N` as appropriate; unknown editors receive only the file path. File lines are resolved on a best-effort basis. For working tree changes, a clean editor exit reloads the displayed file. For `--staged` or refs, a clean editor exit returns to revdiff without reloading the displayed diff. In compare mode, `e` opens the `--compare-new` side. Working tree files with line annotations cannot be opened for editing because edits can orphan those annotations. Diffs read with `--stdin` do not support opening files. Unsupported rows or files and editor errors show a status hint instead of launching an editor or changing the diff.

Press `O` to write the current annotations to the `--output` file without exiting (`flush_output` — rebindable). This keeps revdiff open while handing the file to an AI agent: annotate, flush with `O`, let the agent read the file and edit code, then reload with `R` and continue in the same session. Each flush overwrites the file with the full current annotation set (a snapshot, not an append log), using the same atomic write as a normal quit. `O` requires `-o`/`--output`; with no output file, or with no annotations yet, it shows a status hint and writes nothing.

Press `Space` to mark the focused file reviewed. Press `F` to toggle the sidebar between all files and unreviewed files; while filtered, marking a file reviewed removes it from the list and advances to the next unfinished file. On `R` reload, revdiff keeps the mark only when the file's effective text diff is unchanged; rebases that only shift line numbers or surrounding context keep it, while changed or removed files lose it. Binary files and opaque placeholders are conservatively unmarked on reload because their rendered diff does not expose enough content to prove they are unchanged.

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
| `F` | Toggle filter: all files / unreviewed only |
| `?` | Toggle help overlay showing all keybindings |
| `i` | Toggle info popup — review scope (mode, VCS, ref, filters, file/status counts, aggregate `+/-` stats) plus the commit log for the current ref range when applicable |
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
| `✓` / `○` | `Space` / `F` | Reviewed files / unreviewed-only filter active |
| `∅` | `u` | Untracked files visible in tree |

On narrow terminals, the left-hand segments are dropped before the icons: search position first, then line and hunk info, then the filename truncates. The icon row on the right stays put.

## Mouse Support

revdiff enables mouse tracking by default so the scroll wheel and left-click work consistently across terminals.

- **Scroll wheel** — scrolls whichever pane the cursor is over. In the tree/TOC pane the wheel moves the cursor one entry per notch (matches `j`/`k`). In the diff pane the wheel scrolls the viewport by three lines per notch — the diff cursor stays on its current logical line and is pinned to the visible edge if scrolling pushes it off-screen.
- **Shift+scroll** — half-page scroll in the diff pane. In the tree/TOC pane Shift+wheel behaves the same as plain wheel (one entry per notch — no page step).
- **Left-click in the tree** — focuses the tree and selects/loads the clicked entry. Clicking a directory row moves the cursor but does not load a file.
- **Left-click in the diff** — focuses the diff and moves the cursor to the clicked line. Enables a "click, then `a`" annotation flow.
- **Left-click in the TOC pane** (single-file markdown) — focuses the TOC and selects the clicked header.
- **Scroll wheel in overlay popups** (info, annotations, themes, files) — scrolls the popup content or moves its cursor. Shift+wheel uses a half-page step. In the theme selector, wheel previews each theme live. Help overlay has no scrollable or selectable content so mouse events are ignored.
- **Left-click in the annotation popup** — jumps to the clicked annotation (same as pressing `Enter`).
- **Left-click in the theme popup** — confirms the clicked theme (same as pressing `Enter`). Clicks on the filter row or blank separator are ignored.
- **Left-click in the file picker** — jumps to the clicked file (same as pressing `Enter`). Clicks on the filter row or blank separator are ignored.

Horizontal wheel, right-click, middle-click, drag selection, and clicks on the status bar or diff header are intentionally ignored. Clicks outside an open overlay are swallowed — dismiss an overlay with `Esc` or its toggle key. Modal states (annotation input, search input, confirm discard, reload confirm) swallow mouse events entirely.

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

**Chord bindings (ctrl/alt leader):** bind a two-stage chord by joining the leader and second key with `>`. The leader must be a `ctrl+*` or `alt+*` combo; the second stage is any single key. Only two stages are supported.

```
map ctrl+w>x mark_reviewed
map alt+t>n theme_select
```

When the leader is pressed, the status bar shows `Pending: ctrl+w, esc to cancel`; press the second key to dispatch, or `esc` to cancel silently. Binding a key as both a standalone action and a chord prefix drops the standalone binding (the chord wins, with a warning). Chord bindings work under non-Latin keyboard layouts — the second-stage key is translated via the same layout-resolve fallback as single-key bindings.

**macOS note:** `alt+*` leaders require your terminal to send Option as Meta/Alt. Most terminals default to "Option composes special characters" (e.g. `Option+T` → `†`), in which case Alt chords silently won't fire. To enable: iTerm2 → *Profiles → Keys → Left/Right Option key → `Esc+`*; Terminal.app → *Profiles → Keyboard → Use Option as Meta key*; Kitty → `macos_option_as_alt yes`; Ghostty → `macos-option-as-alt = true`. If you'd rather not touch terminal settings, use `ctrl+*` leaders — those work everywhere with no configuration.

See the [configuration reference](config.md) for the full list of available actions.

## Vim-motion Preset

Opt-in vim-style motion layer activated via `--vim-motion`, `REVDIFF_VIM_MOTION=true`, or `vim-motion = true` in the config file. Off by default — when off, existing single-key bindings are unchanged.

| Keys | Action |
|------|--------|
| `<N>j` / `<N>k` | Move cursor N lines down/up (diff pane, 1-9999) |
| `gg` | Jump to first line (diff pane) |
| `G` | Jump to last line (diff pane) |
| `<N>G` | Goto line N (diff pane) |
| `H` / `<N>H` | Cursor to top of screen / Nth line from top (diff pane) |
| `M` | Cursor to middle of screen (diff pane) |
| `L` / `<N>L` | Cursor to bottom of screen / Nth line from bottom (diff pane) |
| `zz` | Center viewport on cursor (diff pane) |
| `zt` | Align viewport top on cursor (diff pane) |
| `zb` | Align viewport bottom on cursor (diff pane) |
| `ZZ` | Quit (any pane) |
| `ZQ` | Discard annotations and quit (any pane) |

When the preset is on, the digits `0`-`9` and the leader keys `g`, `z`, `Z` are intercepted before the regular keymap, so any standalone binding on those keys is overridden while the flag is active. `<N>j`/`<N>k`/`<N>G`, `gg`/`zz`/`zt`/`zb`, and the screen-position motions `H`/`M`/`L` apply to the diff pane only — in the file tree they fall through to the normal bindings. `ZZ` and `ZQ` work from any pane. While the preset is active `L` is a screen-position motion rather than the line-numbers toggle (`toggle_line_numbers`); remap it in the keybindings file if you need both. Press `Esc` to silently cancel a pending leader; an unknown second key surfaces a transient `Unknown: <chord>` hint in the status bar. A bare digit `0` is not consumed; counts over 9999 are clamped. Modal keys (search input, annotation input, overlay navigation) always take precedence over the interceptor, and `ctrl+*`/`alt+*` chord bindings keep working orthogonally.

The help overlay (`?`) shows a dedicated **Vim motion** section listing all eleven preset bindings when `--vim-motion` is on; when off, the section is hidden.

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

Exit status: `0` = no annotations, discarded annotations, or default mode; `10` = annotations were produced with `--exit-code-on-annotations`, `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`, or `exit-code-on-annotations`; `1` = real errors. Agent launchers set `REVDIFF_EXIT_CODE_ON_ANNOTATIONS` and treat `10` as success-with-annotations.

## Preloading Annotations

Use `--annotations=PATH` to preload the annotation store from a markdown file in the same `-o` format. The format is bidirectional: any file written by `-o` can be read back via `--annotations` for round-trip workflows — review, quit, edit the file externally, relaunch, and continue from the preloaded state.

## Review History

When you quit with annotations (`q`), revdiff automatically saves a copy of the review session to `~/.config/revdiff/history/<repo-name>/<timestamp>.md`. This is a safety net — if annotations are lost (process crash, agent fails to capture stdout), the history file preserves them.

Each history file contains:
- Header with path, refs, and (git only) a short commit hash
- Full annotation output (same format as stdout)
- Raw git diff for annotated files only

History auto-save is always on and silent — errors are logged to stderr, never fail the process. No history is saved on discard quit (`Q`) or when there are no annotations. For `--stdin` mode, files are saved under `stdin/` subdirectory; for `--only` without git, the parent directory name is used instead of a repo name.

The history file is also a crash-recovery save when the process is terminated by a signal — a SIGHUP from a dropped SSH or tmux client, or a SIGTERM. On a signal exit only the history file is written, never the `-o` output, because a signal is not the deliberate handoff that `q` and `O` perform. Recover the annotations the usual way — load the newest history file. A signal-delivered SIGTERM previously wrote the `-o` output; it no longer does.

Override the history directory with `--history-dir`, `REVDIFF_HISTORY_DIR` env var, or `history-dir` in the config file.

## Disconnect-Resilient Window Mode (tmux)

Set `REVDIFF_TMUX_WINDOW=1` in the launcher's environment to open revdiff in a persistent, server-owned tmux window instead of a client-owned `display-popup`. A dropped SSH or tmux client tears down a popup and kills the review, but a server-owned window survives the disconnect — reattach and the live review is still there. This is a launcher environment variable, not a revdiff flag.
