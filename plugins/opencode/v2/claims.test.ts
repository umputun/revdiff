import assert from "node:assert/strict";
import { test } from "node:test";
import {
  mkdtemp,
  rm,
  readdir,
  readFile,
  writeFile,
  utimes,
  stat,
  mkdir,
  symlink,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { ReviewClaims } from "./claims.ts";

test("two local clients claim one completion event exactly once", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-claims-"));
  try {
    const clients = [new ReviewClaims(directory), new ReviewClaims(directory)];
    const results = await Promise.all(
      clients.map((client) => client.claim("event-a")),
    );
    assert.equal(results.filter(Boolean).length, 1);
    assert.equal(await new ReviewClaims(directory).claim("event-a"), false);
    assert.equal(await clients[0].claim("event-b"), true);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("concurrent claims prune expired markers and preserve unrelated files", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-claims-gc-"));
  try {
    const claims = new ReviewClaims(directory);
    await claims.claim("old-event");
    const old = path.join(directory, (await readdir(directory))[0]);
    const expired = new Date(Date.now() - 2 * 24 * 60 * 60 * 1_000);
    await utimes(old, expired, expired);
    const foreign = path.join(directory, "notes.txt");
    await writeFile(foreign, "keep this file");
    await utimes(foreign, expired, expired);
    const folder = path.join(directory, `event-${"a".repeat(64)}.claim`);
    const link = path.join(directory, `event-${"b".repeat(64)}.claim`);
    await mkdir(folder);
    await utimes(folder, expired, expired);
    await symlink(foreign, link);
    const results = await Promise.all(
      Array.from({ length: 8 }, () =>
        new ReviewClaims(directory).claim("new-event"),
      ),
    );
    assert.equal(results.filter(Boolean).length, 1);
    await assert.rejects(stat(old), { code: "ENOENT" });
    assert.equal(await claims.claim("new-event"), false);
    assert.equal(await readFile(foreign, "utf8"), "keep this file");
    assert.equal(await readFile(link, "utf8"), "keep this file");
    assert.ok((await stat(folder)).isDirectory());
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("default claims use the named XDG state directory", async () => {
  const state = await mkdtemp(path.join(tmpdir(), "revdiff-claims-state-"));
  const previous = process.env.XDG_STATE_HOME;
  process.env.XDG_STATE_HOME = state;
  try {
    await new ReviewClaims().claim("event");
    assert.equal(
      (await readdir(path.join(state, "revdiff", "opencode-plan-review")))
        .length,
      1,
    );
  } finally {
    if (previous === undefined) delete process.env.XDG_STATE_HOME;
    else process.env.XDG_STATE_HOME = previous;
    await rm(state, { recursive: true, force: true });
  }
});

test("claim filesystem failures other than EEXIST are errors", async () => {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-claims-error-"));
  try {
    const blocked = path.join(directory, "not-a-directory");
    await writeFile(blocked, "keep");
    await assert.rejects(
      new ReviewClaims(path.join(blocked, "claims")).claim("event"),
      { code: "ENOTDIR" },
    );
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
