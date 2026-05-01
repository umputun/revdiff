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
  - tmux, kitty, wezterm, cmux, or ghostty (macOS) terminal
"""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

# marker placed by the agent on the first line of a revised plan to point at
# the previous revision's snapshot, e.g.
#   <!-- previous revision: /tmp/plan-rev-AAA.md -->
# the hook strips this line before saving the new snapshot and uses the path
# to drive --compare mode.
MARKER_RE = re.compile(r"^<!--\s*previous revision:\s*(.+?)\s*-->\s*$")


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


def resolve_launcher(plugin_root: str, name: str) -> Path | None:
    """resolve launcher path through override chain.

    shells out to <plugin_root>/scripts/resolve-launcher.sh, passing the launcher
    name and CLAUDE_PLUGIN_DATA so the resolver can walk user → bundled.
    returns Path on resolver success (exit 0); returns None if the resolver exits
    non-zero (all layers absent, or invocation error). logs resolver stderr for
    diagnostics."""
    resolver = Path(plugin_root) / "scripts" / "resolve-launcher.sh"
    if not resolver.exists():
        print(f"resolve-launcher.sh not found at {resolver}", file=sys.stderr)
        return None
    data_dir = os.environ.get("CLAUDE_PLUGIN_DATA", "")
    # pin cwd to CLAUDE_PROJECT_DIR for hygiene: ensures consistent CWD for the
    # resolver invocation regardless of where Claude Code launches the hook
    # from. CLAUDE_PROJECT_DIR is the standard env var Claude Code sets for
    # hook subprocesses; if absent we inherit the parent's cwd.
    project_dir = os.environ.get("CLAUDE_PROJECT_DIR") or None
    try:
        result = subprocess.run(
            [str(resolver), name, data_dir],
            capture_output=True, text=True, timeout=10,
            cwd=project_dir,
        )
    except (OSError, subprocess.SubprocessError) as exc:
        print(f"resolve-launcher.sh invocation failed: {exc}", file=sys.stderr)
        return None
    if result.returncode != 0:
        if result.stderr:
            print(result.stderr.rstrip(), file=sys.stderr)
        return None
    path = result.stdout.strip()
    if not path:
        return None
    return Path(path)


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

    launcher = resolve_launcher(plugin_root, "launch-plan-review.sh")
    if launcher is None:
        make_response("ask", "launch-plan-review.sh not found")
        return

    # parse optional first-line marker pointing at the previous snapshot.
    # marker is always stripped from the saved snapshot when it matches the
    # regex; the path is used for --compare only when the file still exists,
    # otherwise we fall back to --only mode (treats stale marker as no marker).
    first_line, sep, rest = plan_content.partition("\n")
    m = MARKER_RE.match(first_line)
    if m:
        candidate = Path(m.group(1))
        old_snap: Path | None = candidate if candidate.is_file() else None
        stripped = rest if sep else ""
    else:
        old_snap = None
        stripped = plan_content

    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".md", prefix="plan-rev-", delete=False
    ) as tmp:
        tmp.write(stripped)
        new_snap = Path(tmp.name)

    if old_snap is not None:
        args = [str(launcher), str(old_snap), str(new_snap)]
    else:
        args = [str(launcher), str(new_snap)]

    result = subprocess.run(
        args,
        capture_output=True, text=True, timeout=345600,
        env={**os.environ},
    )

    # old snapshot is no longer needed once the compare has been viewed —
    # next iteration will compare new_snap against an even-newer revision
    if old_snap is not None:
        old_snap.unlink(missing_ok=True)

    annotations = result.stdout.strip()
    if annotations:
        # keep new_snap on disk — next ExitPlanMode call will compare against it
        make_response(
            "deny",
            "user reviewed the plan in revdiff and added annotations. "
            "each annotation references a specific line and contains the user's feedback.\n\n"
            f"{annotations}\n\n"
            "adjust the plan to address each annotation, then call ExitPlanMode again.\n\n"
            "IMPORTANT: the very first line of your revised plan MUST be exactly:\n"
            f"<!-- previous revision: {new_snap} -->\n"
            "this lets revdiff show only what changed in your revision.",
        )
    else:
        new_snap.unlink(missing_ok=True)
        make_response("ask", "plan reviewed, no annotations")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\r\033[K", end="")
        sys.exit(130)
