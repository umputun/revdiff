---
worth: yes
where: site/llms.txt:53
added: 2026-09-01
---
# site/llms.txt omits agterm from the supported-terminal list

The line reads "Supports tmux, zellij, herdr, kitty, wezterm, kaku, cmux, ghostty, iTerm2, and Emacs
vterm overlays" — no agterm, though the launcher checks it first
(`.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh:157`, ahead of tmux at `:227`).

This file exists specifically to be read by models, so it now contradicts the FAQ answer in
`site/index.html:83` that PR #342 corrected. A model fetching revdiff.com/llms.txt to answer "does
revdiff work in agterm" gets no; the same site's landing page says yes.

Pre-existing: `git log -S agterm -- site/llms.txt` returns nothing, so the line has never carried it.
Fix is one word — insert `agterm, ` at the head of the list. Surfaced reviewing PR #342.
