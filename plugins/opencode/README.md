# revdiff OpenCode integration

## Prerequisites

- [revdiff](https://github.com/umputun/revdiff) installed and in `PATH`
- One of the supported terminals: Ghostty, tmux, Kitty, WezTerm, cmux, iTerm2, Emacs vterm

## Files

```
~/.config/opencode/
├── commands/
│   └── revdiff.md
└── tools/
    └── revdiff.ts
```

## Installation

```sh
# tools
mkdir -p ~/.config/opencode/tools
cp tools/revdiff.ts ~/.config/opencode/tools/

# command
mkdir -p ~/.config/opencode/commands
cp commands/revdiff.md ~/.config/opencode/commands/
```

Restart OpenCode after copying — tools and plugins are loaded at startup.
