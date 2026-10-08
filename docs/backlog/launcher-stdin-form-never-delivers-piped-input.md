---
worth: yes
where: .claude-plugin/skills/revdiff/scripts/launch-revdiff.sh
added: 2026-10-07
---
# `producer | launch-revdiff.sh --stdin` never delivers the piped diff to revdiff

`.claude-plugin/skills/revdiff/SKILL.md:56-62` and `plugins/codex/skills/revdiff/SKILL.md:69-71`
document `gh pr diff 123 | <launcher> --stdin` for reviewing a diff that lives outside the working tree
(added in PR #216). The launcher has no stdin handling: its argument loop only appends `--stdin` to
`REVDIFF_CMD`, and every backend runs that command in a process the terminal starts (popup, split,
overlay), whose stdin is the new pane's tty. `validateStdinInput` in `app/revdiff/stdin.go` rejects a
character device with `--stdin requires piped or redirected input`, so the documented form fails
before anything is read. `agentdeck-window.sh` is the same.

Read only, not run: reproduce on one backend first.

Any fix has to spool the launcher's own stdin to a temp file when `--stdin` is among the arguments,
redirect that file into revdiff inside the pane, and add the file to every `EXIT` trap. Both launcher
copies and the launcher test matrix in `app/revdiff/plugin_exit_code_test.go` are affected. Under the
Claude Code sandbox a pipeline also stays sandboxed unless every command in it matches an exclusion,
so the Ghostty / iTerm2 case needs a form where the launcher is the only command in the call.

Surfaced reviewing PR #373.
