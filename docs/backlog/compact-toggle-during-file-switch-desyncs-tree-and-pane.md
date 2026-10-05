---
worth: later
where: app/ui/model.go:toggleCompactMode
added: 2026-10-05
---
# compact toggle during an in-flight file switch leaves the tree and the pane on different files

Press `n` (or any key that requests another file) and then `C` before that file's load arrives.
`toggleCompactMode` calls `reloadCurrentFile` (`app/ui/loaders.go`), which requests `m.file.name`, the
file still displayed, and bumps `file.loadSeq`. The load of the file the tree moved to is now stale and
`handleFileLoaded` drops it. The reload of the old file is accepted, so the tree highlights `b.go`
while the pane shows `a.go`, with no request outstanding to bring them back together.

Reproduced with a two-file model: `n`, `toggleCompactMode`, then both loads delivered in order gives
tree `b.go`, pane `a.go`, `requestedPath` empty. The window is the duration of one file load, so it
takes a slow VCS call or a fast hand to hit. Surfaced while finishing #371.

The fix has two candidate shapes, and choosing between them is why this was not done inline: reload
the requested file when one is outstanding (`file.requestedPath`) instead of the displayed one, or
ignore the toggle until the load completes. Either needs a test delivering the two loads in both
orders.
