---
worth: later
where: app/plugin_exit_code_test.go
added: 2026-09-01
---
# nothing guards the hand-maintained terminal enumerations against the launcher

The supported-terminal set is duplicated by hand in roughly nine places — both SKILL.md descriptions,
both SKILL.md bodies (two lists each), `README.md:87,97,113`, `site/docs.html:700-717`,
`site/index.html:83` and its card grid, `site/llms.txt:53`, both `references/install.md`,
`plugins/codex/README.md:17`, `plugins/revdiff-planning/README.md:21`, `plugins/opencode/README.md:6`,
and the two launchers' own header comments and error strings. The real source of truth is the `if`
chain in `launch-revdiff.sh`.

Nothing checks them against it, so every new backend lands with ~9 hand edits and no way to tell one
was missed. agterm has now needed two reconciliation passes: `cafa44f` added it to README and
docs.html, PR #342 added it to the Claude skill description and index.html, and llms.txt, both
install.md copies and opencode are still behind.

Precedent for a textual guard already exists in the same file — `TestLauncherNestedHeredocsHaveNoApostrophes`
and `TestShellLaunchersPreserveAnnotationExitCode` both read the launcher scripts as text. The cheapest
shape that fits is a test extracting the backend names from `launch-revdiff.sh` and asserting each
documented enumeration contains them. Which surfaces are in scope is a design call, not a mechanical
one, which is why this is filed rather than done. Worth doing at the next new backend, or if a third
agterm reconciliation is needed.
