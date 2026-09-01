---
worth: yes
where: .claude-plugin/skills/revdiff/references/install.md:17
added: 2026-09-01
---
# both install references omit agterm

`.claude-plugin/skills/revdiff/references/install.md:17` and its codex copy at
`plugins/codex/skills/revdiff/references/install.md:14` both enumerate the overlay terminals as
"(tmux, Zellij, herdr, kitty, wezterm, cmux, ghostty, iTerm2, or Emacs vterm)" — no agterm.

Separately, `install.md:31`'s sandbox note lists the terminals unaffected by the osascript restriction
as "(tmux, Zellij, herdr, kitty, wezterm, cmux)" and omits agterm, where `README.md:113` and
`site/docs.html:717` both include it. A user in agterm reading that note adds an `excludedCommands`
entry he does not need, or concludes his terminal is unsupported and installs tmux first.

Pre-existing: all three lines predate PR #342 and none was touched by it. Fix is `agterm, ` in three
places, keeping the two reference trees as copies per CLAUDE.md. Surfaced reviewing PR #342.
