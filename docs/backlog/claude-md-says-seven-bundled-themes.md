---
worth: yes
where: CLAUDE.md:25
added: 2026-09-01
---
# CLAUDE.md says 7 bundled themes, there are 8

`CLAUDE.md:25` ("7 bundled + community gallery") and `CLAUDE.md:43` ("with 7 bundled themes") both
undercount. `themes/gallery` holds 8 files marked `bundled: true`: basic, catppuccin-latte,
catppuccin-mocha, dracula, gruvbox, nord, revdiff, solarized-dark. `colorblind-dark` and
`colorblind-light` are gallery-only, which is presumably where the drift came from.

Same failure shape as [[no-guard-over-terminal-enumerations]]: a hand-maintained count restated in
several files with nothing checking it against the source of truth.

Check README.md, `site/docs.html`, `themes/README.md` and both plugin `references/config.md` copies
for the same number before fixing, so the count moves everywhere at once rather than one file at a
time.
