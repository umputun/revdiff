---
worth: later
where: plugins/opencode/setup.sh
added: 2026-10-04
---
# opencode setup.sh replaces a symlinked opencode.json with a regular file

The installer registers the plugin by writing jq output to a `mktemp` file and `mv`-ing it over
`opencode.json`. `mv` replaces the path itself, so a config that is a symlink (into a dotfiles repo,
for example) becomes a detached regular file with mode 0600, and the link target keeps its old content.

The file content is correct afterwards and the link is easy to restore, so nothing is lost. A fix has to
choose between the atomic rename and writing through the link (`cat "$tmp" > "$file"`), and must leave
the original intact when jq fails. Surfaced reviewing the OpenCode v2 installer, which reuses the same
idiom for its v1-registration cleanup: fix every such site in one change.
