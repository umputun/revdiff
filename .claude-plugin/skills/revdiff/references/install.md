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

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm).

**Sandbox note:** Ghostty and iTerm2 launchers use `osascript` (Apple Events), which is blocked by Claude Code's sandbox. If you use these terminals with sandbox enabled, add to your Claude Code `settings.json`:

```json
{
  "permissions": {
    "excludedCommands": ["*/launch-revdiff.sh*"]
  }
}
```

Terminals using CLI tools (tmux, Zellij, kitty, wezterm, cmux) are not affected.

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin install revdiff-planning@umputun-revdiff
```
