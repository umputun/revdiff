# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Codex Plugin

Install the revdiff Codex plugin from the marketplace or manually copy the `plugins/codex/` directory to your Codex plugins location.

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm).

### Plan Review

Use `/revdiff-plan` to extract the last Codex assistant message and open it in revdiff for annotation review.
