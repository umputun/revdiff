---
worth: later
where: app/revdiff/testdata/plugin-exit-code/fake-overlay-backend.sh
added: 2026-10-29
---
# tmux window wait loop is never exercised by any test

`agentdeck-window.sh` (both plugin copies) waits on `while [ ! -f "$_rd_sentinel" ]`, polling
`tmux list-windows` and breaking when the review window disappears without a sentinel. No test runs that
loop body: the fake tmux handles `new-window` through `run_after_double_dash`, which runs the review to
completion before returning, so the sentinel already exists on the first check and `list-windows` is never
called. `TestTmuxTitlesEscapeFormatChars` is the only test reaching window mode, and it hits the same dead
loop. A regression that drops the loop (the launcher would exit 1 while a real, asynchronous window is
still open) or that never breaks it (the review hangs after the user kills the window) passes CI.

Fix: give the fake an async `new-window` mode (background the review, as `FAKE_HERDR_ASYNC` with
`FAKE_REVDIFF_DELAY` already does for herdr) with `list-windows` printing the window id while it runs, plus
a killed-window knob that writes no sentinel and stops listing the id. Assert `list-windows` is called and
the killed case exits 1 promptly. Surfaced reviewing PR #421, which adds the same loop to
`launch-plan-review.sh` with the same synchronous fake; one fixture change would cover both.
