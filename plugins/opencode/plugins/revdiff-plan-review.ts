/**
 * revdiff-plan-review plugin
 *
 * On session.idle, if the last assistant message was in plan mode, launches
 * revdiff in a terminal overlay so the user can annotate the plan. Any
 * annotations are injected back as a user message so the AI can revise.
 *
 * Requires launch-plan-review.sh in ~/.config/opencode/plugins/ and revdiff
 * on $PATH. Supports tmux, kitty, wezterm, cmux, ghostty, iTerm2, emacs.
 */
import type { Plugin } from "@opencode-ai/plugin";
import path from "path";
import os from "os";
import fs from "fs/promises";

const LAUNCHER = path.join(
    os.homedir(),
    ".config/opencode/plugins/launch-plan-review.sh",
);

async function isExecutable(filePath: string): Promise<boolean> {
  try {
    await fs.access(filePath, fs.constants.X_OK);
    return true;
  } catch {
    return false;
  }
}

async function isInstalled(bin: string): Promise<boolean> {
  const p = await Bun.$`which ${bin}`.text().catch(() => "");
  return p.trim().length > 0;
}

async function getLastPlanContent(
    client: any,
    sessionID: string,
): Promise<string | null> {
  const resp = await client.session.messages({ path: { id: sessionID } });
  const messages = resp.data ?? [];

  for (let i = messages.length - 1; i >= 0; i--) {
    const { info, parts } = messages[i];
    if (info.role !== "assistant") continue;
    if ((info as any).mode !== "plan") return null;

    return parts
        .filter((p: any) => p.type === "text")
        .map((p: any) => p.text)
        .join("\n");
  }

  return null;
}

async function launchReview(planFile: string): Promise<string> {
  try {
    return await Bun.$`bash ${LAUNCHER} ${planFile}`.text().then((t) => t.trim());
  } catch {
    return "";
  }
}

async function injectAnnotations(
    client: any,
    sessionID: string,
    annotations: string,
): Promise<void> {
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
}

export const server: Plugin = async ({ client }) => ({
  event: async ({ event }) => {
    if (event.type !== "session.idle") return;

    const { sessionID } = event.properties;

    if (!(await isExecutable(LAUNCHER))) return;
    if (!(await isInstalled("revdiff"))) return;

    let planContent: string | null;
    try {
      planContent = await getLastPlanContent(client, sessionID);
    } catch {
      return;
    }
    if (!planContent?.trim()) return;

    const planFile = `/tmp/revdiff-plan-${sessionID}.md`;
    await Bun.write(planFile, planContent);

    let annotations: string;
    try {
      annotations = await launchReview(planFile);
    } finally {
      await fs.unlink(planFile).catch(() => {});
    }

    if (!annotations) return;

    try {
      await injectAnnotations(client, sessionID, annotations);
    } catch {
      // best-effort
    }
  },
});
