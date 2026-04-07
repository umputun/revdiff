import { tool } from "@opencode-ai/plugin";
import path from "path";

const scriptPath = path.join(
  path.dirname(new URL(import.meta.url).pathname),
  "launch-revdiff.sh",
);

export default tool({
  description:
    "Launch revdiff in a terminal overlay to review git diffs and capture annotations. " +
    "Opens an interactive diff viewer in a split pane (tmux/kitty/wezterm/ghostty/iTerm2/emacs vterm) " +
    "and returns any annotations the user wrote.",
  args: {
    ref: tool.schema
      .string()
      .optional()
      .describe(
        "Git ref to diff against (e.g. HEAD, main, a commit SHA). Omit to diff working tree.",
      ),
    staged: tool.schema
      .boolean()
      .optional()
      .describe("Diff staged changes instead of working tree."),
    only: tool.schema
      .array(tool.schema.string())
      .optional()
      .describe("Limit the diff to these file paths."),
  },

  async execute({ ref, staged, only }, context) {
    const cmdArgs = [
      ...(ref ? [ref] : []),
      ...(staged ? ["--staged"] : []),
      ...(only?.map((f) => `--only=${f}`) ?? []),
    ];

    const result = await Bun.$`bash ${scriptPath} ${cmdArgs}`
      .cwd(context.directory)
      .text();

    return result.trim() || "(no annotations)";
  },
});
