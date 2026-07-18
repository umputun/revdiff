#!/usr/bin/env python3
"""Review completed Codex plans through the Stop hook."""

from __future__ import annotations

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any

PLAN_RE = re.compile(r"<proposed_plan>\s*(.*?)\s*</proposed_plan>", re.DOTALL)
MARKER_RE = re.compile(r"^\s*<!--\s*previous revision:\s*(.+?)\s*-->\s*$")
SNAPSHOT_PREFIX = "plan-rev-"
SUCCESS_CODES = {0, 10}


def respond(payload: dict[str, Any] | None = None) -> None:
    print(json.dumps(payload or {}, separators=(",", ":")))


def warn(message: str) -> None:
    respond({"systemMessage": f"RevDiff plan review skipped: {message}"})


def read_event() -> dict[str, Any] | None:
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, OSError):
        warn("invalid Stop hook JSON payload")
        return None
    if not isinstance(event, dict):
        warn("Stop hook payload must be a JSON object")
        return None
    return event


def extract_plan(message: str) -> str | None:
    for match in reversed(PLAN_RE.findall(message)):
        plan = match.strip()
        if plan:
            return plan
    return None


def trusted_snapshot(path: Path) -> Path | None:
    """Return a canonical snapshot path confined to this hook's temp files."""
    try:
        resolved = path.resolve(strict=True)
    except (OSError, RuntimeError):
        return None
    if not resolved.is_file() or not resolved.name.startswith(SNAPSHOT_PREFIX):
        return None
    try:
        resolved.relative_to(Path(tempfile.gettempdir()).resolve())
    except ValueError:
        return None
    return resolved


def message_from_transcript(event: dict[str, Any]) -> tuple[str | None, str | None]:
    """Return this turn's last plan message, or its last assistant message."""
    transcript = event.get("transcript_path")
    session_id = event.get("session_id")
    turn_id = event.get("turn_id")
    if not all(isinstance(value, str) and value for value in (transcript, session_id, turn_id)):
        return None, "transcript fallback requires transcript_path, session_id, and turn_id"

    path = Path(transcript)
    if not path.name.endswith(f"-{session_id}.jsonl"):
        return None, "transcript path does not match the current session"
    if not path.is_file():
        return None, "transcript file is unavailable"

    message: str | None = None
    plan_message: str | None = None
    try:
        with path.open(encoding="utf-8") as stream:
            for line in stream:
                try:
                    item = json.loads(line)
                except json.JSONDecodeError:
                    continue
                payload = item.get("payload") if isinstance(item, dict) else None
                if not isinstance(payload, dict):
                    continue
                metadata = payload.get("internal_chat_message_metadata_passthrough")
                if not (
                    item.get("type") == "response_item"
                    and payload.get("type") == "message"
                    and payload.get("role") == "assistant"
                    and isinstance(metadata, dict)
                    and metadata.get("turn_id") == turn_id
                ):
                    continue
                content = payload.get("content")
                if not isinstance(content, list):
                    continue
                parts = [
                    part.get("text")
                    for part in content
                    if isinstance(part, dict)
                    and part.get("type") == "output_text"
                    and isinstance(part.get("text"), str)
                ]
                if parts:
                    message = "\n".join(parts)
                    if extract_plan(message) is not None:
                        plan_message = message
    except OSError:
        return None, "transcript file could not be read"
    if message is None:
        return None, "transcript has no assistant message for the current turn"
    return plan_message or message, None


def resolve_launcher(plugin_root: Path, data_dir: str, cwd: object) -> Path | None:
    resolver = plugin_root / "scripts" / "resolve-launcher.sh"
    if not resolver.is_file():
        return None
    working_dir = cwd if isinstance(cwd, str) and Path(cwd).is_dir() else None
    try:
        result = subprocess.run(
            [str(resolver), "launch-plan-review.sh", data_dir],
            capture_output=True,
            text=True,
            timeout=10,
            cwd=working_dir,
            check=False,
        )
    except (OSError, subprocess.SubprocessError):
        return None
    if result.returncode != 0:
        return None
    path = Path(result.stdout.strip())
    return path if path.is_file() and os.access(path, os.X_OK) else None


