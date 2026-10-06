import assert from "node:assert/strict";
import { test } from "node:test";
import {
  mkdtemp,
  readFile,
  writeFile,
  mkdir,
  rm,
  stat,
  symlink,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const installer = fileURLToPath(new URL("../setup.sh", import.meta.url));

async function fixture(version: string) {
  const directory = await mkdtemp(path.join(tmpdir(), "revdiff-setup-test-"));
  const bin = path.join(directory, "bin");
  const home = path.join(directory, "home");
  const config = version.startsWith("1.")
    ? path.join(home, ".config", "opencode")
    : path.join(directory, "config with spaces");
  const npmArgs = path.join(directory, "npm-args.txt");
  await Promise.all([
    mkdir(bin),
    mkdir(home, { recursive: true }),
    mkdir(config, { recursive: true }),
  ]);
  await writeFile(
    path.join(bin, "opencode"),
    `#!/bin/bash\nprintf '%s\\n' '${version}'\n`,
    { mode: 0o755 },
  );
  await writeFile(
    path.join(bin, "npm"),
    `#!/bin/bash\nprintf '%s\\n' "$@" > "${npmArgs}"\n`,
    { mode: 0o755 },
  );
  return {
    directory,
    bin,
    home,
    config,
    npmArgs,
    run(args = [config], extraEnv: NodeJS.ProcessEnv = {}) {
      return execFileSync("bash", [installer, ...args], {
        env: {
          ...process.env,
          PATH: `${bin}:${process.env.PATH}`,
          HOME: home,
          XDG_CONFIG_HOME: path.join(directory, "xdg"),
          OPENCODE_CONFIG_DIR: "",
          ...extraEnv,
        },
        encoding: "utf8",
        stdio: "pipe",
      });
    },
    async close() {
      await rm(directory, { recursive: true, force: true });
    },
  };
}

test("setup detects v2 and installs a dependency-free local package", async () => {
  const sandbox = await fixture("opencode v2.0.18");
  try {
    const config =
      '{"plugins": ["another-plugin"], "model": "provider/model"}\n';
    await writeFile(path.join(sandbox.config, "opencode.json"), config);
    sandbox.run();
    const installed = path.join(sandbox.config, "plugins", "revdiff");
    assert.equal(
      await stat(path.join(installed, "index.ts")).catch(() => false),
      false,
      "CLI-only installation must not have a server entrypoint",
    );
    assert.equal(
      await stat(path.join(installed, "server.ts")).catch(() => false),
      false,
    );
    assert.ok(await stat(path.join(installed, "tui.ts")));
    assert.ok(await stat(path.join(installed, "launcher.ts")));
    assert.ok(await stat(path.join(installed, "claims.ts")));
    assert.equal(
      await readFile(path.join(sandbox.config, "opencode.json"), "utf8"),
      config,
    );
    assert.equal(
      await stat(path.join(sandbox.config, "commands", "revdiff.md")).catch(
        () => false,
      ),
      false,
    );
    assert.equal(
      await readFile(
        path.join(installed, "scripts", "launch-revdiff.sh"),
        "utf8",
      ),
      await readFile(
        new URL(
          "../../../.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh",
          import.meta.url,
        ),
        "utf8",
      ),
    );
    assert.ok(
      await stat(path.join(installed, "scripts", "agentdeck-window.sh")),
    );
    assert.equal(
      await stat(sandbox.npmArgs).catch(() => false),
      false,
      "local installation must not run npm",
    );
    assert.equal(
      await stat(path.join(installed, "node_modules")).catch(() => false),
      false,
    );
    assert.equal(
      await stat(path.join(sandbox.config, "tools", "revdiff.ts")).catch(
        () => false,
      ),
      false,
    );
  } finally {
    await sandbox.close();
  }
});

test("setup detects v1 and registers it once in its HOME config", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    await writeFile(
      path.join(sandbox.config, "opencode.json"),
      JSON.stringify({ plugin: ["another-plugin"], model: "provider/model" }),
    );
    sandbox.run();
    sandbox.run();
    assert.ok(
      await stat(path.join(sandbox.config, "tools", "revdiff.ts")).catch(
        () => false,
      ),
      "v1 tool must be installed beside its fixed-path plan launcher",
    );
    assert.equal(
      await stat(
        path.join(sandbox.config, "tools", "agentdeck-window.sh"),
      ).catch(() => false),
      false,
      "v1 retains its original installed file set",
    );
    assert.ok(
      await stat(
        path.join(sandbox.config, "plugins", "revdiff-plan-review.ts"),
      ),
    );
    assert.deepEqual(
      JSON.parse(
        await readFile(path.join(sandbox.config, "opencode.json"), "utf8"),
      ),
      {
        plugin: ["another-plugin", "./plugins/revdiff-plan-review.ts"],
        model: "provider/model",
      },
    );
    assert.equal(await stat(sandbox.npmArgs).catch(() => false), false);
  } finally {
    await sandbox.close();
  }
});

