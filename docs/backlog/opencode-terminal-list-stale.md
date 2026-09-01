---
worth: later
where: plugins/opencode/README.md:6
added: 2026-09-01
---
# opencode's terminal list has drifted across several backend additions

`plugins/opencode/README.md:6` lists the prerequisites as "Ghostty, tmux, Kitty, WezTerm, cmux, iTerm2,
Emacs vterm" and `plugins/opencode/tools/revdiff.ts:27` describes the tool as opening in
"tmux/kitty/wezterm/ghostty/iTerm2/emacs vterm". Both omit agterm, zellij and herdr.

All three do work there: the opencode README itself says it installs a straight copy of
`.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh`. So an opencode user in zellij, herdr or
agterm reads the prerequisites, sees his terminal missing, and skips an integration that would have
worked — and the tool description misinforms the model deciding whether to call it.

Pre-existing and drifted through several backend additions, which is why it is worth one pass against
the launcher's current backend order rather than a per-PR patch. Surfaced reviewing PR #342.
