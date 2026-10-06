import assert from "node:assert/strict";
import { test } from "node:test";
import {
  mkdtemp,
  readFile,
  writeFile,
  mkdir,
  chmod,
  rm,
  realpath,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";
import { Launcher, ReviewError } from "./launcher.ts";

test("preflight requires an executable launcher and revdiff on the launch cwd PATH", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-preflight-"));
  const previous = process.env.PATH;
  process.env.PATH = "bin";
  try {
    const launcher = new Launcher(directory);
    const script = path.join(directory, "launch-revdiff.sh");
    await writeFile(script, "#!/bin/bash\n", { mode: 0o600 });
    await assert.rejects(
      launcher.preflight(directory),
      /launcher.*executable/i,
    );
    await chmod(script, 0o700);
    await assert.rejects(launcher.preflight(directory), /revdiff.*PATH/);
    await mkdir(path.join(directory, "bin"));
    const binary = path.join(directory, "bin", "revdiff");
    await writeFile(binary, "#!/bin/bash\n", { mode: 0o600 });
    await assert.rejects(launcher.preflight(directory), /revdiff.*PATH/);
    await chmod(binary, 0o700);
    await launcher.preflight(directory);
  } finally {
    if (previous === undefined) delete process.env.PATH;
    else process.env.PATH = previous;
    await rm(directory, { recursive: true, force: true });
  }
});

test("launcher failures retain captured notes without becoming successful reviews", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-launch-error-"));
  try {
    await writeFile(
      path.join(directory, "launch-revdiff.sh"),
      "printf 'partial notes'; printf 'terminal unavailable' >&2; exit 2\n",
    );
    await assert.rejects(
      new Launcher(directory).review(
        { directory },
        new AbortController().signal,
      ),
      (error: unknown) => {
        assert.ok(error instanceof ReviewError);
        assert.match(error.message, /terminal unavailable/);
        assert.equal(error.annotations, "partial notes");
        return true;
      },
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("manual arguments preserve ref, staged, quoted filenames and cwd", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-launch-"));
  try {
    await writeFile(
      path.join(directory, "launch-revdiff.sh"),
      `#!/bin/bash
pwd > cwd.txt
printf '%s\\n' "$@" > args.txt
for arg in "$@"; do case "$arg" in --output=*) output="\${arg#--output=}" ;; esac; done
printf 'Please fix this line' > "$output"
exit 10
`,
    );
    const result = await new Launcher(directory).review(
      {
        directory,
        arguments:
          'HEAD~1 --staged --only "file with spaces.go" --only "$(touch unwanted)"',
      },
      new AbortController().signal,
    );
    assert.equal(result, "Please fix this line");
    assert.equal(
      (await readFile(path.join(directory, "cwd.txt"), "utf8")).trim(),
      await realpath(directory),
    );
    const args = (await readFile(path.join(directory, "args.txt"), "utf8"))
      .trim()
      .split("\n");
    assert.deepEqual(args.slice(0, -1), [
      "HEAD~1",
      "--staged",
      "--only=file with spaces.go",
      "--only=$(touch unwanted)",
    ]);
    assert.match(args.at(-1)!, /^--output=/);
    await assert.rejects(readFile(path.join(directory, "unwanted")), {
      code: "ENOENT",
    });
    await assert.rejects(readFile(args.at(-1)!.slice("--output=".length)), {
      code: "ENOENT",
    });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("plan review uses file mode and cleans its plan and output files", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-plan-"));
  try {
    await writeFile(
      path.join(directory, "launch-revdiff.sh"),
      `#!/bin/bash
for arg in "$@"; do
  case "$arg" in --output=*) output="\${arg#--output=}" ;; --only=*) plan="\${arg#--only=}" ;; esac
done
printf '%s' "$plan" > plan-path.txt
cat "$plan" > "$output"
exit 10
`,
    );
    assert.equal(
      await new Launcher(directory).review(
        { directory, plan: "# Plan\n\nA complete plan." },
        new AbortController().signal,
      ),
      "# Plan\n\nA complete plan.",
    );
    const file = await readFile(path.join(directory, "plan-path.txt"), "utf8");
    await assert.rejects(readFile(file), { code: "ENOENT" });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

for (const input of [
  '--only "unterminated',
  "--only",
  "--output=/tmp/other",
  "HEAD main",
]) {
  test(`invalid slash arguments are rejected: ${input}`, async () => {
    await assert.rejects(
      new Launcher("/unused").review(
        { directory: "/unused", arguments: input },
        new AbortController().signal,
      ),
      /argument|quote|option|ref/i,
    );
  });
}

for (const plan of [false, true]) {
  test(
    `unmodified shared launcher preserves ${plan ? "plan" : "diff"} output on cancellation`,
    { timeout: 15_000 },
    async () => {
      const directory = await mkdtemp(path.join(tmpdir(), "revdiff-cancel-"));
      const bin = path.join(directory, "bin");
      const ready = path.join(directory, "ready");
      await mkdir(bin);
      await writeFile(
        path.join(bin, "agtermctl"),
        '#!/bin/bash\nif [[ "$1 $2 $3" == "session overlay open" ]]; then exec bash -c "$4"; fi\n',
        { mode: 0o755 },
      );
      await writeFile(
        path.join(bin, "revdiff"),
        `#!/bin/bash
for arg in "$@"; do case "$arg" in --output=*) output="\${arg#--output=}" ;; esac; done
printf 'Saved review note' > "$output"
printf '%s' "$output" > "$CAPTURE_READY"
sleep 30
`,
        { mode: 0o755 },
      );
      const source = fileURLToPath(
        new URL(
          "../../../.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh",
          import.meta.url,
        ),
      );
      await writeFile(
        path.join(directory, "launch-revdiff.sh"),
        `#!/bin/bash
export PATH="${bin}:$PATH"
export AGTERM_SESSION_ID=fixture
export CAPTURE_READY="${ready}"
exec bash "${source}" "$@"
`,
      );
      const controller = new AbortController();
      const result = new Launcher(directory)
        .review(
          { directory, ...(plan ? { plan: "# Plan" } : {}) },
          controller.signal,
        )
        .then(
          () => undefined,
          (error: unknown) => error,
        );
      try {
        let output: string | undefined;
        const deadline = Date.now() + 10_000;
        while (!output && Date.now() < deadline) {
          output = await readFile(ready, "utf8").catch(() => undefined);
          if (!output) await delay(10);
        }
        assert.ok(output);
        assert.equal(await readFile(output, "utf8"), "Saved review note");
        controller.abort();
        const error = await result;
        assert.ok(error instanceof ReviewError);
        assert.equal(error.annotations, "Saved review note");
        await assert.rejects(readFile(output), { code: "ENOENT" });
      } finally {
        controller.abort();
        await result;
        await rm(directory, { recursive: true, force: true });
      }
    },
  );
}