def main() -> None:
    event = read_event()
    if event is None:
        return
    if event.get("hook_event_name") != "Stop":
        respond()
        return
    if event.get("permission_mode") != "plan":
        respond()
        return

    message = event.get("last_assistant_message")
    plan = extract_plan(message) if isinstance(message, str) else None
    if plan is None:
        transcript_message, error = message_from_transcript(event)
        if error is not None:
            warn(error)
            return
        plan = extract_plan(transcript_message or "")
        if plan is None:
            # A readable current-turn transcript without a complete plan is a
            # clarification or another intermediate Plan-mode response.
            respond()
            return

    plugin_root_value = os.environ.get("PLUGIN_ROOT") or os.environ.get("CLAUDE_PLUGIN_ROOT")
    if not plugin_root_value:
        warn("PLUGIN_ROOT is not set")
        return
    if not shutil.which("revdiff"):
        warn("revdiff binary is not available on PATH")
        return

    plugin_root = Path(plugin_root_value)
    data_dir = os.environ.get("PLUGIN_DATA") or os.environ.get("CLAUDE_PLUGIN_DATA", "")
    launcher = resolve_launcher(plugin_root, data_dir, event.get("cwd"))
    if launcher is None:
        warn("no executable launch-plan-review.sh was found")
        return

    first_line, separator, rest = plan.partition("\n")
    marker = MARKER_RE.match(first_line)
    if marker:
        old_snapshot = trusted_snapshot(Path(marker.group(1)))
        plan = rest if separator else ""
    else:
        old_snapshot = None

    new_snapshot: Path | None = None
    operation = "snapshot"
    try:
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".md", prefix=SNAPSHOT_PREFIX, delete=False
        ) as tmp:
            new_snapshot = Path(tmp.name)
            tmp.write(plan)
        operation = "launcher"
        args = [str(launcher), str(new_snapshot)]
        if old_snapshot is not None:
            args.append(str(old_snapshot))
        result = subprocess.run(
            args,
            capture_output=True,
            text=True,
            timeout=345600,
            check=False,
        )
    except KeyboardInterrupt:
        if new_snapshot is not None:
            new_snapshot.unlink(missing_ok=True)
        warn("launcher was interrupted")
        return
    except subprocess.TimeoutExpired:
        if new_snapshot is not None:
            new_snapshot.unlink(missing_ok=True)
        warn("launcher timed out")
        return
    except OSError:
        if new_snapshot is not None:
            new_snapshot.unlink(missing_ok=True)
        warn(
            "plan snapshot could not be written"
            if operation == "snapshot"
            else "launcher could not be started"
        )
        return

    assert new_snapshot is not None
    if result.returncode not in SUCCESS_CODES:
        new_snapshot.unlink(missing_ok=True)
        warn(f"launcher exited with status {result.returncode}")
        return

    if old_snapshot is not None:
        old_snapshot.unlink(missing_ok=True)

    annotations = result.stdout.strip()
    if not annotations:
        new_snapshot.unlink(missing_ok=True)
        respond()
        return

    respond(
        {
            "decision": "block",
            "reason": (
                "The user reviewed the proposed plan in RevDiff and added annotations. "
                "Revise the plan to address every annotation, then output the complete revised "
                "plan inside <proposed_plan> tags for another review.\n\n"
                f"{annotations}\n\n"
                "IMPORTANT: output the full <proposed_plan> block again, and put this marker "
                "on the very first line inside the block. Do NOT substitute any other "
                "plan-rev-*.md path you may have seen earlier in this conversation; older "
                "markers belong to unrelated planning tasks and would compare against the "
                "wrong baseline:\n"
                f"<!-- previous revision: {new_snapshot} -->"
            ),
        }
    )


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        warn("launcher was interrupted")
