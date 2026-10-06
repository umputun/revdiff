import type { Plugin } from "@opencode/plugin/tui";
import type { SessionInfo } from "@opencode/client";
import { randomUUID } from "node:crypto";
import { fileURLToPath } from "node:url";
import { ReviewClaims } from "./claims.ts";
import { Launcher, ReviewError } from "./launcher.ts";

type Operation = {
  sessionID: string | undefined;
  plan: boolean;
  controller: AbortController;
  annotations: string;
  feedback?: { id: string; accepted: boolean };
};

export default {
  id: "revdiff.cli",
  setup(ctx) {
    const launcher = new Launcher(
      fileURLToPath(new URL("./scripts/", import.meta.url)),
    );
    const claims = new ReviewClaims();
    const operations = new Set<Operation>();
    const plans = new Map<string, Operation>();
    const tasks = new Set<Promise<void>>();
    let tail = Promise.resolve();
    let stopped = false;

    function displayed(sessionID: string | undefined) {
      const route = ctx.ui.router.current();
      if (sessionID === undefined) return route.type === "home";
      return route.type === "session" && route.sessionID === sessionID;
    }

    function run(
      sessionID: string | undefined,
      plan: boolean,
      action: (op: Operation) => Promise<void>,
    ): Promise<void> {
      if (stopped || (plan && sessionID && plans.has(sessionID)))
        return Promise.resolve();
      const op: Operation = {
        sessionID,
        plan,
        controller: new AbortController(),
        annotations: "",
      };
      operations.add(op);
      if (plan && sessionID) plans.set(sessionID, op);
      const task = (async () => action(op))()
        .catch((error: unknown) => {
          if (op.feedback?.accepted) return;
          const notes =
            error instanceof ReviewError ? error.annotations : op.annotations;
          const message =
            error instanceof Error ? error.message : String(error);
          const detached = error instanceof ReviewError && error.detached;
          if (notes || detached) {
            // The host owns the alert; unloading never waits for a user to dismiss it.
            void ctx.ui.dialog
              .alert({
                title: detached
                  ? "Review detached"
                  : "Review annotations were not delivered",
                message: notes ? `${message}\n\n${notes}` : message,
              })
              .catch((failure) =>
                console.error("revdiff annotations dialog:", failure),
              );
          }
          if (!op.controller.signal.aborted) {
            ctx.ui.toast.show({
              message: `revdiff: ${message}`,
              variant: "error",
            });
          }
        })
        .finally(() => {
          operations.delete(op);
          tasks.delete(task);
          if (sessionID && plans.get(sessionID) === op) plans.delete(sessionID);
        });
      tasks.add(task);
      return task;
    }

    function enqueue(op: Operation, action: () => Promise<void>) {
      const task = tail.then(() => {
        op.controller.signal.throwIfAborted();
        if (!displayed(op.sessionID)) return;
        return action();
      });
      tail = task.catch(() => {});
      return task;
    }

    function waitFor<T>(task: Promise<T>, signal: AbortSignal): Promise<T> {
      let abort!: () => void;
      const cancelled = new Promise<never>((_resolve, reject) => {
        abort = () => reject(signal.reason);
        signal.addEventListener("abort", abort, { once: true });
        if (signal.aborted) abort();
      });
      return Promise.race([task, cancelled]).finally(() => {
        signal.removeEventListener("abort", abort);
      });
    }

    async function planFor(
      sessionID: string,
      signal: AbortSignal,
      displayedOnly = false,
    ) {
      const session: SessionInfo = await ctx.client.session.get(
        { sessionID },
        { signal },
      );
      signal.throwIfAborted();
      if (displayedOnly && !displayed(sessionID)) return;
      if (
        session.parentID ||
        session.agent !== "plan" ||
        session.outcome !== "succeeded"
      )
        return;
      await waitFor(ctx.data.session.message.sync(sessionID), signal);
      signal.throwIfAborted();
      if (displayedOnly && !displayed(sessionID)) return;
      const messages = ctx.data.session.message.list(sessionID);
      const last = messages.at(-1);
      const message =
        last?.type === "idle" && last.outcome === "succeeded"
          ? messages.at(-2)
          : last;
      if (
        message?.type !== "assistant" ||
        message.agent !== "plan" ||
        message.finish !== "stop" ||
        message.error ||
        message.time.completed === undefined
      )
        return;
      const text = message.content
        .filter((part) => part.type === "text")
        .map((part) => part.text)
        .join("\n")
        .trim();
      if (text) return { session, messageID: message.id, text };
    }

    const stopPlan = ctx.data.on("session.execution.succeeded", (event) => {
      const sessionID = event.data.sessionID;
      if (!displayed(sessionID)) return;
      return run(sessionID, true, async (op) => {
        const signal = op.controller.signal;
        const plan = await planFor(sessionID, signal, true);
        if (!plan) return;
        await enqueue(op, async () => {
          const current = await planFor(sessionID, signal, true);
          if (current?.messageID !== plan.messageID || !displayed(sessionID))
            return;
          await launcher.preflight(plan.session.location.directory);
          signal.throwIfAborted();
          if (!displayed(sessionID)) return;
          if (!(await claims.claim(event.id))) return;
          signal.throwIfAborted();
          if (!displayed(sessionID)) return;
          op.annotations = await launcher.review(
            { directory: plan.session.location.directory, plan: plan.text },
            signal,
          );
          if (!op.annotations) return;
          const latest = await planFor(sessionID, signal);
          if (latest?.messageID !== plan.messageID)
            throw new Error("The reviewed plan changed.");
          const snapshot = plan.text
            .split("\n")
            .map((line, index) => `${index + 1} | ${line}`)
            .join("\n");
          const feedback = { id: `msg_${randomUUID()}`, accepted: false };
          op.feedback = feedback;
          await ctx.client.session.prompt(
            {
              id: feedback.id,
              sessionID,
              delivery: "queue",
              text: [
                "I reviewed the plan and added annotations. Please revise the plan to address each one.",
                "The temporary plan file named in the annotations has already been deleted. Do not try to read that path. Use the exact reviewed snapshot below: annotation line numbers refer to these original lines, including blank lines. The numbered prefixes are not part of the plan.",
                `Reviewed plan (line-numbered):\n${snapshot}`,
                `Annotations:\n${op.annotations}`,
              ].join("\n\n"),
            },
            { signal },
          );
          feedback.accepted = true;
        });
      });
    });

    const stopFeedback = ctx.data.on("session.inbox.enqueued", (event) => {
      for (const op of operations) {
        if (
          op.sessionID === event.data.sessionID &&
          op.feedback?.id === event.data.inboxID
        ) {
          op.feedback.accepted = true;
        }
      }
    });

    const stopChanges = (
      [
        "session.execution.started",
        "session.agent.selected",
        "session.deleted",
      ] as const
    ).map((type) =>
      ctx.data.on(type, (event) => {
        for (const op of operations) {
          if (
            op.sessionID === event.data.sessionID &&
            !op.feedback?.accepted &&
            (op.plan || type === "session.deleted")
          ) {
            op.controller.abort(
              new Error("Review cancelled because the session changed."),
            );
          }
        }
        plans.delete(event.data.sessionID);
      }),
    );

    // Keymap.Provider is available in this slot, not during setup in v2.0.18.
    const stopCommand = ctx.ui.slot({
      append: "app",
      render: () => {
        ctx.keymap.layer(() => ({
          mode: "global",
          commands: [
            {
              id: "revdiff.review",
              title: "Review changes with revdiff",
              group: "revdiff",
              slash: { name: "revdiff", arguments: true },
              palette: true,
              enabled: () =>
                !stopped &&
                ["home", "session"].includes(ctx.ui.router.current().type),
              run: async (input) => {
                const route = ctx.ui.router.current();
                if (route.type !== "session" && route.type !== "home") return;
                const sessionID =
                  route.type === "session" ? route.sessionID : undefined;
                const location = ctx.data.location.default();
                const selectedModel = ctx.ui.model?.current();
                await run(sessionID, false, async (op) => {
                  const signal = op.controller.signal;
                  const session = sessionID
                    ? await ctx.client.session.get({ sessionID }, { signal })
                    : undefined;
                  signal.throwIfAborted();
                  if (session?.parentID)
                    throw new Error(
                      "Manual review requires a root session. Open the parent session and run /revdiff there.",
                    );
                  await enqueue(op, async () => {
                    op.annotations = await launcher.review(
                      {
                        directory:
                          session?.location.directory ?? location.directory,
                        arguments: input ?? "",
                      },
                      signal,
                    );
                    if (!op.annotations) return;
                    signal.throwIfAborted();
                    let targetID = sessionID;
                    if (!targetID) {
                      const config = await ctx.client.config.get(
                        { location },
                        { signal },
                      );
                      signal.throwIfAborted();
                      const defaults = config.findLast(
                        (entry) =>
                          entry.type === "document" &&
                          entry.info.default_agent !== undefined,
                      );
                      const created = await ctx.client.session.create(
                        {
                          location,
                          agent:
                            defaults?.type === "document"
                              ? defaults.info.default_agent
                              : "build",
                          model: selectedModel
                            ? {
                                providerID: selectedModel.providerID,
                                id: selectedModel.modelID,
                                ...(selectedModel.variant !== undefined
                                  ? { variant: selectedModel.variant }
                                  : {}),
                              }
                            : undefined,
                        },
                        { signal },
                      );
                      signal.throwIfAborted();
                      targetID = created.id;
                      op.sessionID = targetID;
                      if (displayed(undefined))
                        ctx.ui.router.navigate({
                          type: "session",
                          sessionID: targetID,
                        });
                    }
                    await ctx.client.session.prompt(
                      {
                        sessionID: targetID,
                        delivery: "queue",
                        text: `I reviewed the changes and added annotations. Please address each one:\n\n${op.annotations}`,
                      },
                      { signal },
                    );
                  });
                });
              },
            },
          ],
        }));
        return null;
      },
    });

    return async () => {
      stopped = true;
      stopCommand();
      stopPlan();
      stopFeedback();
      for (const stop of stopChanges) stop();
      for (const op of operations) op.controller.abort();
      await Promise.all(tasks);
    };
  },
} satisfies Plugin.Definition;
