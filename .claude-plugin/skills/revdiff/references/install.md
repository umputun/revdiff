# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Claude Code Plugin

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff@umputun-revdiff
```

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm). Set `REVDIFF_LAUNCHER` to a custom launcher script to override built-in terminal detection.

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin install revdiff-planning@umputun-revdiff
```
