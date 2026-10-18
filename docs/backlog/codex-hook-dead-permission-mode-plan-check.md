---
worth: maybe
where: plugins/revdiff-planning/scripts/codex-plan-review-hook.py:main
added: 2026-10-18
---
# Codex hook keeps a permission_mode == "plan" check Codex never satisfies

The Codex Stop hook accepts a turn as Plan mode when `permission_mode == "plan"` or when the transcript's
`task_started` record for the turn carries `collaboration_mode_kind == "plan"`. Codex fills
`permission_mode` from the approval policy (`hook_permission_mode` in `codex-rs/core/src/hook_runtime.rs`
maps to `default` or `bypassPermissions` only), so the first branch never fires in a real session. Every
test row in `TestCodexPlanReviewHook` that sets `permission_mode: plan` exercises that unreachable path.

Kept by the #398 fix because removing it was outside that issue. Removing it means moving those test rows
to the transcript signal; keeping it costs nothing at runtime and would cover a future Codex that reports
the mode there. Undecided whether the cleanup is worth the test churn.
