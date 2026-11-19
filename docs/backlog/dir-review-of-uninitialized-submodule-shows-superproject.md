---
worth: later
where: .claude-plugin/skills/revdiff/SKILL.md:Directory review mode
added: 2026-11-19
---
# directory review of an uninitialized submodule shows the superproject

Directory review mode (both SKILL.md copies) accepts any path-like token that passes `test -d` and
passes it as `--dir`. An uninitialized submodule (a clone made without `--recurse-submodules`) is an
empty directory with no `.git`, so it passes. The `DetectVCS` lookup in revdiff then walks up to the
superproject's `.git`, and the user gets the superproject's diff under a request for
`./modules/child`. Before #413 the same token went to file review and showed a "(not a regular file)"
placeholder.

The submodule has nothing to review, and the file list makes the wrong repository obvious, so the cost
is a confusing review the user quits. Found by a review round on #413.

A fix checks that the target holds its own `.git` (file or directory), `.hg` or `.jj` and otherwise
reports "not a repository, uninitialized submodule?" and stops. It must keep plain subdirectories of the
current repo working, which the round-4 rule on #413 admits on purpose, so the check can apply only
when the directory is not inside the current repository. `git rev-parse --show-toplevel` is not usable
for it: it fails for hg and non-colocated jj targets.