test("fresh v1 installation does not require jq", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    for (const name of ["bash", "mkdir", "cp", "chmod"]) {
      await symlink(`/bin/${name}`, path.join(sandbox.bin, name));
    }
    await symlink("/usr/bin/dirname", path.join(sandbox.bin, "dirname"));
    sandbox.run([], { PATH: sandbox.bin });
    assert.deepEqual(
      JSON.parse(
        await readFile(path.join(sandbox.config, "opencode.json"), "utf8"),
      ),
      {
        plugin: ["./plugins/revdiff-plan-review.ts"],
      },
    );
    assert.ok(
      await stat(
        path.join(sandbox.config, "plugins", "revdiff-plan-review.ts"),
      ),
    );
    assert.ok(
      await stat(path.join(sandbox.config, "tools", "launch-revdiff.sh")),
    );
  } finally {
    await sandbox.close();
  }
});

for (const [name, content] of [
  [
    "comment",
    '{\n // keep this comment\n "plugin": ["./plugins/revdiff-plan-review.ts"],\n "model": "provider/model"\n}\n',
  ],
  [
    "trailing comma",
    '{"plugin": ["./plugins/revdiff-plan-review.ts",], "model": "provider/model",}\n',
  ],
] as const) {
  test(`v2 installs with a ${name} in opencode.json and requests manual cleanup`, async () => {
    const sandbox = await fixture("opencode v2.0.18");
    try {
      const file = path.join(sandbox.config, "opencode.json");
      await writeFile(file, content);
      const output = sandbox.run();
      assert.match(output, /Notice: opencode\.json is unchanged/);
      assert.match(output, /registration manually/);
      assert.equal(await readFile(file, "utf8"), content);
      assert.ok(
        await stat(path.join(sandbox.config, "plugins", "revdiff", "tui.ts")),
      );
    } finally {
      await sandbox.close();
    }
  });
}

test("v1 rejects a custom config directory before modifying its contents", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const custom = path.join(sandbox.directory, "custom config");
    await mkdir(custom);
    const config = '{"plugin":["unrelated"]}\n';
    await writeFile(path.join(custom, "opencode.json"), config);
    assert.throws(() => sandbox.run([custom]), /v1.*config|config.*v2/i);
    assert.equal(
      await readFile(path.join(custom, "opencode.json"), "utf8"),
      config,
    );
    assert.equal(
      await stat(path.join(custom, "plugins")).catch(() => false),
      false,
    );
    assert.equal(
      await stat(path.join(sandbox.config, "plugins")).catch(() => false),
      false,
    );
  } finally {
    await sandbox.close();
  }
});

test("setup can select an explicit OpenCode binary instead of the one on PATH", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const binary = path.join(sandbox.bin, "opencode v2");
    await writeFile(binary, "#!/bin/bash\nprintf 'opencode v2.0.18\\n'\n", {
      mode: 0o755,
    });
    sandbox.run(["--opencode", binary, sandbox.config]);
    assert.ok(
      await stat(
        path.join(sandbox.config, "plugins", "revdiff", "tui.ts"),
      ).catch(() => false),
    );
  } finally {
    await sandbox.close();
  }
});

test("setup uses the XDG config directory by default", async () => {
  const sandbox = await fixture("2.0.18");
  try {
    sandbox.run([]);
    assert.ok(
      await stat(
        path.join(
          sandbox.directory,
          "xdg",
          "opencode",
          "plugins",
          "revdiff",
          "tui.ts",
        ),
      ).catch(() => false),
    );
  } finally {
    await sandbox.close();
  }
});

for (const version of ["2.0.0", "2.0.10", "2.0.17"]) {
  test(`setup accepts OpenCode ${version} without a patch-version floor`, async () => {
    const sandbox = await fixture(version);
    try {
      sandbox.run();
      assert.ok(
        await stat(path.join(sandbox.config, "plugins", "revdiff", "tui.ts")),
      );
    } finally {
      await sandbox.close();
    }
  });
}

