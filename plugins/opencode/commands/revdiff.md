---
description: Review git diff with revdiff TUI, then act on its annotations
---

Use the revdiff tool with ref="$ARGUMENTS" to launch the interactive diff review.
Exit code `10` means annotations were captured; treat it as success, not failure.
Wait for the annotations. An annotation whose comment contains `??` anywhere, or starts with explain, remind, describe, what is, what are, how does, how do or clarify (case-insensitive), is a question: answer it in chat and change no code for it.
Address every other annotation with code changes.
After that, summarize what you answered and what you changed.
