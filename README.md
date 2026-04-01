# revdiff [![Build Status](https://github.com/umputun/revdiff/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/revdiff/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/umputun/revdiff/badge.svg?branch=master)](https://coveralls.io/github/umputun/revdiff?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/umputun/revdiff)](https://goreportcard.com/report/github.com/umputun/revdiff)

Terminal UI for reviewing git diffs with inline annotations. Two-pane layout with a file tree and colorized diff viewport. Add annotations on specific diff lines and get structured output on quit.

## Features

- Two-pane TUI: file tree (left) + colorized diff viewport (right)
- Full-file view with changes highlighted inline (green adds, red removes)
- Inline annotations on any diff line, plus file-level annotations
- Chunk navigation to jump between change groups
- Filter file tree to show only annotated files
- Structured annotation output to stdout on quit
- Navigate between changed files with keyboard shortcuts

## Installation

```bash
go install github.com/umputun/revdiff/cmd/revdiff@latest
```

## Usage

```
revdiff [OPTIONS] [ref]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `ref` | Git ref to diff against | uncommitted changes |
| `--staged` | Show staged changes | `false` |
| `--tree-width` | File tree panel width in units (1-10), env: `TREE_WIDTH` | `3` |
| `-V`, `--version` | Show version info | |

### Examples

```bash
# review uncommitted changes
revdiff

# review changes against a branch
revdiff main

# review staged changes
revdiff --staged

# review last commit
revdiff HEAD~1
```

### Key Bindings

**Navigation:**

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` or left/right | Switch between file tree and diff pane |
| `Tab` | Switch between file tree and diff pane |
| `PgDown/PgUp` | Page scroll in file tree and diff pane |
| `Ctrl+d/Ctrl+u` | Page scroll in file tree and diff pane |
| `Home/End` | Jump to first/last item |
| `Enter` | Select file (tree pane) / start annotation (diff pane) |
| `n/p` | Next/previous changed file |
| `[` / `]` | Jump to previous/next change hunk in diff |

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
| `f` | Toggle filter: all files / annotated only (shown when annotations exist) |
| `q` | Quit, output annotations to stdout |

### Output Format

```
## handler.go (file-level)
consider splitting this file into smaller modules

## handler.go:43 (+)
use errors.Is() instead of direct comparison

## store.go:18 (-)
don't remove this validation
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.
