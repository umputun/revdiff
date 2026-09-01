---
worth: maybe
where: app/diff/diff.go:900
added: 2026-09-01
---
# parseUnifiedDiff silently accepts a multi-file diff as one file

`parseUnifiedDiff` is documented and used as a single-file parser, but nothing rejects input holding
several `diff --git` headers. Given one, it absorbs the second and later headers plus their `index`
lines as **context rows**, turns each `---`/`+++` marker into a fake remove/add row with the first
character eaten, and restarts line numbering per embedded file. The result renders as a plausible diff
under whichever filename the caller asked for, and annotations placed on those rows export against
line numbers that do not exist.

Suggested by the reporter of #341 as a separate change, and it is the layer that turned that bug from
loud into silent: `jj diff -- 'a*b.txt'` globbed onto other files, and this parser presented the
concatenated output as one file's diff rather than failing. Confirmed there at 8 rows for a one-line
change, and 184 rows in a 31-file repo.

No known producer today. #346 removed the jj path that reached it, git's `pathArgs` emits a single
pathspec (a rename pair still yields one header), and hg passes literal paths. So this is hardening a
shared parser against a class of caller bug rather than fixing a live defect, which is why it stayed
out of #346.

The open question is the behavior, and it is a real design call rather than a mechanical guard: return
an error, parse only the first file and drop the rest, or keep today's behavior behind a debug warning.
An error is the most honest but turns any future caller mistake into a hard failure in the diff pane,
where the current failure mode at least renders something. Whatever is chosen applies to git and hg
callers too, so the blast radius is every `FileDiff` implementation, not just jj.
