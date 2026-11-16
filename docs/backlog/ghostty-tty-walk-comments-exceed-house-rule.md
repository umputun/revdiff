---
worth: later
where: app/revdiff/plugin_exit_code_test.go:TestGhosttySplitTargetsAgentTTY
added: 2026-11-16
---
# ghostty tty-walk comments exceed the comment rules

`TestGhosttySplitTargetsAgentTTY` carries a two-line regression comment and an in-body comment describing
the fake `ps` setup, against the rule of no test comments beyond one lowercase line naming the defect. The
three-line comment above the tty walk in all three launchers (launch-revdiff.sh, its codex copy,
launch-plan-review.sh) partly narrates what the loop does rather than why. Fix: fold the test lead into
one line, drop the in-body comment, and cut the launcher comment to the reason (the tool shell is
usually detached, so the nearest ancestor's tty is the agent terminal's). Surfaced reviewing PR #450 (an
outside contributor's change, so not asked of him); no behavior defect behind it.
