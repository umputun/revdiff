---
worth: yes
where: README.md
added: 2026-10-07
---
# sandbox exclusion is documented under "permissions", Claude Code reads "sandbox"

`README.md:116`, `.claude-plugin/skills/revdiff/references/install.md:33` and `site/docs.html:730` show
the Ghostty / iTerm2 workaround as `"permissions": { "excludedCommands": ["*/launch-revdiff.sh*"] }`.
Claude Code's settings reference has only `sandbox.excludedCommands`
(https://code.claude.com/docs/en/sandboxing.md, "Run commands outside the sandbox with
`excludedCommands`"), so a user who copies the snippet adds a key the sandbox does not read and the
launch keeps failing.

PR #189 (commit 0efec6c) moved the README to `"sandbox"`. Commit 8a4cfa8 ("fix: detect cmux sessions
with ghostty env") put `"permissions"` back, and install.md and docs.html were never changed.

Fix: change the parent key to `"sandbox"` in all three. Surfaced reviewing PR #373, whose new fallback
text sends users to install.md for this snippet.
