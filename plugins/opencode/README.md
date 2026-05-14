# revdiff OpenCode integration

## Prerequisites

- [revdiff](https://github.com/umputun/revdiff) installed and in `PATH`
- One of the supported terminals: Ghostty, tmux, Kitty, WezTerm, cmux, iTerm2, Emacs vterm

## Files

```
~/.config/opencode/
├── commands/
│   └── revdiff.md
├── tools/
│   ├── revdiff.ts
│   └── launch-revdiff.sh
└── plugins/
    ├── revdiff-plan-review.ts
    └── launch-plan-review.sh
```

## Installation

```sh
bash setup.sh
```

The script creates the target directories if needed, copies all files, marks the shell scripts as executable, and registers the plan-review plugin in `~/.config/opencode/opencode.json`. Or manually:

```sh
mkdir -p ~/.config/opencode/commands ~/.config/opencode/tools ~/.config/opencode/plugins
cp ../../.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh ~/.config/opencode/tools/
chmod +x ~/.config/opencode/tools/launch-revdiff.sh
cp ../revdiff-planning/scripts/launch-plan-review.sh ~/.config/opencode/plugins/
chmod +x ~/.config/opencode/plugins/launch-plan-review.sh
cp commands/revdiff.md ~/.config/opencode/commands/
cp tools/revdiff.ts ~/.config/opencode/tools/
cp plugins/revdiff-plan-review.ts ~/.config/opencode/plugins/
```

Then register the plan-review plugin in `~/.config/opencode/opencode.json`:

```json
{
  "plugin": ["./plugins/revdiff-plan-review.ts"]
}
```

Restart OpenCode after installing — tools and commands are loaded at startup.
