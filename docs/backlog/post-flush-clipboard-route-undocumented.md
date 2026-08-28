---
worth: later
where: README.md:781
added: 2026-08-28
---
# post-flush command clipboard route is undocumented

Setting `post-flush-command = pbcopy` in the config file and exporting `REVDIFF_OUTPUT` in the shell
profile makes every `O` flush copy the annotations to the clipboard in every session, with no per-launch
flags. This is the direct answer to a common ask, but it appears nowhere in the documentation.

`Output` has `no-ini:"true"` in app/config.go, so the config file cannot provide the output path required
by `O`. Users must either export `REVDIFF_OUTPUT` once or pass `--output` on each launch. Whether
`--output` should become config-settable is a separate product decision.

README.md:781 teaches only the hand-rolled OSC 52 shell-script recipe. `pbcopy` appears in
site/docs.html only inside the Zed section, where it is attached to quit-time output rather than to `O`.

Add the same four-line explanation to README.md, site/docs.html,
.claude-plugin/skills/revdiff/references/config.md, and
.claude-plugin/skills/revdiff/references/usage.md. Surfaced while investigating issue #336, which is
otherwise being answered rather than implemented.
