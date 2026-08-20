---
worth: later
where: Makefile:29
added: 2026-08-19
---
# make race timeout leaves too little headroom for the launcher matrix

`make race` runs with `-timeout=100s` and the `app` package alone takes 68s of that on an idle M1 Ultra,
almost all of it `TestShellLaunchersPreserveAnnotationExitCode` shelling out through the launcher-backend
matrix (59s for that test by itself). Under any concurrent load the package crosses 100s and the run fails
with a panic naming that test, which reads as a test defect rather than as the budget being spent.

Surfaced while reviewing PR #321 with six parallel review agents running: the same test timed out in the
review worktree, passed in 192s standalone under that load, and was green at 68s once the machine was idle.
Nothing about the PR was involved. CI is green and so is an idle local run, which is why this is `later`
rather than `yes`.

Re-measured during the PR #327 review (2026-08-19), and the margin has shrunk: the `app` package now runs
81.5s and 87.5s on master and 79.9s on that PR's branch, against 68s before, with
`TestShellLaunchersPreserveAnnotationExitCode` alone at ~77s. Headroom against the 100s budget is down from
roughly 32s to roughly 13s, so an idle run is no longer comfortably clear of it. Still `later`, but the next
addition to the launcher matrix is what turns this into `yes`.

Fix is a choice, not a one-liner: raise the timeout, or split the launcher matrix into its own target with
its own budget so the ordinary race run stays fast and the slow matrix is allowed to be slow.
