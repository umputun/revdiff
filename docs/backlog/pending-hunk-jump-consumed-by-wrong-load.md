---
worth: later
where: app/ui/loaders.go:placeCursorAfterLoad
added: 2026-10-05
---
# the cross-file hunk landing is applied by the next accepted successful file load

With `--cross-file-hunks`, `]` on the last hunk of a file sets `nav.pendingHunkJump` and requests the
next file. `placeCursorAfterLoad` applies that landing to the next accepted successful load whatever
file it is for; a load that fails returns earlier and leaves the landing armed. Press `C` before the
next file arrives: the compact reload of the current file is accepted first, the landing meant for
the next file moves the cursor to the first hunk of the current one, and the function returns before
the compact anchor is applied, leaving `compact.pendingAnchor` set.

Reproduced with a two-file model: cursor on line index 3 of `a.go`, `]`, `toggleCompactMode`, both
loads delivered in order gives pane `a.go`, cursor index 1, `pendingAnchor` still set. Surfaced while
finishing #371.

`nav.pendingBoundaryJump` had the same defect and carries the fix to copy: a `seq` field set from
`file.loadSeq` right after `requestFileDiff`, with `placeCursorAfterLoad` dropping the landing when
`seq` differs from the message's. `pendingHunkJump` is a `*bool`, so the change is turning it into a
small struct and updating `handleHunkNav`, `applyPendingHunkJump` and the tests that assign it. Left
out of #371 to keep that change to the feature it adds.
