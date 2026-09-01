---
worth: later
where: .claude-plugin/skills/revdiff/SKILL.md:3
added: 2026-09-01
---
# both skill descriptions omit kaku while README and the site name it

Neither `description` frontmatter names kaku — `.claude-plugin/skills/revdiff/SKILL.md:3` and
`plugins/codex/skills/revdiff/SKILL.md:3` are byte-identical at 904 characters and both list wezterm
alone — while `README.md:91`, `site/docs.html:707` and `site/index.html:83` all name it.

Codex filed this as a defect against PR #342; verification downgraded it and the downgrade holds.
`launch-revdiff.sh:383` gates a single branch on `$WEZTERM_PANE` and probes `wezterm` first, falling
back to `kaku cli` for the identical split-pane API (`WEZTERM_CLI=(kaku cli)` at `:388`). README:91 says
so outright — "(same API as wezterm)" — and README:97, docs.html:714 and the index.html card at :327
all render the pair as one `wezterm/kaku` step. So the description's `wezterm` already covers the
branch; omitting the alias is editorial compression, not a missing backend. Skill routing also runs off
the separate `Activates on` list, which names no terminal at all, so the practical routing risk is
close to nil.

Pre-existing: the merge-base description carried no kaku either, and PR #342 added only the `agterm/`
prefix. Fix, whenever those files are next open, is one word in both descriptions, positioned as
`wezterm/kaku` to match README. The two copies must stay byte-identical.
