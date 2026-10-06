import { spawn } from "node:child_process";
import {
  mkdtemp,
  writeFile,
  readFile,
  rm,
  access,
  stat,
  constants,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

type ReviewInput = { directory: string; arguments?: string; plan?: string };
type ProcessResult = { code: number | null; stdout: string; stderr: string };

export class ReviewError extends Error {
  readonly annotations: string;
  readonly detached: boolean;

  constructor(message: string, annotations: string, detached = false) {
    super(message);
    this.name = "ReviewError";
    this.annotations = annotations;
    this.detached = detached;
  }
}

export class Launcher {
  private readonly directory: string;

  constructor(directory: string) {
    this.directory = directory;
  }

  async preflight(cwd: string): Promise<void> {
    const script = path.join(this.directory, "launch-revdiff.sh");
    if (!(await this.executable(script))) {
      throw new Error(`revdiff launcher is not executable: ${script}`);
    }
    for (const directory of (process.env.PATH ?? "").split(path.delimiter)) {
      if (await this.executable(path.resolve(cwd, directory, "revdiff")))
        return;
    }
    throw new Error("revdiff was not found as an executable on PATH");
  }

  async review(request: ReviewInput, signal: AbortSignal): Promise<string> {
    signal.throwIfAborted();
    let args = this.parseArguments(request.arguments ?? "");
    const temporary = await mkdtemp(path.join(tmpdir(), "revdiff-review-"));
    try {
      if (request.plan !== undefined) {
        const file = path.join(temporary, "plan.md");
        await writeFile(file, request.plan, { mode: 0o600 });
        args = [`--only=${file}`, "--wrap"];
      }
      // The last --output wins. Keeping this file in the CLI's own directory
      // preserves flushed notes even when the unchanged launcher removes its file.
      const output = path.join(temporary, "annotations.txt");
      await writeFile(output, "", { mode: 0o600 });
      const result = await this.run(
        "launch-revdiff.sh",
        [...args, `--output=${output}`],
        request.directory,
        signal,
      );
      const annotations =
        (await readFile(output, "utf8")).trim() || result.stdout.trim();
      if (signal.aborted)
        throw new ReviewError(
          "Review cancelled. A terminal-owned review may still be open, but it is detached from OpenCode and its output directory has been removed. Finish the review normally to save any remaining notes to revdiff history; they will not be delivered automatically.",
          annotations,
          true,
        );
      if (result.code !== 0 && result.code !== 10)
        throw new ReviewError(
          result.stderr.trim() ||
            `revdiff launcher exited with code ${result.code}`,
          annotations,
        );
      return annotations;
    } finally {
      await rm(temporary, { recursive: true, force: true });
    }
  }

  private parseArguments(input: string): string[] {
    const words: string[] = [];
    let word = "";
    let quote = "";
    let escaped = false;
    let started = false;
    for (const char of input) {
      if (escaped) {
        word += char;
        escaped = false;
        started = true;
      } else if (char === "\\" && quote !== "'") {
        escaped = true;
        started = true;
      } else if (quote) {
        if (char === quote) quote = "";
        else word += char;
      } else if (char === "'" || char === '"') {
        quote = char;
        started = true;
      } else if (/\s/.test(char)) {
        if (started) words.push(word);
        word = "";
        started = false;
      } else {
        word += char;
        started = true;
      }
    }
    if (quote || escaped)
      throw new Error("Unfinished quote or escape in /revdiff arguments");
    if (started) words.push(word);
    const args: string[] = [];
    let ref = false;
    for (let index = 0; index < words.length; index++) {
      const token = words[index];
      if (token === "--staged") args.push(token);
      else if (token === "--only" || token.startsWith("--only=")) {
        const file = token === "--only" ? words[++index] : token.slice(7);
        if (!file) throw new Error("--only requires a filename argument");
        args.push(`--only=${file}`);
      } else {
        if (token.startsWith("-"))
          throw new Error(`Unknown /revdiff option: ${token}`);
        if (ref) throw new Error("Only one ref argument is supported");
        ref = true;
        args.push(token);
      }
    }
    return args;
  }

  private async executable(file: string): Promise<boolean> {
    try {
      await access(file, constants.X_OK);
      return (await stat(file)).isFile();
    } catch (error) {
      if (
        ["ENOENT", "EACCES", "ENOTDIR"].includes(
          (error as NodeJS.ErrnoException).code ?? "",
        )
      )
        return false;
      throw error;
    }
  }

  private run(
    script: string,
    args: string[],
    cwd: string,
    signal: AbortSignal,
  ): Promise<ProcessResult> {
    signal.throwIfAborted();
    return new Promise((resolve, reject) => {
      const child = spawn(
        "bash",
        [path.join(this.directory, script), ...args],
        {
          cwd,
          detached: true,
          stdio: ["ignore", "pipe", "pipe"],
        },
      );
      let killTimer: ReturnType<typeof setTimeout> | undefined;
      const kill = (termination: NodeJS.Signals) => {
        if (!child.pid) return;
        try {
          process.kill(-child.pid, termination);
        } catch (error) {
          if ((error as NodeJS.ErrnoException).code !== "ESRCH") reject(error);
        }
      };
      const cancel = () => {
        kill("SIGTERM");
        killTimer = setTimeout(() => kill("SIGKILL"), 1_000);
      };
      signal.addEventListener("abort", cancel, { once: true });
      let stdout = "";
      let stderr = "";
      child.stdout.setEncoding("utf8").on("data", (data: string) => {
        stdout += data;
      });
      child.stderr.setEncoding("utf8").on("data", (data: string) => {
        stderr += data;
      });
      child.on("error", reject);
      child.on("close", (code) => {
        signal.removeEventListener("abort", cancel);
        clearTimeout(killTimer);
        resolve({ code, stdout, stderr });
      });
    });
  }
}
