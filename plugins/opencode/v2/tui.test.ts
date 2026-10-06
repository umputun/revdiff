import assert from "node:assert/strict";
import { test, before, after, beforeEach } from "node:test";
import { randomUUID } from "node:crypto";
import {
  mkdtemp,
  mkdir,
  rm,
  readdir,
  readFile,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import {
  setTimeout as delay,
  setImmediate as nextTurn,
} from "node:timers/promises";
import type { Plugin } from "@opencode/plugin/tui";
import type { KeymapCommand } from "@opencode/plugin/tui/context";
import { Launcher, ReviewError } from "./launcher.ts";
import { ReviewClaims } from "./claims.ts";
import plugin from "./tui.ts";

let state: string;
let previousState: string | undefined;
before(async () => {
  state = await mkdtemp(path.join(tmpdir(), "revdiff-cli-state-"));
  previousState = process.env.XDG_STATE_HOME;
  process.env.XDG_STATE_HOME = state;
});
after(async () => {
  if (previousState === undefined) delete process.env.XDG_STATE_HOME;
  else process.env.XDG_STATE_HOME = previousState;
  await rm(state, { recursive: true, force: true });
});

beforeEach((test) => {
  assert.ok("mock" in test);
  test.mock.method(Launcher.prototype, "preflight", async () => {});
});

function client(
  sessionID: string = randomUUID(),
  eventID: string = randomUUID(),
) {
  const session = {
    id: sessionID,
    projectID: "project",
    agent: "plan",
    outcome: "succeeded",
    location: { directory: "/project" },
    parentID: undefined as string | undefined,
  };
  let displayed: string | undefined = sessionID;
  const messages: any[] = [
    {
      id: "plan-a",
      type: "assistant",
      agent: "plan",
      finish: "stop",
      time: { completed: 2 },
      content: [{ type: "text", text: "# Plan\n\nImplement and test." }],
    },
    { type: "idle", outcome: "succeeded" },
  ];
  const listeners = new Map<string, (event: any) => unknown>();
  const commands: KeymapCommand[] = [];
  const prompts: any[] = [];
  const created: any[] = [];
  const alerts: any[] = [];
  let syncedMessages: any[] = [];
  const messageAPI = {
    sync: async (_sessionID?: string) => {
      syncedMessages = structuredClone(messages);
    },
    list: (_sessionID?: string) => syncedMessages,
  };
  const api = {
    create: async (input: any) => {
      const result = {
        ...structuredClone(session),
        ...input,
        agent: input.agent,
        id: randomUUID(),
      };
      created.push(result);
      return result;
    },
    get: async (_input?: unknown, _options?: { signal?: AbortSignal }) =>
      structuredClone(session),
    context: async (_input?: unknown, _options?: { signal?: AbortSignal }) =>
      structuredClone(messages),
    prompt: async (input: unknown, options?: { signal?: AbortSignal }) => {
      options?.signal?.throwIfAborted();
      prompts.push(input);
    },
  };
  const ctx = {
    options: {},
    location: session.location,
    client: {
      session: api,
      config: {
        get: async () => [
          { type: "document", info: { default_agent: "build" } },
          { type: "directory", path: "/project" },
          { type: "document", info: { default_agent: "plan" } },
        ],
      },
    },
    data: {
      location: { default: () => session.location },
      session: { message: messageAPI },
      on: (type: string, handler: (event: any) => unknown) => {
        listeners.set(type, handler);
        return () => listeners.delete(type);
      },
    },
    keymap: {
      layer: (get: () => { commands: KeymapCommand[] }) =>
        commands.push(...get().commands),
    },
    ui: {
      slot: (spec: { render: () => unknown }) => {
        spec.render();
        return () => {
          commands.length = 0;
        };
      },
      router: {
        current: () =>
          displayed
            ? { type: "session", sessionID: displayed }
            : { type: "home" },
        navigate: (route: { type: string; sessionID?: string }) => {
          displayed = route.sessionID;
        },
      },
      model: {
        current: () => ({ providerID: "fixture", modelID: "selected-model" }),
      },
      toast: {
        show: (input: unknown) => {
          alerts.push(input);
        },
      },
      dialog: {
        alert: async (input: unknown) => {
          alerts.push(input);
        },
      },
    },
  } as unknown as Plugin.Context;
  return {
    ctx,
    session,
    messages,
    api,
    messageAPI,
    commands,
    prompts,
    created,
    alerts,
    listeners,
    navigate(id?: string) {
      displayed = id;
    },
    emit(type: string, id = sessionID, data: Record<string, unknown> = {}) {
      return listeners.get(type)?.({
        id: eventID,
        type,
        data: { sessionID: id, ...data },
        location: session.location,
      });
    },
  };
}

test("CLI /revdiff opens locally and returns annotations to its captured session", async (test) => {
  const terminal = client();
  const launches: any[] = [];
  test.mock.method(Launcher.prototype, "review", async (request: unknown) => {
    launches.push(request);
    terminal.navigate("another-session");
    return "Fix the race";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    const command = terminal.commands.find(
      (command) => command.slash?.name === "revdiff",
    );
    assert.ok(command, "CLI must register /revdiff directly");
    await command.run('HEAD~1 --staged --only="a b.go"');
    assert.equal(launches[0]?.directory, "/project");
    assert.equal(launches[0]?.arguments, 'HEAD~1 --staged --only="a b.go"');
    assert.equal(terminal.prompts[0]?.sessionID, terminal.session.id);
    assert.match(terminal.prompts[0]?.text, /Fix the race/);
  } finally {
    await dispose?.();
  }
});

test("manual review from a subagent session reports why it cannot launch", async (test) => {
  const terminal = client();
  terminal.session.parentID = "parent-session";
  let launches = 0;
  test.mock.method(Launcher.prototype, "review", async () => {
    launches++;
    return "";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.commands[0].run();
    assert.equal(launches, 0);
    assert.deepEqual(terminal.prompts, []);
    assert.equal(terminal.alerts.length, 1);
    assert.equal(terminal.alerts[0].variant, "error");
    assert.match(terminal.alerts[0].message, /root session/i);
  } finally {
    await dispose?.();
  }
});

test("manual review from home opens locally and a clean exit leaves home unchanged", async (test) => {
  const terminal = client();
  terminal.navigate();
  const launches: any[] = [];
  test.mock.method(Launcher.prototype, "review", async (request: unknown) => {
    launches.push(request);
    return "";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    const command = terminal.commands[0];
    assert.equal(
      typeof command.enabled === "function"
        ? command.enabled()
        : command.enabled,
      true,
    );
    await command.run("HEAD~1");
    assert.equal(launches[0]?.directory, "/project");
    assert.equal(launches[0]?.arguments, "HEAD~1");
    assert.deepEqual(terminal.created, []);
    assert.deepEqual(terminal.prompts, []);
    assert.equal(terminal.ctx.ui.router.current().type, "home");
  } finally {
    await dispose?.();
  }
});

for (const home of [false, true]) {
  test(`manual review works without the newer model-selection API; home: ${home}`, async (test) => {
    const terminal = client();
    if (home) terminal.navigate();
    delete (terminal.ctx.ui as unknown as Record<string, unknown>).model;
    test.mock.method(
      Launcher.prototype,
      "review",
      async () => "Legacy CLI notes",
    );
    const dispose = await plugin.setup(terminal.ctx);
    try {
      await terminal.commands[0].run();
      assert.equal(terminal.prompts.length, 1);
      assert.match(terminal.prompts[0].text, /Legacy CLI notes/);
      if (home) {
        assert.equal(terminal.created.length, 1);
        assert.equal(
          terminal.created[0].model,
          undefined,
          "the server resolves its configured model when the CLI cannot expose a selection",
        );
      }
    } finally {
      await dispose?.();
    }
  });
}

for (const switchView of [false, true]) {
  test(`home review notes create their own session; switched view: ${switchView}`, async (test) => {
    const terminal = client();
    terminal.navigate();
    test.mock.method(Launcher.prototype, "review", async () => {
      if (switchView) terminal.navigate("other-session");
      return "Notes from home";
    });
    const dispose = await plugin.setup(terminal.ctx);
    try {
      await terminal.commands[0].run();
      assert.equal(terminal.created.length, 1);
      assert.equal(
        terminal.created[0].agent,
        "plan",
        "the configured default agent must be recorded in the new session",
      );
      assert.deepEqual(terminal.created[0].location, { directory: "/project" });
      assert.deepEqual(terminal.created[0].model, {
        providerID: "fixture",
        id: "selected-model",
      });
      assert.equal(terminal.prompts[0]?.sessionID, terminal.created[0].id);
      assert.match(terminal.prompts[0]?.text, /Notes from home/);
      assert.deepEqual(terminal.ctx.ui.router.current(), {
        type: "session",
        sessionID: switchView ? "other-session" : terminal.created[0].id,
      });
    } finally {
      await dispose?.();
    }
  });
}

test("two CLI clients showing the same plan open one review, and clean exit keeps plan mode", async (test) => {
  const eventID = randomUUID();
  const first = client(undefined, eventID);
  const second = client(first.session.id, eventID);
  const launches: unknown[] = [];
  test.mock.method(Launcher.prototype, "review", async (request: unknown) => {
    launches.push(request);
    return "";
  });
  const cleanup = await Promise.all([
    plugin.setup(first.ctx),
    plugin.setup(second.ctx),
  ]);
  try {
    await Promise.all([
      first.emit("session.execution.succeeded"),
      second.emit("session.execution.succeeded"),
    ]);
    assert.equal(launches.length, 1);
    assert.equal(
      (launches[0] as { plan: string }).plan,
      "# Plan\n\nImplement and test.",
    );
    for (const terminal of [first, second]) {
      assert.deepEqual(terminal.prompts, []);
      assert.deepEqual(terminal.alerts, []);
    }
    await Promise.all([
      first.emit("session.execution.succeeded"),
      second.emit("session.execution.succeeded"),
    ]);
    assert.equal(launches.length, 1);
    for (const terminal of [first, second]) {
      assert.deepEqual(terminal.prompts, []);
      assert.deepEqual(terminal.alerts, []);
    }
  } finally {
    for (const close of cleanup) await close?.();
  }
});

test("one completion event has one owner even with different message snapshots", async (test) => {
  const eventID = randomUUID();
  const first = client(undefined, eventID);
  const second = client(first.session.id, eventID);
  second.messages[0].id = "different-snapshot";
  const launches: unknown[] = [];
  test.mock.method(Launcher.prototype, "review", async (request: unknown) => {
    launches.push(request);
    return "";
  });
  const cleanup = await Promise.all([
    plugin.setup(first.ctx),
    plugin.setup(second.ctx),
  ]);
  try {
    await Promise.all([
      first.emit("session.execution.succeeded"),
      second.emit("session.execution.succeeded"),
    ]);
    assert.equal(launches.length, 1);
    for (const terminal of [first, second]) {
      assert.deepEqual(terminal.prompts, []);
      assert.deepEqual(terminal.alerts, []);
    }
    await Promise.all([
      first.emit("session.execution.succeeded"),
      second.emit("session.execution.succeeded"),
    ]);
    assert.equal(launches.length, 1);
    for (const terminal of [first, second]) {
      assert.deepEqual(terminal.prompts, []);
      assert.deepEqual(terminal.alerts, []);
    }
  } finally {
    for (const close of cleanup) await close?.();
  }
});

test("plan annotations target the original session after the client changes its view", async (test) => {
  const terminal = client();
  test.mock.method(Launcher.prototype, "review", async () => {
    terminal.navigate("another-session");
    return "Add verification";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(terminal.prompts[0]?.sessionID, terminal.session.id);
    assert.match(terminal.prompts[0]?.text, /Add verification/);
    assert.deepEqual(terminal.alerts, []);
  } finally {
    await dispose?.();
  }
});

test("feedback admission before the prompt HTTP reply is not cancelled by its own execution", async (test) => {
  const terminal = client();
  test.mock.method(Launcher.prototype, "review", async () => "Accepted notes");
  test.mock.method(
    terminal.api,
    "prompt",
    async (input: any, options?: { signal?: AbortSignal }) => {
      terminal.prompts.push(input);
      terminal.emit("session.inbox.enqueued", terminal.session.id, {
        inboxID: input.id,
      });
      terminal.emit("session.execution.started");
      await nextTurn();
      options?.signal?.throwIfAborted();
    },
  );
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(terminal.prompts.length, 1);
    assert.deepEqual(
      terminal.alerts,
      [],
      "accepted notes must not be reported as undelivered",
    );
    assert.match(terminal.prompts[0].id, /^msg_/);
  } finally {
    await dispose?.();
  }
});

test("another inbox item does not exempt pending plan feedback from cancellation", async (test) => {
  const terminal = client();
  test.mock.method(Launcher.prototype, "review", async () => "Unsent notes");
  test.mock.method(
    terminal.api,
    "prompt",
    async (input: unknown, options?: { signal?: AbortSignal }) => {
      terminal.emit("session.inbox.enqueued", terminal.session.id, {
        inboxID: "msg_other",
      });
      terminal.emit("session.execution.started");
      options?.signal?.throwIfAborted();
      terminal.prompts.push(input);
    },
  );
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.deepEqual(terminal.prompts, []);
    assert.match(terminal.alerts[0]?.message, /Unsent notes/);
  } finally {
    await dispose?.();
  }
});

test("an admitted feedback message is not reported lost when its HTTP response fails", async (test) => {
  const terminal = client();
  test.mock.method(
    Launcher.prototype,
    "review",
    async () => "Already admitted notes",
  );
  test.mock.method(terminal.api, "prompt", async (input: any) => {
    terminal.prompts.push(input);
    terminal.emit("session.inbox.enqueued", terminal.session.id, {
      inboxID: input.id,
    });
    throw new Error("HTTP reply lost after admission");
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(terminal.prompts.length, 1);
    assert.deepEqual(terminal.alerts, []);
  } finally {
    await dispose?.();
  }
});

test("plan annotations include the reviewed lines after the real launcher deletes its temporary file", async (test) => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-plan-context-"));
  const terminal = client();
  terminal.session.location.directory = directory;
  terminal.messages[0].content[0].text = [
    "# План",
    "",
    "## Тема",
    "```css",
    "  :root { color-scheme: light dark; }",
    "```",
    "Получить настройку через GET /me.",
    "",
    "## Хранение",
    "Сохранить в localStorage.",
  ].join("\n");
  await writeFile(
    path.join(directory, "launch-revdiff.sh"),
    `#!/bin/bash
for arg in "$@"; do
  case "$arg" in --only=*) plan="\${arg#--only=}" ;; --output=*) output="\${arg#--output=}" ;; esac
done
printf '%s' "$plan" > reviewed-plan-path.txt
printf '## %s:7\nUse backend preferences.\n\n## %s:10\nRemove localStorage.\n' "$plan" "$plan" > "$output"
exit 10
`,
  );
  const original = Launcher.prototype.review;
  test.mock.method(
    Launcher.prototype,
    "review",
    (request: Parameters<Launcher["review"]>[0], signal: AbortSignal) =>
      original.call(new Launcher(directory), request, signal),
  );
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    const planFile = await readFile(
      path.join(directory, "reviewed-plan-path.txt"),
      "utf8",
    );
    await assert.rejects(readFile(planFile), { code: "ENOENT" });
    const prompt = terminal.prompts[0];
    assert.equal(prompt?.sessionID, terminal.session.id);
    const expectedSnapshot =
      "1 | # План\n2 | \n3 | ## Тема\n4 | ```css\n5 |   :root { color-scheme: light dark; }\n6 | ```\n7 | Получить настройку через GET /me.\n8 | \n9 | ## Хранение\n10 | Сохранить в localStorage.";
    assert.ok(
      prompt.text.includes(expectedSnapshot),
      "the follow-up must carry the exact reviewed lines, including blanks and indentation",
    );
    assert.match(prompt.text, /temporary.*deleted/i);
    assert.ok(
      prompt.text.includes(`## ${planFile}:7\nUse backend preferences.`),
    );
    assert.ok(prompt.text.includes(`## ${planFile}:10\nRemove localStorage.`));
  } finally {
    await dispose?.();
    await rm(directory, { recursive: true, force: true });
  }
});

test("automatic review reads freshly synced messages rather than session context", async (test) => {
  const terminal = client();
  const stale = structuredClone(terminal.messages);
  terminal.messages[0].content[0].text = "# Fresh plan";
  test.mock.method(terminal.api, "context", async () => stale);
  const launches: any[] = [];
  test.mock.method(Launcher.prototype, "review", async (input: unknown) => {
    launches.push(input);
    return "";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(launches[0]?.plan, "# Fresh plan");
  } finally {
    await dispose?.();
  }
});

test("a client without an executable launcher does not consume a claim", async (test) => {
  test.mock.restoreAll();
  const terminal = client();
  let launches = 0;
  test.mock.method(Launcher.prototype, "review", async () => {
    launches++;
    return "";
  });
  const directory = path.join(state, "revdiff", "opencode-plan-review");
  const before = await readdir(directory).catch(() => []);
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(launches, 0);
    assert.deepEqual(await readdir(directory).catch(() => []), before);
    assert.equal(terminal.alerts[0]?.variant, "error");
    assert.match(terminal.alerts[0]?.message, /launcher/i);
  } finally {
    await dispose?.();
  }
});

for (const stage of ["preflight", "claim"] as const) {
  test(`changing the displayed session during ${stage} never opens a stale overlay`, async (test) => {
    const terminal = client();
    let entered!: () => void;
    let release!: () => void;
    const reached = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    if (stage === "preflight") {
      test.mock.method(Launcher.prototype, "preflight", async () => {
        entered();
        await blocked;
      });
    } else {
      const claim = ReviewClaims.prototype.claim;
      test.mock.method(
        ReviewClaims.prototype,
        "claim",
        async function (this: ReviewClaims, eventID: string) {
          const won = await claim.call(this, eventID);
          entered();
          await blocked;
          return won;
        },
      );
    }
    let launches = 0;
    test.mock.method(Launcher.prototype, "review", async () => {
      launches++;
      return "";
    });
    const directory = path.join(state, "revdiff", "opencode-plan-review");
    const before = await readdir(directory).catch(() => []);
    const dispose = await plugin.setup(terminal.ctx);
    try {
      const review = terminal.emit("session.execution.succeeded");
      await reached;
      terminal.navigate("other-session");
      release();
      await review;
      assert.equal(launches, 0);
      if (stage === "preflight")
        assert.deepEqual(await readdir(directory).catch(() => []), before);
    } finally {
      release();
      await dispose?.();
    }
  });
}

for (const notes of ["", "Saved notes"]) {
  test(`a failed launcher shows a toast and preserves captured notes: ${Boolean(notes)}`, async (test) => {
    const terminal = client();
    let launches = 0;
    test.mock.method(Launcher.prototype, "review", async () => {
      launches++;
      throw new ReviewError("launcher exited with code 2", notes);
    });
    const dispose = await plugin.setup(terminal.ctx);
    try {
      await terminal.emit("session.execution.succeeded");
      const toasts = terminal.alerts.filter((item) => item.variant === "error");
      assert.equal(toasts.length, 1);
      assert.match(toasts[0].message, /code 2/);
      if (notes)
        assert.ok(
          terminal.alerts.some(
            (item) => item.title && item.message.includes(notes),
          ),
        );
      assert.deepEqual(terminal.prompts, []);
      await terminal.emit("session.execution.succeeded");
      assert.equal(launches, 1, "a failed review does not release its claim");
    } finally {
      await dispose?.();
    }
  });
}

test("claim storage failure shows a toast and never launches", async (test) => {
  const terminal = client();
  const blocked = path.join(state, randomUUID());
  await writeFile(blocked, "not a state directory");
  const previous = process.env.XDG_STATE_HOME;
  process.env.XDG_STATE_HOME = blocked;
  let dispose;
  try {
    dispose = await plugin.setup(terminal.ctx);
  } finally {
    process.env.XDG_STATE_HOME = previous;
  }
  let launches = 0;
  test.mock.method(Launcher.prototype, "review", async () => {
    launches++;
    return "";
  });
  try {
    await terminal.emit("session.execution.succeeded");
    assert.equal(launches, 0);
    assert.equal(terminal.alerts[0]?.variant, "error");
    assert.match(terminal.alerts[0]?.message, /ENOTDIR/);
  } finally {
    await dispose?.();
  }
});

test("connecting or switching a CLI view does not sweep existing completed plans", async (test) => {
  const terminal = client();
  let launches = 0;
  test.mock.method(Launcher.prototype, "review", async () => {
    launches++;
    return "";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    await nextTurn();
    terminal.navigate("other-session");
    terminal.navigate(terminal.session.id);
    await nextTurn();
    assert.equal(launches, 0);
    await terminal.emit("session.execution.succeeded");
    assert.equal(launches, 1);
  } finally {
    await dispose?.();
  }
});

for (const scenario of [
  "background",
  "subagent",
  "build",
  "truncated",
  "new user",
] as const) {
  test(`automatic plan review ignores ${scenario}`, async (test) => {
    const terminal = client();
    let launches = 0;
    test.mock.method(Launcher.prototype, "review", async () => {
      launches++;
      return "";
    });
    if (scenario === "background") terminal.navigate("another-session");
    if (scenario === "subagent") terminal.session.parentID = "parent";
    if (scenario === "build") terminal.session.agent = "build";
    if (scenario === "truncated") terminal.messages[0].finish = "length";
    if (scenario === "new user") terminal.messages.push({ type: "user" });
    const dispose = await plugin.setup(terminal.ctx);
    try {
      await terminal.emit("session.execution.succeeded");
      assert.equal(launches, 0);
      assert.deepEqual(terminal.prompts, []);
      assert.deepEqual(terminal.alerts, []);
    } finally {
      await dispose?.();
    }
  });
}

test("a session change invalidates initial plan lookup", async (test) => {
  const terminal = client();
  let entered!: () => void;
  let release!: () => void;
  const reached = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const blocked = new Promise<void>((resolve) => {
    release = resolve;
  });
  const sync = terminal.messageAPI.sync;
  test.mock.method(terminal.messageAPI, "sync", async () => {
    await sync();
    entered();
    await blocked;
  });
  let launches = 0;
  test.mock.method(Launcher.prototype, "review", async () => {
    launches++;
    return "";
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    assert.ok(terminal.listeners.has("session.execution.succeeded"));
    const review = terminal.emit("session.execution.succeeded");
    await reached;
    terminal.emit("session.agent.selected");
    release();
    await review;
    assert.equal(launches, 0);
    assert.deepEqual(terminal.prompts, []);
    assert.deepEqual(terminal.alerts, []);
  } finally {
    release();
    await dispose?.();
  }
});

test("cleanup releases subscriptions without waiting for the notes dialog", async (test) => {
  const terminal = client();
  let entered!: () => void;
  let dismiss!: () => void;
  const launched = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const dismissed = new Promise<void>((resolve) => {
    dismiss = resolve;
  });
  test.mock.method(
    Launcher.prototype,
    "review",
    async (_request: unknown, signal: AbortSignal) => {
      entered();
      return new Promise<string>((_resolve, reject) =>
        signal.addEventListener(
          "abort",
          () => reject(new ReviewError("Review cancelled", "Captured notes")),
          { once: true },
        ),
      );
    },
  );
  test.mock.method(terminal.ctx.ui.dialog, "alert", async (input: unknown) => {
    terminal.alerts.push(input);
    await dismissed;
  });
  const dispose = await plugin.setup(terminal.ctx);
  try {
    assert.ok(terminal.commands.length);
    const review = terminal.commands[0].run();
    await launched;
    assert.equal(
      await Promise.race([
        Promise.resolve(dispose?.()).then(() => true),
        delay(100).then(() => false),
      ]),
      true,
    );
    await review;
    await nextTurn();
    assert.equal(terminal.listeners.size, 0);
    assert.match(terminal.alerts[0]?.message, /Captured notes/);
  } finally {
    dismiss();
    await dispose?.();
  }
});

for (const reason of [
  "session.execution.started",
  "session.agent.selected",
  "session.deleted",
  "unload",
]) {
  test(
    `an external review is reported as detached on ${reason} before any notes are flushed`,
    { timeout: 10_000 },
    async (test) => {
      const directory = await mkdtemp(path.join(tmpdir(), "revdiff-detached-"));
      const bin = path.join(directory, "bin");
      const pidFile = path.join(directory, "pane-pid");
      const readyFile = path.join(directory, "ready");
      const releaseFile = path.join(directory, "quit");
      const historyFile = path.join(directory, "history.txt");
      const doneFile = path.join(directory, "done");
      await mkdir(bin);
      await writeFile(
        path.join(bin, "agtermctl"),
        `#!${process.execPath}
const { spawn } = require("node:child_process");
const fs = require("node:fs");
const args = process.argv.slice(2);
if (args.slice(0, 3).join(" ") === "session overlay open") {
  const child = spawn("/bin/sh", ["-c", args[3]], { detached: true, stdio: "ignore" });
  fs.writeFileSync(${JSON.stringify(pidFile)}, String(child.pid));
  child.on("exit", code => process.exit(code ?? 1));
}
`,
        { mode: 0o755 },
      );
      await writeFile(
        path.join(bin, "revdiff"),
        `#!${process.execPath}
const fs = require("node:fs");
const output = process.argv.slice(2).findLast(arg => arg.startsWith("--output=")).slice(9);
fs.writeFileSync(${JSON.stringify(readyFile)}, output);
const timer = setInterval(() => {
  if (!fs.existsSync(${JSON.stringify(releaseFile)})) return;
  clearInterval(timer);
  try { fs.writeFileSync(output, "Late review notes"); } catch {}
  fs.writeFileSync(${JSON.stringify(historyFile)}, "Late review notes");
  fs.writeFileSync(${JSON.stringify(doneFile)}, "done");
}, 10);
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
exec bash "${source}" "$@"
`,
      );
      const terminal = client();
      terminal.session.location.directory = directory;
      const original = Launcher.prototype.review;
      test.mock.method(
        Launcher.prototype,
        "review",
        (input: Parameters<Launcher["review"]>[0], signal: AbortSignal) =>
          original.call(new Launcher(directory), input, signal),
      );
      const dispose = await plugin.setup(terminal.ctx);
      const review = terminal.emit("session.execution.succeeded");
      try {
        let output: string | undefined;
        const deadline = Date.now() + 5_000;
        while (!output && Date.now() < deadline) {
          output = await readFile(readyFile, "utf8").catch(() => undefined);
          if (!output) await delay(10);
        }
        assert.ok(output, "the separate terminal process must have started");
        assert.equal(await readFile(output, "utf8"), "");
        const panePID = Number(await readFile(pidFile, "utf8"));
        assert.doesNotThrow(() => process.kill(-panePID, 0));
        if (reason === "unload") await dispose?.();
        else terminal.emit(reason);
        await review;
        assert.doesNotThrow(
          () => process.kill(-panePID, 0),
          "terminal-owned revdiff must survive launcher cancellation",
        );
        assert.deepEqual(terminal.prompts, []);
        const notice = terminal.alerts.find(
          (item) => item.title === "Review detached",
        );
        assert.ok(
          notice,
          "cancellation must report detachment even with no flushed notes",
        );
        assert.match(notice.message, /history/i);
        assert.match(notice.message, /not.*delivered automatically/i);
        await writeFile(releaseFile, "quit");
        const finishDeadline = Date.now() + 3_000;
        while (
          Date.now() < finishDeadline &&
          !(await readFile(doneFile, "utf8").catch(() => ""))
        )
          await delay(10);
        assert.equal(await readFile(historyFile, "utf8"), "Late review notes");
        assert.deepEqual(terminal.prompts, []);
      } finally {
        await dispose?.();
        await writeFile(releaseFile, "quit");
        const panePID = Number(await readFile(pidFile, "utf8").catch(() => ""));
        if (panePID > 0) {
          const deadline = Date.now() + 1_000;
          while (Date.now() < deadline) {
            try {
              process.kill(-panePID, 0);
            } catch {
              break;
            }
            await delay(10);
          }
          try {
            process.kill(-panePID, "SIGKILL");
          } catch {}
        }
        await review;
        await rm(directory, { recursive: true, force: true });
      }
    },
  );
}

for (const stage of ["initial", "delivery"] as const) {
  test(`cleanup interrupts a stalled message sync during ${stage} lookup`, async (test) => {
    const terminal = client();
    let delivering = false;
    let entered!: () => void;
    let release!: () => void;
    const reached = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const blocked = new Promise<void>((resolve) => {
      release = resolve;
    });
    const sync = terminal.messageAPI.sync;
    test.mock.method(terminal.messageAPI, "sync", async () => {
      await sync();
      if (stage === "initial" || delivering) {
        entered();
        await blocked;
      }
    });
    let launches = 0;
    test.mock.method(Launcher.prototype, "review", async () => {
      launches++;
      delivering = true;
      return "Keep the captured notes";
    });
    const dispose = await plugin.setup(terminal.ctx);
    const review = terminal.emit("session.execution.succeeded");
    try {
      await reached;
      const cleanup = Promise.resolve(dispose?.());
      assert.equal(
        await Promise.race([
          cleanup.then(() => true),
          delay(100).then(() => false),
        ]),
        true,
        "cleanup must finish before the non-cancellable sync settles",
      );
      assert.deepEqual(terminal.prompts, []);
      assert.equal(launches, stage === "initial" ? 0 : 1);
      if (stage === "delivery")
        assert.match(terminal.alerts[0]?.message, /Keep the captured notes/);
      assert.equal(terminal.listeners.size, 0);
    } finally {
      release();
      await review;
      await dispose?.();
    }
  });
}

for (const stage of ["get", "sync", "prompt"] as const) {
  for (const change of [
    "session.agent.selected",
    "session.execution.started",
  ] as const) {
    test(
      `plan delivery cancels on ${change} during ${stage}`,
      { timeout: 2_000 },
      async (test) => {
        const terminal = client();
        let delivering = false;
        let entered!: () => void;
        let release!: () => void;
        const reached = new Promise<void>((resolve) => {
          entered = resolve;
        });
        const blocked = new Promise<void>((resolve) => {
          release = resolve;
        });
        const original =
          stage === "sync" ? terminal.messageAPI.sync : terminal.api[stage];
        test.mock.method(Launcher.prototype, "review", async () => {
          delivering = true;
          return "Captured plan notes";
        });
        const delayed = async (
          input: any,
          options?: { signal?: AbortSignal },
        ) => {
          if (!delivering) return original(input, options);
          const snapshot =
            stage === "prompt" ? undefined : await original(input, options);
          entered();
          await blocked;
          if (stage === "prompt") {
            options?.signal?.throwIfAborted();
            return original(input, options);
          }
          return snapshot;
        };
        if (stage === "sync")
          test.mock.method(terminal.messageAPI, stage, delayed);
        else test.mock.method(terminal.api, stage, delayed);
        const dispose = await plugin.setup(terminal.ctx);
        try {
          const review = terminal.emit("session.execution.succeeded");
          await reached;
          if (change === "session.agent.selected")
            terminal.session.agent = "build";
          terminal.emit(change);
          release();
          await review;
          assert.deepEqual(terminal.prompts, []);
          assert.match(terminal.alerts[0]?.message, /Captured plan notes/);
        } finally {
          release();
          await dispose?.();
        }
      },
    );
  }
}
