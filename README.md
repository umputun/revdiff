# revdiff [![Build Status](https://github.com/umputun/revdiff/actions/workflows/ci.yml/badge.svg)](https://github.com/umputun/revdiff/actions/workflows/ci.yml) [![Coverage Status](https://coveralls.io/repos/github/umputun/revdiff/badge.svg?branch=master)](https://coveralls.io/github/umputun/revdiff?branch=master) [![Go Report Card](https://goreportcard.com/badge/github.com/umputun/revdiff)](https://goreportcard.com/report/github.com/umputun/revdiff)

Terminal UI for reviewing git diffs with inline annotations. Two-pane layout with a file tree and colorized diff viewport. Add annotations on specific diff lines and get structured output on quit.

## Features

- Two-pane TUI: file tree (left) + colorized diff viewport (right)
- Full-file view with changes highlighted inline (green adds, red removes)
- Inline annotations on any diff line
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

| Key | Action |
|-----|--------|
| `j/k` or up/down | Navigate files (tree) / scroll diff (diff pane) |
| `h/l` or left/right | Switch between file tree and diff pane |
| `PgDown/PgUp` | Page scroll in file tree and diff pane |
| `Ctrl+d/Ctrl+u` | Page scroll in file tree and diff pane |
| `Home/End` | Jump to first/last item |
| `Enter` | Select file from list |
| `Tab` | Toggle all files / annotated only |
| `n/p` | Next/previous changed file |
| `a` | Annotate current diff line |
| `d` | Delete annotation under cursor (shown only on annotated lines) |
| `Esc` | Cancel annotation input |
| `q` | Quit, output annotations to stdout |

### Output Format

```
## handler.go:43 (+)
use errors.Is() instead of direct comparison

## store.go:18 (-)
don't remove this validation
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT License - see [LICENSE](LICENSE) file for details.