for (const version of ["3.0.0", "unknown version"]) {
  test(`setup rejects unsupported version ${version} before installing files`, async () => {
    const sandbox = await fixture(version);
    try {
      assert.throws(() => sandbox.run(), /version/i);
      assert.equal(
        await stat(path.join(sandbox.config, "plugins")).catch(() => false),
        false,
      );
      assert.equal(await stat(sandbox.npmArgs).catch(() => false), false);
    } finally {
      await sandbox.close();
    }
  });
}

test("v2 setup removes only the legacy plan plugin, command and exact JSON registration", async () => {
  const sandbox = await fixture("opencode v2.0.18");
  try {
    await mkdir(path.join(sandbox.config, "plugins"));
    const legacy = path.join(
      sandbox.config,
      "plugins",
      "revdiff-plan-review.ts",
    );
    await writeFile(legacy, "customized v1 plugin");
    await mkdir(path.join(sandbox.config, "commands"));
    await mkdir(path.join(sandbox.config, "tools"));
    await writeFile(
      path.join(sandbox.config, "commands", "revdiff.md"),
      "old command",
    );
    await writeFile(
      path.join(sandbox.config, "tools", "revdiff.ts"),
      "v1 tool stays",
    );
    await writeFile(
      path.join(sandbox.config, "plugins", "unrelated.ts"),
      "unrelated plugin",
    );
    const entry = "./plugins/revdiff-plan-review.ts";
    await writeFile(
      path.join(sandbox.config, "opencode.json"),
      JSON.stringify({
        plugin: [
          "unrelated",
          entry,
          [entry, {}],
          "./plugins/revdiff-plan-review.tsx",
        ],
        model: "provider/model",
      }),
    );
    sandbox.run();
    assert.equal(await stat(legacy).catch(() => false), false);
    assert.equal(
      await stat(path.join(sandbox.config, "commands", "revdiff.md")).catch(
        () => false,
      ),
      false,
    );
    assert.equal(
      await readFile(path.join(sandbox.config, "tools", "revdiff.ts"), "utf8"),
      "v1 tool stays",
    );
    assert.equal(
      await readFile(
        path.join(sandbox.config, "plugins", "unrelated.ts"),
        "utf8",
      ),
      "unrelated plugin",
    );
    assert.deepEqual(
      JSON.parse(
        await readFile(path.join(sandbox.config, "opencode.json"), "utf8"),
      ),
      {
        plugin: ["unrelated", [entry, {}], "./plugins/revdiff-plan-review.tsx"],
        model: "provider/model",
      },
    );
  } finally {
    await sandbox.close();
  }
});

test("rollback to v1 leaves the inert v2 directory intact", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const installed = path.join(sandbox.config, "plugins", "revdiff");
    await mkdir(installed, { recursive: true });
    await writeFile(path.join(installed, "tui.ts"), "customized v2 plugin");
    sandbox.run();
    assert.equal(
      await readFile(path.join(installed, "tui.ts"), "utf8"),
      "customized v2 plugin",
    );
    assert.ok(await stat(path.join(sandbox.config, "tools", "revdiff.ts")));
  } finally {
    await sandbox.close();
  }
});

test("v2 config selection honors OPENCODE_CONFIG_DIR before XDG_CONFIG_HOME", async () => {
  const sandbox = await fixture("2.0.18");
  try {
    sandbox.run([], { OPENCODE_CONFIG_DIR: sandbox.config });
    assert.ok(
      await stat(
        path.join(sandbox.config, "plugins", "revdiff", "tui.ts"),
      ).catch(() => false),
    );
  } finally {
    await sandbox.close();
  }
});

test("v2 setup leaves JSONC byte-identical and prints a manual cleanup notice", async () => {
  const sandbox = await fixture("2.0.18");
  try {
    const text =
      '{\n // user comment\n "plugin": ["./plugins/revdiff-plan-review.ts"]\n}\n';
    await writeFile(path.join(sandbox.config, "opencode.jsonc"), text);
    assert.match(sandbox.run(), /JSONC|opencode\.jsonc/);
    assert.equal(
      await readFile(path.join(sandbox.config, "opencode.jsonc"), "utf8"),
      text,
    );
    assert.ok(
      await stat(path.join(sandbox.config, "plugins", "revdiff", "tui.ts")),
    );
  } finally {
    await sandbox.close();
  }
});

