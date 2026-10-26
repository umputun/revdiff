# Installation

**Homebrew (macOS/Linux):**
```bash
brew install umputun/apps/revdiff
```

**Go:**

```bash
go install github.com/umputun/revdiff/app/revdiff@latest
```

Installs the `revdiff` binary into `GOBIN` (defaults to `$(go env GOPATH)/bin`). Add that directory to your `PATH`.

**Binary releases:** download from [GitHub Releases](https://github.com/umputun/revdiff/releases) (deb, rpm, archives for linux/darwin amd64/arm64).

## Claude Code Plugin

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff@revdiff
```

Use: `/revdiff [base] [against]` — opens review session in a terminal overlay (agterm, tmux, Zellij, herdr, kitty, wezterm, Kaku, cmux, ghostty, iTerm2, or Emacs vterm).

cmux is detected before ghostty when `$CMUX_SURFACE_ID` is set, `__CFBundleIdentifier=com.cmuxterm.app`, or `GHOSTTY_RESOURCES_DIR` / `GHOSTTY_BIN_DIR` contains `cmux.app`, so cmux uses the cmux CLI instead of Ghostty AppleScript.

**Sandbox note:** Ghostty and iTerm2 launchers use `osascript` (Apple Events), which is blocked by Claude Code's sandbox. If you use these terminals with sandbox enabled, add to your Claude Code `settings.json`:

```json
{
  "sandbox": {
    "excludedCommands": ["*/launch-revdiff.sh*"]
  }
}
```

Claude Code exempts a command only when every part of it matches, so the skill runs the launcher by its resolved path as a plain command, and passes a diff to review as `--stdin-file=PATH` instead of piping it in.

Terminals using CLI tools (agterm, tmux, Zellij, herdr, kitty, wezterm, Kaku, cmux) are not affected.

### Plan Review Plugin

Automatically opens revdiff when Claude exits plan mode for interactive annotation:

```bash
/plugin marketplace add umputun/revdiff
/plugin install revdiff-planning@revdiff
```

### Overrides

The diff-review skill resolves `launch-revdiff.sh` through a two-layer chain (first-found wins). Drop your own launcher into the user layer to customize how revdiff opens (separate window, alternate split layout, custom terminal multiplexer) without forking the plugin.

| Layer | Path | Scope |
|---|---|---|
| User | `${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh` | every project (per-user, lives under `~/.claude/plugins/data/<plugin-id>/`) |
| Bundled | `${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh` | default — ships with the plugin, used when no override is present |

There is no project-level (`.claude/...`) override layer by design: the resolver is shared with the `revdiff-planning` hook, which fires automatically on every `ExitPlanMode`, and a repo-controlled executable layer would let an untrusted repo run arbitrary code on routine Claude actions. The diff-review skill keeps the same two-layer shape for symmetry.

The override file must be **executable** (`chmod +x`). A non-executable file in the user layer is treated as absent — the resolver falls through to the bundled default rather than erroring. Using `chmod -x` is a quick way to disable an override without deleting the file.

To start from the bundled launcher as a template:

```bash
mkdir -p "${CLAUDE_PLUGIN_DATA}/scripts"
cp "${CLAUDE_SKILL_DIR}/scripts/launch-revdiff.sh" "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh"
chmod +x "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh"
# edit "${CLAUDE_PLUGIN_DATA}/scripts/launch-revdiff.sh" to taste
```

**Customizing the launcher**: production overrides should start from a copy of the bundled `launch-revdiff.sh` and modify only the terminal branch you care about. The bundled launcher already handles the tricky parts — shell-quoting positional args via the `sq()` helper, output-file lifecycle, sentinel polling, env-var propagation. Writing a thin wrapper from scratch is fragile: naive use of `$*` or `$@` inside `sh -c "..."` does not preserve arguments containing spaces, quotes, or globs.

For example, to open revdiff in a fresh kitty window instead of an overlay, copy the bundled launcher and replace the existing `kitty @ launch` overlay block with a `kitty --detach --title "revdiff"` invocation, reusing the existing `$REVDIFF_CMD` variable (which is already correctly quoted) — do **not** rebuild the command line from `$*`.

The override receives the same positional arguments the bundled launcher does (`[base] [against] [--staged] [--only=file1] [--all-files] [--exclude=prefix] [--description=text|--description-file=path]`), plus `--stdin-file=PATH` when the skill reviews a diff from a file and `--dir=PATH` when it reviews a repository other than the current directory. `--stdin-file` and `--dir` are launcher options, not revdiff flags: an override copied from an older bundled launcher passes them through to revdiff, which rejects them as unknown flags. Refresh the override from the current bundled launcher and reapply your terminal customization, or port them by hand: for `--stdin-file`, its argument handling and its file-to-spool branch; for `--dir`, the `--dir`/`-C` argument loop with the `cd` that follows it, the `_RD_SCRIPT_DIR` resolution placed ahead of that loop, and the `cd` in the cmux branch's launch script. The bundled launcher sets `REVDIFF_EXIT_CODE_ON_ANNOTATIONS` internally; custom launchers should do the same, print captured annotations to stdout, preserve exit `10` as success-with-annotations, and relay revdiff's stderr to their own stderr on any exit code other than `0` or `10` so the failure text reaches the caller. Other nonzero statuses remain launcher failures.

The bundled launcher does the relay by capturing revdiff's stderr to a temp file and replaying it on a failing exit. An override that adds its own `EXIT` trap must name that temp file in the trap, or a `revdiff-err-*` file is left behind in `$TMPDIR` on every run.

**Failure mode**: if the resolver finds no launcher in any layer (user / bundled), it exits nonzero with `error: launcher not found in override chain: launch-revdiff.sh` on stderr and the skill stops before launching — check that the override file is present and executable in one of the two layers above.
