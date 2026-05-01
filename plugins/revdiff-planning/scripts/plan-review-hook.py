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
# to drive --compare mode. leading whitespace before <!-- is tolerated to
# survive common LLM drift (leading blank lines, indented code fences, etc.);
# without that tolerance one stray space silently collapses the rolling chain.
MARKER_RE = re.compile(r"^\s*<!--\s*previous revision:\s*(.+?)\s*-->\s*$")

# snapshot prefix used by NamedTemporaryFile below. the hook only honors a
# marker that resolves to a file under $TMPDIR with this prefix — the marker
# string is fully agent-controlled, so without this guard a confused or hostile
# plan could point --compare-old at any readable file (~/.ssh/id_rsa,
# /etc/passwd, etc.) and surface its contents through the diff overlay and
# subsequent annotations. confining to our own snapshots keeps the hook a
# closed system.
SNAPSHOT_PREFIX = "plan-rev-"


def trusted_snapshot(p: Path) -> Path | None:
    """return the canonical path iff p resolves to an existing file under
    $TMPDIR with the plan-rev-* prefix. callers MUST use the returned path
    rather than the input — using the unresolved input would let a marker
    that names a symlink pass validation here and then read a different
    file when handed to git/revdiff (TOCTOU on the symlink target)."""
    try:
        resolved = p.resolve(strict=True)
    except (OSError, RuntimeError):
        return None
    if not resolved.is_file():
        return None
    if not resolved.name.startswith(SNAPSHOT_PREFIX):
        return None
    tmp_root = Path(tempfile.gettempdir()).resolve()
    try:
        resolved.relative_to(tmp_root)
    except ValueError:
        return None
    return resolved


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
    # regex; the path is used for --compare only when it resolves to a trusted
    # snapshot we wrote ourselves (see is_trusted_snapshot). a stale or
    # untrusted marker silently falls back to --only mode.
    first_line, sep, rest = plan_content.partition("\n")
    m = MARKER_RE.match(first_line)
    if m:
        # use the canonical resolved path returned by trusted_snapshot, NOT
        # the raw marker string — the validator follows symlinks, and so must
        # the launcher, or a benign-looking symlink could be re-pointed
        # between validation and the subprocess.run call.
        old_snap = trusted_snapshot(Path(m.group(1)))
        stripped = rest if sep else ""
    else:
        old_snap = None
        stripped = plan_content

    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".md", prefix=SNAPSHOT_PREFIX, delete=False
    ) as tmp:
        tmp.write(stripped)
        new_snap = Path(tmp.name)

    if old_snap is not None:
        # arg order in compare mode is (new, old), NOT (old, new). a stale
        # 1-arg launcher (pre-compare-mode user override copied from master
        # before this feature shipped) silently picks $1 as PLAN_FILE; putting
        # new_snap first means the stale launcher degrades to --only of the
        # NEW revision (legacy UX, zero regression) instead of opening the
        # OLD revision the user already reviewed last round. the bundled
        # 2-arg branch in launch-plan-review.sh relabels accordingly and
        # composes --compare-old=$OLD --compare-new=$NEW for revdiff itself.
        args = [str(launcher), str(new_snap), str(old_snap)]
    else:
        args = [str(launcher), str(new_snap)]

    try:
        result = subprocess.run(
            args,
            capture_output=True, text=True, timeout=345600,
            env={**os.environ},
        )
    except subprocess.TimeoutExpired:
        # 4-day timeout fired or process hung. clean up our orphan new_snap;
        # preserve old_snap so the next attempt can still resolve the marker.
        new_snap.unlink(missing_ok=True)
        make_response("ask", "plan review timed out; plan not reviewed this round")
        return
    except OSError as exc:
        # launcher could not start (binary missing, permission denied, etc.).
        new_snap.unlink(missing_ok=True)
        make_response("ask", f"plan review launcher failed to start: {exc}")
        return

    # launcher failure (terminal not available, AppleScript split failed, etc.)
    # means the user never saw the diff. preserve old_snap so the next attempt
    # can still resolve the marker; clean up our own orphan new_snap.
    if result.returncode != 0:
        new_snap.unlink(missing_ok=True)
        make_response(
            "ask",
            f"plan review launcher exited {result.returncode}; plan not reviewed this round",
        )
        return

    # launcher succeeded → user saw the compare → old_snap is no longer needed.
    # next iteration will compare new_snap against an even-newer revision.
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
            "IMPORTANT: the very first line of your revised plan MUST be exactly the "
            "marker below. Do NOT substitute any other plan-rev-*.md path you may have "
            "seen earlier in this conversation — older markers belong to unrelated "
            "planning tasks, and pointing at them would compare your revision against "
            "the wrong baseline.\n"
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
