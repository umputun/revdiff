# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Codex Plugin

Install the revdiff Codex plugin from the marketplace or manually copy the `plugins/codex/` directory to your Codex plugins location.

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (tmux, Zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm). The bundled launcher sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`; exit `10` is success-with-annotations, not launcher failure.

cmux is detected before ghostty when `$CMUX_SURFACE_ID` is set, `__CFBundleIdentifier=com.cmuxterm.app`, or `GHOSTTY_RESOURCES_DIR` / `GHOSTTY_BIN_DIR` contains `cmux.app`, so cmux uses the cmux CLI instead of Ghostty AppleScript.

### Plan Review

Use `/revdiff-plan` to extract the last Codex assistant message and open it in revdiff for annotation review.
