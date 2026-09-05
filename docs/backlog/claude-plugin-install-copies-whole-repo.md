---
worth: later
where: .claude-plugin/marketplace.json:10
added: 2026-09-04
---
# claude plugin install copies the whole repo into the plugin cache

The `revdiff` entry in `.claude-plugin/marketplace.json` has `source: "./"`, so `/plugin install
revdiff@revdiff` copies the entire repository into `~/.claude/plugins/cache/revdiff/revdiff/<version>/`:
28M on 0.8.23 with `vendor/`, `app/`, `docs/` and `site/` included, when the plugin needs only
`.claude-plugin/`. Codex had the same shape until #349 moved its source to `plugins/codex`.

Proposed fix: relocate the Claude plugin source out of the repo root the way `revdiff-planning` already
sits under `plugins/revdiff-planning` with its own `.claude-plugin/plugin.json`, and point the marketplace
entry at the new directory. `plugin.json`'s `skills` path is repo-root relative today (CLAUDE.md), so the
manifest, the marketplace entry, the `claude --plugin-dir` testing line, the Codex script-copy source
comments, and the launcher paths in `app/plugin_exit_code_test.go` move together. Surfaced reviewing
PR #349; deferred because the move touches every Claude-side path and the codex copies' `# source:`
headers, none of which the PR owned.
