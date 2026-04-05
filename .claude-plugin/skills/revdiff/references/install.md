# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Go install:**
```bash
go install github.com/umputun/revdiff/cmd/revdiff@latest
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Claude Code Plugin

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff@umputun-revdiff
```

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, kitty, or wezterm).

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin install revdiff-planning@umputun-revdiff
```
