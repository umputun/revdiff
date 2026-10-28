---
worth: yes
where: plugins/codex/skills/revdiff/references/usage.md
added: 2026-09-04
---
# The codex usage.md copy is several features behind

`plugins/codex/skills/revdiff/references/usage.md` is supposed to track the `.claude-plugin` copy
(CLAUDE.md names them as a keep-in-sync pair). It has drifted well past a single line and is missing:

- the whole multi-file stdin diff mode, still describing `--stdin` as "one synthetic file" where
  "all lines are treated as context"
- the 64 MiB input cap and the per-section parse-failure fallback to raw text
- the `gh pr diff` / `git format-patch` piping examples
- the signal-safe history save paragraph, including that a signal exit writes history but never `-o`
- the `REVDIFF_TMUX_WINDOW=1` disconnect-resilient window section
- the `REVDIFF_AGTERM_PANE=1` pane-scoped overlay section

The multi-file stdin paragraph must not list compact mode: `compactApplicable`
(`app/revdiff/main.go`) returns false whenever `opts.Stdin` is set. The `--compare-old/--compare-new`
paragraph does list it correctly, since `CompareReader` honors `contextLines`.

Same failure shape as [[no-guard-over-terminal-enumerations]]: hand-maintained duplicates with
nothing checking one against the other.
