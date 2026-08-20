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

Fix is a choice, not a one-liner: raise the timeout, or split the launcher matrix into its own target with
its own budget so the ordinary race run stays fast and the slow matrix is allowed to be slow.
