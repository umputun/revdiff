// Exercise the real OpenCode TUI in a private PTY. Only the model endpoint,
// terminal overlay command and revdiff binary are deterministic fixtures.
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import {
  mkdtemp,
  mkdir,
  writeFile,
  readFile,
  readdir,
  rm,
  realpath,
  stat,
} from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { setTimeout as delay } from "node:timers/promises";

const binary = process.argv[2];
assert.ok(
  binary && path.isAbsolute(binary),
  "usage: npm run test:runtime -- /absolute/path/to/opencode-v2",
);
assert.ok(
  process.platform === "darwin" || process.platform === "linux",
  "runtime PTY check requires macOS or Linux",
);
const root = await realpath(
  await mkdtemp(path.join(tmpdir(), "revdiff-cli-runtime-")),
);
const project = path.join(root, "project");
const config = path.join(root, "config", "opencode");
const bin = path.join(root, "bin");
const calls = path.join(root, "calls");
for (const directory of [
  project,
  bin,
  calls,
  path.join(root, "home"),
  path.join(root, "tmp"),
])
  await mkdir(directory);
const env = {
  PATH: `${bin}:${path.dirname(process.execPath)}:${process.env.PATH}`,
  HOME: path.join(root, "home"),
  TMPDIR: path.join(root, "tmp"),
  XDG_CONFIG_HOME: path.join(root, "config"),
  XDG_DATA_HOME: path.join(root, "data"),
  XDG_STATE_HOME: path.join(root, "state"),
  XDG_CACHE_HOME: path.join(root, "cache"),
  TERM: "xterm-256color",
  COLORTERM: "truecolor",
  AGTERM_SESSION_ID: "cli-runtime-fixture",
};
execFileSync(
  "bash",
  [
    fileURLToPath(new URL("../setup.sh", import.meta.url)),
    "--opencode",
    binary,
    config,
  ],
  { env },
);
const version = execFileSync(binary, ["--version"], {
  env,
  encoding: "utf8",
}).trim();
const installed = path.join(config, "plugins", "revdiff");
for (const name of ["index.ts", "server.ts", "rpc.ts", "node_modules"]) {
  assert.equal(
    await stat(path.join(installed, name)).catch(() => undefined),
    undefined,
  );
}
assert.equal(
  await stat(path.join(config, "commands", "revdiff.md")).catch(
    () => undefined,
  ),
  undefined,
);

const notes = "Review fixture note: add verification.";
await writeFile(
  path.join(bin, "agtermctl"),
  '#!/bin/bash\nif [[ "$1 $2 $3" == "session overlay open" ]]; then exec bash -c "$4"; fi\n',
  { mode: 0o755 },
);
await writeFile(
  path.join(bin, "revdiff"),
  `#!/usr/bin/env node
const fs = require("node:fs");
const path = require("node:path");
const calls = ${JSON.stringify(calls)};
const args = process.argv.slice(2);
const count = fs.readdirSync(calls).length + 1;
const output = args.findLast(arg => arg.startsWith("--output=")).slice(9);
const only = args.find(arg => arg.startsWith("--only="))?.slice(7);
const plan = only && path.isAbsolute(only) && fs.existsSync(only) ? fs.readFileSync(only, "utf8") : undefined;
const annotated = count === 2 || count === 3;
fs.writeFileSync(output, annotated ? "## " + only + ":3\\n" + ${JSON.stringify(notes)} : "");
fs.writeFileSync(path.join(calls, count + ".json"), JSON.stringify({args, cwd: process.cwd(), plan}), {flag: "wx"});
process.exit(annotated ? 10 : 0);
`,
  { mode: 0o755 },
);

const planText = "# Plan\n\n1. Add a theme toggle.\n2. Test both themes.";
let annotationObserved = false;
let homeFeedbackObserved = false;
let planSnapshotObserved = false;
let requests = 0;
const model = createServer((request, response) => {
  let body = "";
  request.on("data", (chunk) => {
    body += chunk;
  });
  request.on("end", () => {
    if (!body) {
      response.writeHead(200, { "content-type": "application/json" });
      response.end('{"data":[]}');
      return;
    }
    const input = JSON.parse(body);
    requests++;
    if (body.includes(notes)) {
      annotationObserved = true;
      homeFeedbackObserved ||= body.includes(
        "I reviewed the changes and added annotations.",
      );
      planSnapshotObserved ||=
        input.messages?.some(
          (message: {
            role: string;
            content: string | Array<{ text?: string }>;
          }) => {
            const text =
              typeof message.content === "string"
                ? message.content
                : message.content.map((part) => part.text ?? "").join("\n");
            return (
              message.role === "user" &&
              text.includes(notes) &&
              text.includes(
                "1 | # Plan\n2 | \n3 | 1. Add a theme toggle.\n4 | 2. Test both themes.",
              ) &&
              /temporary.*deleted/i.test(text)
            );
          },
        ) ?? false;
    }
    if (input.stream) {
      response.writeHead(200, { "content-type": "text/event-stream" });
      for (const [delta, finish_reason] of [
        [{ role: "assistant", content: planText }, null],
        [{}, "stop"],
      ]) {
        response.write(
          `data: ${JSON.stringify({ id: "fixture", object: "chat.completion.chunk", created: 1, model: "fixture", choices: [{ index: 0, delta, finish_reason }] })}\n\n`,
        );
      }
      response.end("data: [DONE]\n\n");
    } else {
      response.writeHead(200, { "content-type": "application/json" });
      response.end(
        JSON.stringify({
          id: "fixture",
          object: "chat.completion",
          created: 1,
          model: "fixture",
          choices: [
            {
              index: 0,
              message: { role: "assistant", content: planText },
              finish_reason: "stop",
            },
          ],
          usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
        }),
      );
    }
  });
});
await new Promise<void>((resolve) => model.listen(0, "127.0.0.1", resolve));
const address = model.address();
assert.ok(address && typeof address !== "string");
await writeFile(
  path.join(config, "opencode.json"),
  JSON.stringify({
    model: "fixture/fixture",
    default_agent: "plan",
    update: "disable",
    providers: {
      fixture: {
        package: "@opencode/ai/providers/openai-compatible",
        settings: {
          baseURL: `http://127.0.0.1:${address.port}/v1`,
          apiKey: "fixture",
        },
        models: {
          fixture: { name: "Fixture", limit: { context: 64000, output: 8000 } },
        },
      },
    },
  }),
);

