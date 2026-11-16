---
worth: later
where: README.md
added: 2026-11-16
---
# ghostty backend row does not say agent-tab targeting needs 1.4.0

Since #450 the Ghostty launchers split the agent's own terminal, found by Ghostty 1.4.0's AppleScript `tty`
property, and fall back to the front tab on older Ghostty. The backend row in README.md and its twin in
site/docs.html still read only "AppleScript split + zoom (macOS only)", so a 1.3.x user who hits the #110
symptom gets no hint that upgrading fixes it. The version gate is stated only in the launcher comment.
Fix: add "targets the agent's own terminal on Ghostty 1.4.0+, the front tab on older versions" to both
rows together. Surfaced reviewing PR #450; deferred because the release note can say it first.
