#!/usr/bin/env python3
"""plan-review-hook.py - PreToolUse hook for ExitPlanMode.

intercepts ExitPlanMode and opens plan for user review in revdiff TUI.
if revdiff is not installed, passes through to normal confirmation.

hook receives JSON on stdin with the plan content in tool_input.plan field.
returns PreToolUse hook JSON response with permissionDecision:
  - "ask"  → no changes/annotations, proceed to normal confirmation
  - "deny" → feedback found, sent as denial reason

requirements:
  - revdiff binary in PATH
  - tmux, kitty, or wezterm terminal
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path


def read_plan_from_stdin() -> str:
    """read plan content from hook event JSON on stdin."""
    raw = sys.stdin.read()
    if not raw.strip():
        return ""
    try:
        event = json.loads(raw)
        return event.get("tool_input", {}).get("plan", "")
    except json.JSONDecodeError:
        return ""


def make_response(decision: str, reason: str = "") -> None:
    """output PreToolUse hook response and exit with appropriate code.
    deny: plain text to stderr + exit 2 (Claude Code blocks the tool and shows the text).
    ask/allow: JSON to stdout + exit 0."""
    if decision == "deny":
        print(reason, file=sys.stderr)
        sys.exit(2)
    resp: dict = {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": decision,
        }
    }
    if reason:
        resp["hookSpecificOutput"]["permissionDecisionReason"] = reason
    print(json.dumps(resp, indent=2))


def main() -> None:
    plan_content = read_plan_from_stdin()
    if not plan_content:
        make_response("ask", "no plan content in hook event")
        return

    plugin_root = os.environ.get("CLAUDE_PLUGIN_ROOT", "")
    if not plugin_root:
        make_response("ask", "CLAUDE_PLUGIN_ROOT not set")
        return

    if not shutil.which("revdiff"):
        make_response("ask", "revdiff not installed, skipping plan review")
        return

    launcher = Path(plugin_root) / "scripts" / "launch-plan-review.sh"
    if not launcher.exists():
        make_response("ask", "launch-plan-review.sh not found")
        return

    # write plan to temp file
    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".md", prefix="plan-review-", delete=False
    ) as tmp:
        tmp.write(plan_content)
        tmp_path = Path(tmp.name)

    try:
        result = subprocess.run(
            [str(launcher), str(tmp_path)],
            capture_output=True, text=True, timeout=345600,
            env={**os.environ},
        )
        annotations = result.stdout.strip()
        if not annotations:
            make_response("ask", "plan reviewed, no annotations")
        else:
            make_response(
                "deny",
                "user reviewed the plan in revdiff and added annotations. "
                "each annotation references a specific line and contains the user's feedback.\n\n"
                f"{annotations}\n\n"
                "adjust the plan to address each annotation, then call ExitPlanMode again.",
            )
    finally:
        tmp_path.unlink(missing_ok=True)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\r\033[K", end="")
        sys.exit(130)