const pty = path.join(root, "runtime-pty");
execFileSync("go", ["build", "-o", pty, "./app/ptybridge"], {
  cwd: fileURLToPath(new URL("../../../", import.meta.url)),
});
const terminal = spawn(pty, [binary, "--standalone", project], {
  cwd: project,
  env,
  stdio: ["pipe", "pipe", "pipe"],
});
let screen = "";
let pluginFailed = false;
let feedbackFailure = false;
terminal.stdout.on("data", (chunk) => {
  screen = (screen + chunk).slice(-6000);
  if (screen.includes("plugin failed")) pluginFailed = true;
  if (screen.includes("Review annotations were not delivered"))
    feedbackFailure = true;
});
terminal.stderr.on("data", (chunk) => {
  screen = (screen + chunk).slice(-6000);
});
const exited = new Promise<void>((resolve) =>
  terminal.once("exit", () => resolve()),
);
async function waitForCalls(count: number) {
  const deadline = Date.now() + 30_000;
  while (
    (await readdir(calls)).length < count &&
    Date.now() < deadline &&
    terminal.exitCode === null
  )
    await delay(100);
  const plain = screen.replace(/\x1b\[[0-?]*[ -/]*[@-~]/g, "");
  assert.ok(
    (await readdir(calls)).length >= count,
    `Expected ${count} CLI launches, received ${(await readdir(calls)).length}, model requests ${requests}. PTY: ${plain}`,
  );
}
try {
  const readyDeadline = Date.now() + 15_000;
  while (!screen.includes("Fixture") && Date.now() < readyDeadline)
    await delay(100);
  terminal.stdin.write('/revdiff --only="home-clean.go"\r');
  await waitForCalls(1);
  await delay(300);
  assert.equal(
    requests,
    0,
    "clean review from home must not create a model turn",
  );
  const homeClean = JSON.parse(
    await readFile(path.join(calls, "1.json"), "utf8"),
  );
  assert.ok(homeClean.args.includes("--only=home-clean.go"));
  assert.equal(homeClean.cwd, project);
  terminal.stdin.write('/revdiff --only="home-notes.go"\r');
  await waitForCalls(4);
  assert.ok(
    homeFeedbackObserved,
    "home review must create a session and deliver its notes to the model",
  );
  assert.ok(
    annotationObserved,
    "review annotations must reach the real plan session/model",
  );
  assert.ok(
    planSnapshotObserved,
    "the annotation user message must include the exact reviewed plan with line numbers, not only a deleted file path",
  );
  const first = JSON.parse(await readFile(path.join(calls, "3.json"), "utf8"));
  const second = JSON.parse(await readFile(path.join(calls, "4.json"), "utf8"));
  assert.equal(first.plan, planText);
  assert.equal(second.plan, planText);
  assert.equal(first.cwd, project);
  await delay(300);
  const before = requests;
  terminal.stdin.write('/revdiff --staged --only="a b.go"\r');
  await waitForCalls(5);
  const manual = JSON.parse(await readFile(path.join(calls, "5.json"), "utf8"));
  assert.ok(
    manual.args.includes("--staged"),
    `manual args: ${JSON.stringify(manual.args)}`,
  );
  assert.ok(manual.args.includes("--only=a b.go"));
  await delay(300);
  assert.equal(
    requests,
    before,
    "clean manual review must not submit a model prompt",
  );
  assert.equal((await readdir(calls)).length, 5);
  assert.equal(pluginFailed, false, "CLI must not report a failed plugin");
  assert.equal(
    feedbackFailure,
    false,
    "accepted feedback must not produce a delivery-failure alert",
  );
  assert.equal(
    await stat(path.join(installed, "node_modules")).catch(() => undefined),
    undefined,
  );
  console.log(
    `PASS (${version}): CLI-only discovery, clean and annotated /revdiff from home, automatic plan review and revision with its snapshot, in-session /revdiff and clean exit without a follow-up prompt`,
  );
} finally {
  terminal.stdin.write("\u0003\u0003");
  await Promise.race([exited, delay(2_000)]);
  if (terminal.exitCode === null && terminal.signalCode === null)
    terminal.kill("SIGTERM");
  await exited;
  model.closeAllConnections();
  model.close();
  await rm(root, { recursive: true, force: true });
}
