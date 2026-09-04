---
worth: yes
where: .claude-plugin/skills/revdiff/references/usage.md:102
added: 2026-09-04
---
# usage.md says multi-file stdin supports compact mode, it does not

`.claude-plugin/skills/revdiff/references/usage.md:102` lists what works in multi-file stdin mode as
"one tree entry per file, with `+`/`-` markers, hunk navigation, word-diff, compact mode, per-file
annotations". Compact mode is off there: `compactApplicable` (`app/main.go:451-454`) returns false
whenever `opts.Stdin` is set, before any renderer type assertion. Drop "compact mode" from that list.

Note the same phrase two sections earlier is correct and must not be swept up with it. Line 93
("All standard features work: word-diff, compact mode, ...") documents `--compare-old/--compare-new`,
and `CompareReader` does honor `contextLines`, so compact mode genuinely applies in that mode.

Found while investigating #350; unrelated to submodules.

## The bigger half: the codex copy is several features behind

`plugins/codex/skills/revdiff/references/usage.md` is supposed to track the `.claude-plugin` copy
(CLAUDE.md names them as a keep-in-sync pair). It has drifted well past this one line and is missing:

- the whole multi-file stdin diff mode, still describing `--stdin` as "one synthetic file" where
  "all lines are treated as context"
- the 64 MiB input cap and the per-section parse-failure fallback to raw text
- the `gh pr diff` / `git format-patch` piping examples
- the signal-safe history save paragraph, including that a signal exit writes history but never `-o`
- the `REVDIFF_TMUX_WINDOW=1` disconnect-resilient window section
- the `REVDIFF_AGTERM_PANE=1` pane-scoped overlay section

So the codex copy needs a resync rather than the one-line edit, and the compact-mode error does not
exist there yet only because the paragraph carrying it was never copied over. Fix the claim in the
Claude copy first, then resync, or the resync reintroduces it.

Same failure shape as [[no-guard-over-terminal-enumerations]]: hand-maintained duplicates with
nothing checking one against the other.
