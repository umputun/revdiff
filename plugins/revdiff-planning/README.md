# revdiff-planning

Claude Code plugin that intercepts `ExitPlanMode` and opens the proposed plan in the [revdiff](https://github.com/umputun/revdiff) TUI for interactive annotation. Annotations captured in the overlay are returned as the deny reason, prompting Claude to adjust the plan and call `ExitPlanMode` again.

## Install

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff-planning@revdiff
```

Requires the `revdiff` binary in `PATH` and one of: tmux, Zellij, kitty, wezterm, cmux, ghostty (macOS), iTerm2 (macOS), or Emacs vterm.

## How It Works

The plugin registers a `PreToolUse` hook on `ExitPlanMode` that:

1. Reads the plan content from the hook event JSON.
2. Parses the optional first-line marker pointing at the previous revision's snapshot (see [Rolling reviews](#rolling-reviews) below) and strips it from the saved content.
3. Writes the stripped plan to a fresh `plan-rev-*.md` snapshot in `$TMPDIR`.
4. Resolves `launch-plan-review.sh` through the override chain (see below).
5. Launches revdiff against the new snapshot — `--compare-old=<prev> --compare-new=<new> --collapsed` when the marker resolved, `--only=<new>` otherwise.
6. Returns the annotations (if any) as the deny reason; otherwise allows the original `ExitPlanMode` to proceed.

## Rolling reviews

After the first round, every iteration shows only what changed since the previous revision instead of re-rendering the whole plan. The chain is held together by an HTML-comment marker the hook tells the agent to put on the first line of its revised plan:

```html
<!-- previous revision: /tmp/plan-rev-AAA.md -->
```

The marker is stripped from the saved snapshot, and the path it points at drives `--compare`. Each chain references its own snapshot, so parallel sessions can't collide.

**Snapshot lifecycle.** New snapshots are created on every invocation. The previous snapshot (`old_snap`) is deleted only when the launcher exits 0 — i.e., the user actually saw the diff. On launcher failure (no overlay terminal available, AppleScript split error, etc.) `old_snap` is preserved so the next attempt can resolve the same marker, and the user sees an `ask` response noting the launcher exit code instead of a misleading "no annotations" success. The new snapshot is kept on `deny` (so the agent's next revision can compare against it) and deleted on the clean `ask` path.

**Agent-discipline failure mode.** The chain assumes the agent prepends the marker on every revised plan. If it forgets — context truncation, model swap, tool drift — the hook treats the revision as a fresh v1 and falls back to `--only`. The user sees the full file again for that round but the loop self-heals: the deny reason re-issues the marker contract every time, so the next round resumes the rolling compare. Failure is recoverable, never fatal.

## Overrides

The hook resolves `launch-plan-review.sh` through a two-layer chain (first-found wins). Drop your own launcher into the user layer to customize how revdiff opens (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

| Layer | Path | Scope |
|---|---|---|
| User | `${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh` | every project (per-user, lives under `~/.claude/plugins/data/<plugin-id>/`) |
| Bundled | `${CLAUDE_PLUGIN_ROOT}/scripts/launch-plan-review.sh` | default — ships with the plugin, used when no override is present |

There is no project-level (`.claude/...`) override layer by design: the hook fires automatically on every `ExitPlanMode` in any repo Claude opens, and a repo-controlled executable layer would let an untrusted repo run arbitrary code on routine Claude actions without per-repo opt-in.

The override file must be **executable** (`chmod +x`). A non-executable file in the user layer is treated as absent — the resolver falls through to the bundled default rather than erroring. Using `chmod -x` is a quick way to disable an override without deleting the file.

To start from the bundled launcher as a template:

```bash
mkdir -p "${CLAUDE_PLUGIN_DATA}/scripts"
cp "${CLAUDE_PLUGIN_ROOT}/scripts/launch-plan-review.sh" "${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh"
chmod +x "${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh"
# edit "${CLAUDE_PLUGIN_DATA}/scripts/launch-plan-review.sh" to taste
```

### Launcher contract

The override receives the same positional arguments the bundled launcher does:

| Form | Args | Mode |
|---|---|---|
| First round (or fallback) | `<plan-file>` | `--only` (single-revision review) |
| Subsequent rounds | `<new-revision> <old-revision>` | compare (rolling diff against the previous snapshot) |

In compare mode the **new revision comes first**, prior revision second. This ordering is deliberate: a stale 1-arg override (e.g. one copied from master before compare mode shipped) silently picks `$1` as its plan file. Putting the new revision first makes that stale launcher degrade to a single-file review of the new content — the legacy UX, no functional regression — instead of opening the prior revision the user already reviewed.

If you want compare-mode highlighting in your own custom launcher, add a 2-arg branch that builds `--compare-old="$2" --compare-new="$1"` and passes it through to revdiff. The bundled `launch-plan-review.sh` is a worked example.

Print captured annotations to stdout on exit so the hook can include them in the deny reason; print nothing to allow the plan as-is. The hook treats a non-zero exit as launcher failure and preserves the previous snapshot so the next attempt can resume the rolling chain.
