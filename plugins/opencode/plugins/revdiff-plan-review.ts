/**
 * revdiff-plan-review plugin
 *
 * Automatically launches revdiff to review the plan whenever the AI finishes
 * responding in plan mode. If the user adds annotations, they are injected
 * back as a follow-up message so the AI can revise its plan.
 *
 * Uses launch-plan-review.sh (must be in ~/.config/opencode/tools/) which
 * handles all terminal types: tmux, kitty, wezterm, cmux, ghostty, iTerm2, emacs.
 */
import type { Plugin } from "@opencode-ai/plugin";
import { $ } from "bun";
import path from "path";
import os from "os";
import fs from "fs/promises";

const LAUNCHER = path.join(
  os.homedir(),
  ".config/opencode/plugins/launch-plan-review.sh",
);

export const server: Plugin = async ({ client }) => {
  return {
    event: async ({ event }) => {
      if (event.type !== "session.idle") return;

      const sessionID = event.properties.sessionID;

      // Check launcher is available
      try {
        await fs.access(LAUNCHER, fs.constants.X_OK);
      } catch {
        return;
      }

      // Check revdiff is installed
      const revdiffPath = await $`which revdiff`
        .text()
        .catch(() => "")
        .then((p) => p.trim());
      if (!revdiffPath) return;

      // Fetch messages to find the last assistant message and check it was in plan mode
      let planContent = "";
      try {
        const resp = await client.session.messages({ path: { id: sessionID } });
        const messages = resp.data ?? [];
        for (let i = messages.length - 1; i >= 0; i--) {
          const { info, parts } = messages[i];
          if (info.role !== "assistant") continue;
          // Only trigger when the last response was in plan mode
          if ((info as any).mode !== "plan") return;
          planContent = parts
            .filter((p: any) => p.type === "text")
            .map((p: any) => p.text)
            .join("\n");
          break;
        }
      } catch {
        return;
      }

      if (!planContent.trim()) return;

      const planFile = `/tmp/revdiff-plan-${sessionID}.md`;
      await Bun.write(planFile, planContent);

      let annotations = "";
      try {
        annotations = await $`bash ${LAUNCHER} ${planFile}`
          .text()
          .then((t) => t.trim());
      } catch {
        // launcher failed or terminal not supported — silently skip
      } finally {
        await fs.unlink(planFile).catch(() => {});
      }

      if (!annotations) return;

      // Inject annotations as a new user message so the AI revises the plan
      try {
        await client.session.prompt({
          path: { id: sessionID },
          body: {
            agent: "plan",
            parts: [
              {
                type: "text",
                text: `I reviewed the plan and added annotations. Please revise the plan to address each one:\n\n${annotations}`,
              },
            ],
          },
        });
      } catch {
        // best-effort
      }
    },
  };
};
