---
worth: later
where: .claude-plugin/skills/revdiff/scripts/launch-revdiff.sh:herdr_cleanup_unlaunched
added: 2026-10-07
---
# herdr EXIT trap removes the --stdin spool of a pane it preserves

The herdr `EXIT` trap removes `$STDIN_FILE` unconditionally, while `herdr_cleanup_unlaunched` preserves
the pane when `HERDR_DISPATCHED=1` and the sentinel is absent. The dispatched script opens the spool by
name (`<$STDIN_FILE` inside `REVDIFF_BARE_CMD`), with no equivalent of the `$ERR_WRITER` hard link. If
the launcher exits after `pane run` accepted the dispatch but before the pane's shell opens the spool,
the redirect fails and the preserved pane shows a cannot-open error where `SKILL.md` promises a live
review.

Needs `REVDIFF_HERDR_PANE=1`, `--stdin` with piped input, and a signal during `pane run` or the refusal
grace. A kill during the normal wait loop is unaffected: revdiff has read the whole payload by then.
Read only, not reproduced; found by a review round on the change that added the spool.

Any fix has to leave `$STDIN_FILE` out of the trap's `rm` when a pending dispatch is preserved and move
its removal into the dispatched script, after revdiff exits, in both launcher copies. It also wants a
`--stdin` case in `TestHerdrSignalPaneOwnership`. Deferred because the fix edits the pane-ownership
code for a narrow window on an opt-in mode.
