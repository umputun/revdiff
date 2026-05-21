---
description: Review git diff with revdiff TUI, then fix all annotations
---

Use the revdiff tool with ref="$ARGUMENTS" to launch the interactive diff review.
Exit code `10` means annotations were captured; treat it as success, not failure.
Wait for the annotations, then address each one with code changes.
After fixing everything, summarize what you changed.
