import { createHash } from "node:crypto";
import { mkdir, open, readdir, lstat, unlink } from "node:fs/promises";
import { homedir } from "node:os";
import path from "node:path";

export class ReviewClaims {
  private readonly directory: string;

  constructor(
    directory = path.join(
      process.env.XDG_STATE_HOME || path.join(homedir(), ".local", "state"),
      "revdiff",
      "opencode-plan-review",
    ),
  ) {
    this.directory = directory;
  }

  async claim(eventID: string): Promise<boolean> {
    await mkdir(this.directory, { recursive: true, mode: 0o700 });
    await this.prune();
    const key = createHash("sha256").update(eventID).digest("hex");
    let marker;
    try {
      marker = await open(
        path.join(this.directory, `event-${key}.claim`),
        "wx",
        0o600,
      );
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "EEXIST") return false;
      throw error;
    }
    await marker.close();
    return true;
  }

  private async prune(): Promise<void> {
    const cutoff = Date.now() - 24 * 60 * 60 * 1_000;
    for (const entry of await readdir(this.directory, {
      withFileTypes: true,
    })) {
      if (!entry.isFile() || !/^event-[a-f0-9]{64}\.claim$/.test(entry.name))
        continue;
      const file = path.join(this.directory, entry.name);
      try {
        const info = await lstat(file);
        if (info.isFile() && info.size === 0 && info.mtimeMs < cutoff)
          await unlink(file);
      } catch (error) {
        // A second client may already have removed the same expired marker.
        if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
      }
    }
  }
}
