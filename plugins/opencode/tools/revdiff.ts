import { tool } from "@opencode-ai/plugin";
import { $ } from "bun";

// Allow common git ref characters; blocks shell metacharacters.
// Covers: branch names, commit hashes, HEAD~N, origin/main, --staged, etc.
const REF_RE = /^[-a-zA-Z0-9_.~^@{}/]+$/;

async function readAnnotations(outputFile: string): Promise<string> {
  try {
    const annotations = await $`cat ${outputFile}`.text();
    await $`rm -f ${outputFile}`;
    if (!annotations.trim()) {
      return "No annotations were made. The diff looks good.";
    }
    return `User annotations from revdiff:\n\n${annotations}`;
  } catch {
    await $`rm -f ${outputFile}`.nothrow().quiet();
    return "revdiff exited with no annotations.";
  }
}

async function waitForSentinel(sentinelFile: string): Promise<boolean> {
  for (let i = 0; i < 600; i++) {
    await Bun.sleep(500);
    const r = await $`test -f ${sentinelFile}`.nothrow().quiet();
    if (r.exitCode === 0) return true;
  }
  return false;
}

export default tool({
  description:
    "Launch revdiff TUI for interactive git diff review. Returns annotations made by the user. Use when asked to review changes, diffs, or annotations.",
  args: {
    ref: tool.schema
      .string()
      .optional()
      .describe(
        "Git ref to diff against, e.g. 'main', 'HEAD~1', '--staged'. Defaults to uncommitted changes.",
      ),
  },
  async execute(args, context) {
    const ref = args.ref ?? "";
    if (ref && !REF_RE.test(ref)) {
      return "Error: invalid ref argument. Use a valid git ref (e.g. 'main', 'HEAD~1', '--staged').";
    }

    // Sanitize sessionID to safe characters for use in file paths and shell scripts.
    const sessionId =
      String(context.sessionID ?? "").replace(/[^a-zA-Z0-9-]/g, "") ||
      crypto.randomUUID();
    const outputFile = `/tmp/revdiff-${sessionId}.txt`;
    const revdiffPath =
      (await $`which revdiff`.text().catch(() => "")).trim() || "revdiff";
    const cmd = ref
      ? `${revdiffPath} ${ref} -o ${outputFile}`
      : `${revdiffPath} -o ${outputFile}`;

    // tmux display-popup -E blocks until the popup closes — no sentinel needed.
    if (process.env.TMUX) {
      try {
        await $`tmux display-popup -E -w 95% -h 95% ${cmd}`;
      } catch (e) {
        return `Error launching revdiff: ${e}`;
      }
      return readAnnotations(outputFile);
    }

    // All other supported terminals launch asynchronously. Write a launch script
    // that touches a sentinel file when revdiff exits, then poll for completion.
    const sentinelFile = `/tmp/revdiff-done-${sessionId}.sentinel`;
    const launchScript = `/tmp/revdiff-launch-${sessionId}.sh`;
    await $`rm -f ${sentinelFile}`;
    await Bun.write(launchScript, `#!/bin/sh\n${cmd}; touch ${sentinelFile}\n`);
    await $`chmod +x ${launchScript}`;

    const cleanup = async () =>
      $`rm -f ${sentinelFile} ${launchScript}`.nothrow().quiet();

    if (process.env.KITTY_LISTEN_ON) {
      try {
        await $`kitty @ launch --type=overlay ${launchScript}`;
      } catch (e) {
        await cleanup();
        return `Error launching revdiff: ${e}`;
      }
    } else if (process.env.WEZTERM_PANE) {
      try {
        await $`wezterm cli split-pane -- ${launchScript}`;
      } catch (e) {
        await cleanup();
        return `Error launching revdiff: ${e}`;
      }
    } else if (process.env.CMUX_SURFACE_ID) {
      try {
        const newSplit = await $`cmux new-split down`.text().catch(() => "");
        const surfaceMatch = newSplit.match(/surface:(\d+)/);
        const surfaceArgs = surfaceMatch ? ["--surface", surfaceMatch[0]] : [];
        await $`cmux send ${[...surfaceArgs, "exec " + launchScript + "\\n"]}`;
      } catch (e) {
        await cleanup();
        return `Error launching revdiff: ${e}`;
      }
    } else if (process.env.TERM_PROGRAM === "ghostty") {
      const appleScript = `
on run argv
    set launchScript to item 1 of argv
    tell application "Ghostty"
        set cfg to new surface configuration
        set command of cfg to launchScript
        set wait after command of cfg to false
        set ft to focused terminal of selected tab of front window
        set newTerm to split ft direction down with configuration cfg
        perform action "toggle_split_zoom" on newTerm
        return id of newTerm
    end tell
end run`;

      let termId: string;
      try {
        termId = await $`osascript -e ${appleScript} -- ${launchScript}`
          .text()
          .then((t) => t.trim());
      } catch (e) {
        await cleanup();
        return `Error launching revdiff via Ghostty AppleScript: ${e}`;
      }

      const done = await waitForSentinel(sentinelFile);
      await cleanup();

      const closeScript = `
on run argv
    tell application "Ghostty" to close terminal id (item 1 of argv)
end run`;
      await $`osascript -e ${closeScript} -- ${termId}`.nothrow().quiet();

      if (!done) {
        await $`rm -f ${outputFile}`.nothrow().quiet();
        return "Error: revdiff timed out after 5 minutes.";
      }
      return readAnnotations(outputFile);
    } else if (process.env.ITERM_SESSION_ID) {
      const appleScript = `
on run argv
    set launchScript to item 1 of argv
    tell application "iTerm2"
        tell current window
            tell current session
                split vertically with default profile command launchScript
            end tell
        end tell
    end tell
end run`;
      try {
        await $`osascript -e ${appleScript} -- ${launchScript}`;
      } catch (e) {
        await cleanup();
        return `Error launching revdiff via iTerm2 AppleScript: ${e}`;
      }
    } else if (process.env.INSIDE_EMACS === "vterm") {
      // launchScript path only contains [a-zA-Z0-9/\-_.] so it is safe in a Lisp string.
      const lispExpr = `(let ((buf (generate-new-buffer "*revdiff*"))) (switch-to-buffer buf) (term "${launchScript}"))`;
      try {
        await $`emacsclient -c -e ${lispExpr}`;
      } catch (e) {
        await cleanup();
        return `Error launching revdiff: ${e}`;
      }
    } else {
      await cleanup();
      return [
        "Error: no supported terminal detected.",
        "Supported terminals: tmux, kitty, wezterm, cmux, Ghostty, iTerm2, Emacs vterm.",
        "Detected env: TERM_PROGRAM=" + (process.env.TERM_PROGRAM ?? "unset"),
      ].join("\n");
    }

    // Wait for sentinel written by the launch script.
    const done = await waitForSentinel(sentinelFile);
    await cleanup();

    if (!done) {
      await $`rm -f ${outputFile}`;
      return "Error: revdiff timed out after 5 minutes.";
    }
    return readAnnotations(outputFile);
  },
});