test("v1 to v2 upgrade and rollback preserve other config and restore v1 files", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const configFile = path.join(sandbox.config, "opencode.json");
    await writeFile(
      configFile,
      JSON.stringify({ plugin: ["unrelated"], model: "provider/model" }),
    );
    sandbox.run();
    await writeFile(
      path.join(sandbox.bin, "opencode"),
      "#!/bin/bash\nprintf 'opencode v2.0.18\\n'\n",
    );
    sandbox.run();
    assert.deepEqual(JSON.parse(await readFile(configFile, "utf8")), {
      plugin: ["unrelated"],
      model: "provider/model",
    });
    assert.equal(
      await stat(path.join(sandbox.config, "commands", "revdiff.md")).catch(
        () => false,
      ),
      false,
    );
    await writeFile(
      path.join(sandbox.bin, "opencode"),
      "#!/bin/bash\nprintf '1.18.32\\n'\n",
    );
    sandbox.run();
    assert.deepEqual(JSON.parse(await readFile(configFile, "utf8")), {
      plugin: ["unrelated", "./plugins/revdiff-plan-review.ts"],
      model: "provider/model",
    });
    assert.equal(
      await readFile(
        path.join(sandbox.config, "plugins", "revdiff-plan-review.ts"),
        "utf8",
      ),
      await readFile(
        new URL("../plugins/revdiff-plan-review.ts", import.meta.url),
        "utf8",
      ),
    );
    assert.ok(
      await stat(path.join(sandbox.config, "plugins", "revdiff", "tui.ts")),
    );
  } finally {
    await sandbox.close();
  }
});

test("v1 retains its HOME config default", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const override = path.join(sandbox.directory, "ignored-config");
    sandbox.run([], { OPENCODE_CONFIG_DIR: override });
    assert.ok(
      await stat(
        path.join(sandbox.home, ".config", "opencode", "tools", "revdiff.ts"),
      ),
    );
    assert.equal(
      await stat(path.join(override, "tools")).catch(() => false),
      false,
    );
  } finally {
    await sandbox.close();
  }
});

test("setup reports failure to execute the selected OpenCode binary", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const binary = path.join(sandbox.bin, "broken-opencode");
    await writeFile(binary, "#!/bin/bash\nexit 9\n", { mode: 0o755 });
    assert.throws(
      () => sandbox.run(["--opencode", binary, sandbox.config]),
      /version/i,
    );
    assert.equal(
      await stat(path.join(sandbox.config, "plugins")).catch(() => false),
      false,
    );
  } finally {
    await sandbox.close();
  }
});

test("v2 installation does not require a working npm", async () => {
  const sandbox = await fixture("2.0.18");
  try {
    await writeFile(path.join(sandbox.bin, "npm"), "#!/bin/bash\nexit 42\n", {
      mode: 0o755,
    });
    assert.doesNotThrow(() => sandbox.run());
  } finally {
    await sandbox.close();
  }
});

test("v1 setup preserves invalid JSON without installing partial files", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const content = '{"plugin": "not an array"}';
    const config = path.join(sandbox.config, "opencode.json");
    await writeFile(config, content);
    assert.throws(() => sandbox.run(), /Invalid/);
    assert.equal(await readFile(config, "utf8"), content);
    assert.equal(
      await stat(path.join(sandbox.config, "tools")).catch(() => false),
      false,
    );
  } finally {
    await sandbox.close();
  }
});

test("v1 setup leaves JSONC untouched and installs beside it", async () => {
  const sandbox = await fixture("1.18.32");
  try {
    const config = path.join(sandbox.config, "opencode.jsonc");
    const content =
      '{\n  // keep this comment\n  "model": "provider/model"\n}\n';
    await writeFile(config, content);
    sandbox.run();
    assert.equal(await readFile(config, "utf8"), content);
    assert.deepEqual(
      JSON.parse(
        await readFile(path.join(sandbox.config, "opencode.json"), "utf8"),
      ),
      {
        plugin: ["./plugins/revdiff-plan-review.ts"],
      },
    );
    assert.ok(
      await stat(
        path.join(sandbox.config, "plugins", "revdiff-plan-review.ts"),
      ),
    );
    assert.ok(
      await stat(path.join(sandbox.config, "tools", "launch-revdiff.sh")),
    );
  } finally {
    await sandbox.close();
  }
});
