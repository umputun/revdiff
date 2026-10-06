# revdiff OpenCode integration

## Prerequisites

- [revdiff](https://github.com/umputun/revdiff) installed and in the terminal client's `PATH`
- A supported terminal: agterm, tmux, zellij, herdr, Kitty, WezTerm, cmux, Ghostty, iTerm2 or Emacs vterm
- `jq` when `opencode.json` already exists

## Installation

Use the same installer for OpenCode v1 and v2:

```sh
bash setup.sh
```

It runs `opencode --version` using the binary on `PATH` and installs the matching integration. V2 has no Node.js/npm installation step.

For v2, the destination is `OPENCODE_CONFIG_DIR`, then `$XDG_CONFIG_HOME/opencode`, then `~/.config/opencode`. V1 installs only to `~/.config/opencode`, matching its fixed plan-launcher path; a different config directory is rejected before installation. Select a different binary and, for v2, a config directory explicitly when using multiple installations:

```sh
bash setup.sh --opencode /path/to/opencode-v2 /path/to/opencode-config
```

Restart OpenCode after installation. Existing installations of the same major can be updated by rerunning the command.

### Switching major versions

- **v1 → v2:** cleanup is automatic. The installer removes `plugins/revdiff-plan-review.ts`, the obsolete `commands/revdiff.md`, and the exact string `./plugins/revdiff-plan-review.ts` from the `plugin`/`plugins` arrays in `opencode.json`. Other entries, options, files and the v1 tool are preserved.
- **JSONC:** V2 leaves `opencode.jsonc`, and `opencode.json` containing comments or trailing commas, byte-identical and prints a manual-cleanup notice. V1 installs normally and creates its registration in `opencode.json` alongside an existing JSONC file.
- **v2 → v1:** the installer restores the v1 files and registration. The nested `plugins/revdiff/` directory is inert to v1 and remains in place.

If `plugins/revdiff/` contains `index.*` or `server.*`, move that directory aside before installation: OpenCode would load it as a server plugin.

## OpenCode v2

The v2 integration lives in `v2/` and supports OpenCode **v2.0.0+**. One CLI plugin provides `/revdiff` and automatic plan review. There is no v2 agent-callable tool or server-side plugin.

### Installation

`setup.sh` copies `tui.ts`, `launcher.ts`, `claims.ts`, the package manifest and the shared diff launcher into `plugins/revdiff/`. The directory contains no `index.*`, `server.*` or RPC entrypoint. OpenCode discovers the CLI plugin automatically; no `cli.json` edit is needed. SDK imports are type-only, so the local plugin has no runtime npm dependencies.

Or install manually from `plugins/opencode/`:

For a manual v1 → v2 migration, first remove the old plan plugin, Markdown command and exact config registration listed under **Switching major versions**. If the destination contains `index.*` or `server.*`, move it aside before copying the CLI-only files.

```sh
CONFIG_DIR="${OPENCODE_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/opencode}"
PLUGIN_DIR="$CONFIG_DIR/plugins/revdiff"
mkdir -p "$PLUGIN_DIR/scripts"
cp v2/tui.ts v2/launcher.ts v2/claims.ts v2/package.json "$PLUGIN_DIR/"
cp ../../.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh "$PLUGIN_DIR/scripts/"
cp ../../.claude-plugin/skills/revdiff/scripts/agentdeck-window.sh "$PLUGIN_DIR/scripts/"
chmod +x "$PLUGIN_DIR/scripts/launch-revdiff.sh"
```

Restart OpenCode after installation.

### Behavior

- `/revdiff` reviews working-tree changes; `/revdiff HEAD~3` supplies a ref. `/revdiff --staged --only="a b.go"` supports staged review and repeated `--only` arguments. The command runs directly in the CLI, without a model tool call.
- Manual review also works from the home screen. A clean exit leaves home unchanged. When there are annotations, the CLI creates a session in the captured project with the configured default agent and selected model, then sends the notes there; it does not redirect them to a different session opened while review was running. Older v2 clients without the model-selection API use the configured default model.
- The CLI launches the unchanged shared `launch-revdiff.sh` with its own terminal environment and the captured session's project directory. Plans use the binary's `--only` file mode and wrapping through that same launcher.
- Automatic review handles live `session.execution.succeeded` events only in the root session this CLI is displaying. It requires loaded session information without a `parentID` and a successful, completed, non-truncated `plan` response after a fresh `data.session.message.sync`. It ignores subagents and other sessions and does not sweep older plans on startup or session switches.
- The session ID is captured at launch. Annotations return to that session even if the user switches views while the overlay is open. Changing the reviewed session's agent or starting another execution cancels the stale plan operation, including its initial lookup, until feedback is admitted. A matching `session.inbox.enqueued` event confirms delivery by message ID, so the feedback's own execution cannot cancel its pending HTTP response or produce a false undelivered-notes alert.
- Plan feedback includes the exact reviewed snapshot with original line numbers, including blank lines, alongside the annotations. The temporary plan file is deleted after review; the agent is told to use the included snapshot rather than read the removed path.
- A clean exit sends no prompt and leaves the session in plan mode (#211). Annotations are submitted for revision without switching agents. Exit `10` is successful annotation output; other nonzero exits show an error toast, including when captured notes are also shown in an alert.
- The CLI owns a private output file, passed as the final `--output` argument. Notes already flushed before cancellation are preserved even though the unchanged launcher cleans its own output file. Undelivered notes are displayed in an alert; plugin cleanup does not wait for dismiss or close the alert. Cancellation also interrupts waiting for message synchronization, even though the SDK's underlying sync request is not cancellable.

Both client and server must use the same local checkout; remote filesystem mapping is not implemented. Cancellation terminates the local launcher process group and removes its temporary files, but a terminal-owned review may stay open. The CLI reports it as detached in a persistent alert even when no notes have been flushed. Its output directory is gone, so flushing to OpenCode is no longer available: finish that review normally to save later notes in revdiff history. Those later notes are not delivered automatically. Any notes captured before cancellation are shown in the alert.

### Automatic review ownership

Ownership is local to clients of the same user sharing the same state directory. Immediately before launching a plan, the CLI atomically creates a private marker using exclusive file creation (`open(..., "wx")`). The claim key is the ID of the `session.execution.succeeded` event frame, shared by every subscriber, rather than a message ID from a client's snapshot. The filename is `event-<SHA-256 of event ID>.claim`; markers contain no plan text. Only the winning client launches the review, even when clients have different message snapshots. Message IDs select and validate the plan text after synchronization.

Markers live under `${XDG_STATE_HOME:-$HOME/.local/state}/revdiff/opencode-plan-review`. Each claim removes empty marker files older than one day; unrelated files, directories and symlinks are left alone. Markers only coordinate a live event's race: the event feed has no replay, so long-term persistence across restarts is not required. A claim means at most one review attempt: cancellation, launcher failure or a crash after claiming does not transfer that event to another client. A new completion event has a new claim key.

Before claiming, the CLI checks that the launcher is executable and `revdiff` is on its launch environment's `PATH`, then rechecks the displayed session after asynchronous preparation. It checks the displayed session again after the claim and before opening the overlay. `EEXIST` means another client owns the event; other claim errors show a toast and prevent launch.

This does not coordinate clients on different machines or with different state directories. The winning client may be in a window the user is not currently looking at; that is an accepted limitation. Manual `/revdiff` commands are explicit user actions and do not use plan deduplication. This local-only, at-most-once ownership follows issue #369 and uses no server extension or tool-routing protocol.

### Development and checks

```sh
cd v2
npm ci --ignore-scripts
npm test
npm run typecheck
npm run test:runtime -- /absolute/path/to/opencode-v2
```

Development checks require Node.js 22.18+ and npm. The runtime check uses the project's Go toolchain to build `app/ptybridge/`, a macOS/Linux PTY bridge using the existing `golang.org/x/sys/unix` dependency. The bridge is covered by the root Go test suite and is not installed with the plugin. The check installs the package without `node_modules`, server entrypoints or a Markdown command, then runs the supplied **real OpenCode CLI** with a private server and isolated config. It exercises clean and annotated review from home, creation of a feedback session, automatic plan review and revision with the reviewed snapshot, and `/revdiff --staged --only="a b.go"` in an existing session. A local model endpoint, fake terminal-control executable and fake revdiff binary provide deterministic boundaries; no external LLM or real emulator overlay is used. Two-client ownership and cancellation, including a terminal process outside the launcher's process group, are covered separately through the CLI entrypoint and real filesystem claims.

## OpenCode v1

### Files

```
~/.config/opencode/
├── commands/
│   └── revdiff.md
├── tools/
│   ├── revdiff.ts
│   └── launch-revdiff.sh
└── plugins/
    ├── revdiff-plan-review.ts
    └── launch-plan-review.sh
```

### Installation

```sh
bash setup.sh
```

The script creates the target directories if needed, copies all files, marks the shell scripts as executable, and registers the plan-review plugin in `~/.config/opencode/opencode.json`. Or manually:

```sh
mkdir -p ~/.config/opencode/commands ~/.config/opencode/tools ~/.config/opencode/plugins
cp ../../.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh ~/.config/opencode/tools/
chmod +x ~/.config/opencode/tools/launch-revdiff.sh
cp ../revdiff-planning/scripts/launch-plan-review.sh ~/.config/opencode/plugins/
chmod +x ~/.config/opencode/plugins/launch-plan-review.sh
cp commands/revdiff.md ~/.config/opencode/commands/
cp tools/revdiff.ts ~/.config/opencode/tools/
cp plugins/revdiff-plan-review.ts ~/.config/opencode/plugins/
```

Then register the plan-review plugin in `~/.config/opencode/opencode.json`:

```json
{
  "plugin": ["./plugins/revdiff-plan-review.ts"]
}
```

Restart OpenCode after installing — tools and commands are loaded at startup.

The tool and plan-review plugin set `REVDIFF_EXIT_CODE_ON_ANNOTATIONS`; exit `10` is success-with-annotations and captured stdout is still processed. Other nonzero statuses remain failures.
