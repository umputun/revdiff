package main

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cmdReq struct {
	dir   string
	name  string
	stdin string
	args  []string
	env   map[string]string
}

type cmdResult struct {
	stdout string
	stderr string
	code   int
}

type launcherBackend struct {
	name    string
	command string
	env     map[string]string
}

type launcherRun struct {
	backend launcherBackend
	code    int
	output  string
}

func TestShellLaunchersPreserveAnnotationExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell launchers are not used on windows")
	}

	root := testRepoRoot(t)
	planFile := filepath.Join(t.TempDir(), "plan.md")
	writeTestFile(t, planFile, "# Plan\n")

	launchers := []struct {
		name string
		path string
		args []string
	}{
		{name: "claude", path: ".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh"},
		{name: "codex", path: "plugins/codex/skills/revdiff/scripts/launch-revdiff.sh"},
		{name: "plan review", path: "plugins/revdiff-planning/scripts/launch-plan-review.sh", args: []string{planFile}},
	}
	cases := []struct {
		name   string
		code   int
		output string
	}{
		{name: "clean", code: 0},
		{name: "annotations", code: exitCodeAnnotations, output: "## file.go:1 (+)\ncomment\n"},
		{name: "failure", code: 1, output: "partial output\n"},
	}

	for _, launcher := range launchers {
		for _, backend := range launcherBackends() {
			t.Run(launcher.name+"/"+backend.name, func(t *testing.T) {
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						script := filepath.Join(root, launcher.path)
						args := append([]string{script}, launcher.args...)
						res := runTestCmd(t, cmdReq{
							dir:  root,
							name: "bash",
							args: args,
							env: fakeLauncherEnv(t, launcherRun{
								backend: backend,
								code:    tc.code,
								output:  tc.output,
							}),
						})
						assert.Equal(t, tc.code, res.code)
						assert.Equal(t, tc.output, res.stdout)
					})
				}
			})
		}
	}
}

func TestPlanReviewHookAnnotationExitCodes(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}

	root := testRepoRoot(t)
	hook := filepath.Join(root, "plugins", "revdiff-planning", "scripts", "plan-review-hook.py")
	cases := []struct {
		name          string
		code          int
		output        string
		wantExit      int
		wantStdout    string
		wantStderr    string
		wantSnapshots int
	}{
		{name: "clean", code: 0, wantStdout: "plan reviewed, no annotations", wantSnapshots: 0},
		{
			name:          "annotations",
			code:          exitCodeAnnotations,
			output:        "## plan.md:2 (+)\nrevise this\n",
			wantExit:      2,
			wantStderr:    "revise this",
			wantSnapshots: 1,
		},
		{name: "failure", code: 1, wantStdout: "launcher exited 1", wantSnapshots: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			pluginRoot := filepath.Join(tmp, "plugin")
			launcher := filepath.Join(tmp, "launch-plan-review.sh")
			binDir := filepath.Join(tmp, "bin")
			writeExecutable(t, launcher, testFixtureScript(t, "fake-stdout-launcher.sh"))
			writeExecutable(t, filepath.Join(binDir, "revdiff"), "#!/bin/sh\nexit 0\n")
			writeExecutable(t, filepath.Join(pluginRoot, "scripts", "resolve-launcher.sh"), resolverScript(launcher))

			res := runTestCmd(t, cmdReq{
				dir:   root,
				name:  python,
				args:  []string{hook},
				stdin: `{"tool_input":{"plan":"# Plan\n- item"}}`,
				env: map[string]string{
					"CLAUDE_PLUGIN_ROOT": pluginRoot,
					"CLAUDE_PROJECT_DIR": root,
					"FAKE_OUTPUT":        tc.output,
					"FAKE_RC":            strconv.Itoa(tc.code),
					"PATH":               binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
					"TMPDIR":             tmp,
				},
			})
			assert.Equal(t, tc.wantExit, res.code)
			if tc.wantStdout == "" {
				assert.Empty(t, res.stdout)
			} else {
				assert.Contains(t, res.stdout, tc.wantStdout)
			}
			if tc.wantStderr == "" {
				assert.Empty(t, res.stderr)
			} else {
				assert.Contains(t, res.stderr, tc.wantStderr)
			}
			assert.Len(t, planSnapshots(t, tmp), tc.wantSnapshots)
		})
	}
}

func TestOpenCodeCallersPreserveAnnotationExitCode(t *testing.T) {
	root := testRepoRoot(t)
	tool := readRepoFile(t, root, "plugins", "opencode", "tools", "revdiff.ts")
	assert.Contains(t, tool, "const EXIT_CODE_ANNOTATIONS = 10;")
	assert.Contains(t, tool, ".quiet()")
	assert.Contains(t, tool, ".nothrow()")
	assert.Contains(t, tool, "if (!isRevdiffSuccess(result.exitCode))")
	assert.Contains(t, tool, "return stdout || \"(no annotations)\";")
	assert.Contains(t, tool, "return exitCode === 0 || exitCode === EXIT_CODE_ANNOTATIONS;")

	plugin := readRepoFile(t, root, "plugins", "opencode", "plugins", "revdiff-plan-review.ts")
	assert.Contains(t, plugin, "const EXIT_CODE_ANNOTATIONS = 10;")
	assert.Contains(t, plugin, ".quiet().nothrow()")
	assert.Contains(t, plugin, "if (isRevdiffSuccess(result.exitCode))")
	assert.Contains(t, plugin, "return stdout;")
	assert.Contains(t, plugin, "return exitCode === 0 || exitCode === EXIT_CODE_ANNOTATIONS;")
	assert.Contains(t, plugin, "if (!annotations) return;")
}

func TestPiCallerPreservesAnnotationExitCode(t *testing.T) {
	root := testRepoRoot(t)
	src := readRepoFile(t, root, "plugins", "pi", "extensions", "revdiff.ts")
	assert.Contains(t, src, `const EXIT_CODE_ON_ANNOTATIONS_ENV = "REVDIFF_EXIT_CODE_ON_ANNOTATIONS";`)
	assert.Contains(t, src, "const commandArgs = [...launch.args, `--output=${outputFile}`];")
	assert.Contains(t, src, "env: withAnnotationExitCode(process.env),")
	assert.NotContains(t, src, "runOverlayReview")
	assert.NotContains(t, src, "withRevdiffOnPath")
	assert.NotContains(t, src, "spawnSync(launcher")
	assert.Contains(t, src, "return exitCode === 0 || exitCode === EXIT_CODE_ANNOTATIONS;")
	assert.Contains(t, src, "if (!outputExists && exitCode === EXIT_CODE_ANNOTATIONS)")
	assert.Contains(t, src, "if (result.signal)")
	assert.Contains(t, src, "done(result.status ?? 1)")
	assert.Contains(t, src, "revdiff terminated by signal")
	assert.Contains(t, src, "return buildResult(launch, rawOutput);")
}

func TestPiExtensionExecutableBehavior(t *testing.T) {
	bun, err := exec.LookPath("bun")
	require.NoError(t, err, "bun is required for the Pi extension executable regression")

	root := testRepoRoot(t)
	tmp := t.TempDir()
	testPath := filepath.Join(tmp, "plugins", "pi", "extensions", "revdiff-test.ts")
	writeTestFile(t, filepath.Join(tmp, "node_modules", "typebox", "index.ts"), piTypeboxStub())
	writeTestFile(t, testPath, readRepoFile(t, root, "plugins", "pi", "extensions", "revdiff.ts")+piExtensionHarness())

	res := runTestCmd(t, cmdReq{
		dir:  tmp,
		name: bun,
		args: []string{"run", testPath},
		env:  map[string]string{"PATH": os.Getenv("PATH")},
	})
	require.Equal(t, 0, res.code, "stdout:\n%s\nstderr:\n%s", res.stdout, res.stderr)
}

func piTypeboxStub() string {
	return `export const Type = {
  Object: (schema: unknown) => schema,
  Optional: (schema: unknown) => schema,
  String: (options?: unknown) => ({ type: "string", options }),
};
`
}

func piExtensionHarness() string {
	return `
import { chmodSync as testChmodSync, mkdirSync as testMkdirSync, writeFileSync as testWriteFileSync } from "node:fs";

function testAssert(condition: unknown, message: string): asserts condition {
	if (!condition) {
		throw new Error(message);
	}
}

function assertArray(actual: string[], expected: string[], message: string): void {
	const actualText = JSON.stringify(actual);
	const expectedText = JSON.stringify(expected);
	testAssert(actualText === expectedText, message + ": got " + actualText + ", want " + expectedText);
}

function fakeCtx(choice?: "uncommitted" | "branch") {
	return {
		hasUI: true,
		isIdle: () => true,
		ui: {
			notifications: [] as string[],
			notify(message: string) {
				this.notifications.push(message);
			},
			select(_title: string, choices: string[]) {
				return choice === "branch" ? choices[1] : choices[0];
			},
			custom(factory: any) {
				let value: unknown;
				factory(
					{ stop() {}, start() {}, requestRender(_full?: boolean) {} },
					{},
					{},
					(next: unknown) => {
						value = next;
					},
				);
				return value;
			},
		},
	} as any;
}

function fakePi() {
	const commands = new Map<string, any>();
	const tools = new Map<string, any>();
	const sentMessages: string[] = [];
	return {
		commands,
		tools,
		sentMessages,
		registerCommand(name: string, command: any) {
			commands.set(name, command);
		},
		registerTool(tool: any) {
			tools.set(tool.name, tool);
		},
		sendUserMessage(message: string) {
			sentMessages.push(message);
		},
	} as any;
}

function writeExecutable(pathname: string, content: string): void {
	testMkdirSync(path.dirname(pathname), { recursive: true });
	testWriteFileSync(pathname, content);
	testChmodSync(pathname, 0o700);
}

function fakeRevdiffScript(): string {
	return [
		"#!/bin/sh",
		"test \"$REVDIFF_EXIT_CODE_ON_ANNOTATIONS\" = \"true\" || exit 21",
		"out=",
		"for arg in \"$@\"; do",
		"  case \"$arg\" in --output=*) out=${arg#--output=};; esac",
		"done",
		"test -n \"$out\" || exit 22",
		"printf '## src/app.go:12-14 (+)\\nfix it\\n' > \"$out\"",
		"printf '%s\\n' \"$@\" > \"$FAKE_ARG_FILE\"",
		"exit 10",
		"",
	].join("\n");
}

function fakeSignalRevdiffScript(): string {
	return ["#!/bin/sh", "kill -TERM $$", ""].join("\n");
}

async function testCommandSendsAnnotations(): Promise<void> {
	const tempDir = mkdtempSync(path.join(tmpdir(), "pi-revdiff-command-"));
	const fakeBin = path.join(tempDir, "revdiff");
	const argFile = path.join(tempDir, "args.txt");
	writeExecutable(fakeBin, fakeRevdiffScript());

	const oldBin = process.env.REVDIFF_BIN;
	const oldArgFile = process.env.FAKE_ARG_FILE;
	process.env.REVDIFF_BIN = fakeBin;
	process.env.FAKE_ARG_FILE = argFile;
	try {
		const pi = fakePi();
		revdiffExtension(pi);
		await pi.commands.get("revdiff").handler("--only 'docs/my plan.md'", fakeCtx());

		testAssert(pi.sentMessages.length === 1, "expected captured annotations to be sent to the agent");
		const prompt = pi.sentMessages[0];
		testAssert(prompt.includes("Review target: docs/my plan.md"), "prompt should include review target");
		testAssert(prompt.includes("Original command: revdiff --only 'docs/my plan.md'"), "prompt should shell-quote original args");
		testAssert(prompt.includes("Rerun command: Call revdiff_review with args: --only 'docs/my plan.md'"), "prompt should include round-trippable rerun args");
		testAssert(prompt.includes("## src/app.go:12-14 (+)"), "prompt should include captured hunk annotation header");
		testAssert(prompt.includes("fix it"), "prompt should include captured annotation body");

		const args = readFileSync(argFile, "utf8").trim().split("\n");
		assertArray(args.slice(0, 2), ["--only", "docs/my plan.md"], "fake revdiff should receive split review args");
		testAssert(args[2]?.startsWith("--output="), "fake revdiff should receive output file arg");
	} finally {
		if (oldBin === undefined) {
			delete process.env.REVDIFF_BIN;
		} else {
			process.env.REVDIFF_BIN = oldBin;
		}
		if (oldArgFile === undefined) {
			delete process.env.FAKE_ARG_FILE;
		} else {
			process.env.FAKE_ARG_FILE = oldArgFile;
		}
		rmSync(tempDir, { recursive: true, force: true });
	}
}

async function testSignalTerminatedReviewFails(): Promise<void> {
	const tempDir = mkdtempSync(path.join(tmpdir(), "pi-revdiff-signal-"));
	const fakeBin = path.join(tempDir, "revdiff");
	writeExecutable(fakeBin, fakeSignalRevdiffScript());

	const oldBin = process.env.REVDIFF_BIN;
	process.env.REVDIFF_BIN = fakeBin;
	try {
		const pi = fakePi();
		const ctx = fakeCtx();
		revdiffExtension(pi);
		await pi.commands.get("revdiff").handler("--only README.md", ctx);

		testAssert(pi.sentMessages.length === 0, "signal-terminated revdiff must not send annotations");
		testAssert(
			ctx.ui.notifications.some((message: string) => message.includes("terminated by signal")),
			"signal-terminated revdiff should notify failure",
		);
	} finally {
		if (oldBin === undefined) {
			delete process.env.REVDIFF_BIN;
		} else {
			process.env.REVDIFF_BIN = oldBin;
		}
		rmSync(tempDir, { recursive: true, force: true });
	}
}

async function testArgumentResolution(): Promise<void> {
	let launch = await resolveLaunchSpec("--output ignored --only 'docs/my plan.md'", fakeCtx());
	testAssert(Boolean(launch), "expected launch after stripping --output");
	assertArray(launch!.args, ["--only", "docs/my plan.md"], "--output stripping should preserve remaining args");
	testAssert(launch!.label === "docs/my plan.md", "--only label should use target path");

	launch = await resolveLaunchSpec("all-files exclude vendor and dist", fakeCtx());
	testAssert(Boolean(launch), "expected all-files shortcut launch");
	assertArray(launch!.args, ["--all-files", "--exclude=vendor", "--exclude=dist"], "all-files shortcut should expand excludes");

	launch = await resolveLaunchSpec("docs/new-file.md", fakeCtx());
	testAssert(Boolean(launch), "expected path-like file launch");
	assertArray(launch!.args, ["--only", "docs/new-file.md"], "path-like file arg should map to --only");

	const roundTrip = ["--description=why this matters", "--only", "docs/it's mine.md"];
	assertArray(shellSplit(shellJoin(roundTrip)), roundTrip, "shellJoin output should shellSplit back to original args");
}

function runGit(repo: string, args: string[]): void {
	const result = spawnSync("git", args, { cwd: repo, encoding: "utf8" });
	testAssert(result.status === 0, "git " + args.join(" ") + " failed: " + (result.stderr || result.stdout));
}

function initGitRepo(): string {
	const repo = mkdtempSync(path.join(tmpdir(), "pi-revdiff-git-"));
	runGit(repo, ["init"]);
	runGit(repo, ["checkout", "-b", "main"]);
	runGit(repo, ["config", "user.email", "test@example.com"]);
	runGit(repo, ["config", "user.name", "Test User"]);
	testWriteFileSync(path.join(repo, "file.txt"), "base\n");
	runGit(repo, ["add", "file.txt"]);
	runGit(repo, ["commit", "-m", "initial"]);
	return repo;
}

async function testNeedsAskWithoutMainStops(): Promise<void> {
	const detectScript = path.resolve(".claude-plugin", "skills", "revdiff", "scripts", "detect-ref.sh");
	writeExecutable(
		detectScript,
		[
			"#!/bin/sh",
			"echo 'branch: @'",
			"echo 'main_branch: '",
			"echo 'is_main: false'",
			"echo 'has_uncommitted: false'",
			"echo 'has_staged_only: false'",
			"echo 'suggested_ref: '",
			"echo 'use_staged: false'",
			"echo 'needs_ask: true'",
			"",
		].join("\n"),
	);

	const ctx = fakeCtx();
	const launch = await detectSmartLaunch(ctx);
	testAssert(launch === undefined, "needsAsk without a main branch should not launch uncommitted review");
	testAssert(
		ctx.ui.notifications.some((message: string) => message.includes("Could not determine a revdiff target")),
		"needsAsk without a main branch should notify a clear target error",
	);
	rmSync(path.resolve(".claude-plugin"), { recursive: true, force: true });
}

async function testStagedSmartDetection(): Promise<void> {
	const oldCwd = process.cwd();
	const mainRepo = initGitRepo();
	const featureRepo = initGitRepo();
	try {
		testWriteFileSync(path.join(mainRepo, "file.txt"), "main staged\n");
		runGit(mainRepo, ["add", "file.txt"]);
		process.chdir(mainRepo);
		let launch = await detectSmartLaunch(fakeCtx());
		testAssert(Boolean(launch), "expected staged launch on main");
		assertArray(launch!.args, ["--staged"], "main staged-only should launch --staged");
		testAssert(launch!.label === "staged changes", "main staged-only label should be staged changes");

		runGit(featureRepo, ["checkout", "-b", "feature"]);
		testWriteFileSync(path.join(featureRepo, "file.txt"), "feature staged\n");
		runGit(featureRepo, ["add", "file.txt"]);
		process.chdir(featureRepo);
		launch = await detectSmartLaunch(fakeCtx("uncommitted"));
		testAssert(Boolean(launch), "expected dirty feature uncommitted launch");
		assertArray(launch!.args, ["--staged"], "dirty feature uncommitted choice should launch --staged");

		launch = await detectSmartLaunch(fakeCtx("branch"));
		testAssert(Boolean(launch), "expected dirty feature branch launch");
		assertArray(launch!.args, ["main"], "dirty feature branch choice should preserve branch diff");
		testAssert(launch!.label === "feature vs main", "dirty feature branch label should identify main branch");
	} finally {
		process.chdir(oldCwd);
		rmSync(mainRepo, { recursive: true, force: true });
		rmSync(featureRepo, { recursive: true, force: true });
	}
}

await testCommandSendsAnnotations();
await testSignalTerminatedReviewFails();
await testArgumentResolution();
await testNeedsAskWithoutMainStops();
await testStagedSmartDetection();
console.log("pi extension executable behavior ok");
`
}

func testRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Dir(wd)
}

func runTestCmd(t *testing.T, r cmdReq) cmdResult {
	t.Helper()
	cmd := exec.Command(r.name, r.args...) //nolint:gosec // tests execute fixed repo scripts and temp fixtures
	cmd.Dir = r.dir
	cmd.Env = mergeEnv(r.env)
	if r.stdin != "" {
		cmd.Stdin = strings.NewReader(r.stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return cmdResult{stdout: stdout.String(), stderr: stderr.String(), code: commandExitCode(err)}
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}

func mergeEnv(overrides map[string]string) []string {
	if len(overrides) == 0 {
		return os.Environ()
	}
	keys := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		keys[k] = struct{}{}
	}
	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, item := range os.Environ() {
		key, _, ok := strings.Cut(item, "=")
		if _, found := keys[key]; ok && found {
			continue
		}
		env = append(env, item)
	}
	for k, v := range overrides {
		env = append(env, k+"="+v)
	}
	return env
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	writeTestFile(t, path, content)
	require.NoError(t, os.Chmod(path, 0o700)) //nolint:gosec // test fixtures must be executable
}

func readRepoFile(t *testing.T, root string, elems ...string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(append([]string{root}, elems...)...))
	require.NoError(t, err)
	return string(b)
}

func testFixtureScript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "plugin-exit-code", name)) //nolint:gosec // tests read fixed fixture files
	require.NoError(t, err)
	return string(b)
}

func launcherBackends() []launcherBackend {
	return []launcherBackend{
		{name: "tmux", command: "tmux", env: map[string]string{"TMUX": "1"}},
		{name: "zellij", command: "zellij", env: map[string]string{"ZELLIJ": "1"}},
		{name: "kitty", command: "kitty", env: map[string]string{"KITTY_LISTEN_ON": "unix:/tmp/kitty"}},
		{name: "wezterm", command: "wezterm", env: map[string]string{"WEZTERM_PANE": "1"}},
		{name: "cmux", command: "cmux", env: map[string]string{"CMUX_SURFACE_ID": "1"}},
		{name: "cmux ghostty env", command: "cmux", env: map[string]string{
			"TERM_PROGRAM":          "ghostty",
			"__CFBundleIdentifier":  "com.cmuxterm.app",
			"GHOSTTY_RESOURCES_DIR": "/Applications/cmux.app/Contents/Resources/ghostty",
			"GHOSTTY_BIN_DIR":       "/Applications/cmux.app/Contents/MacOS",
		}},
		{name: "ghostty", command: "osascript", env: map[string]string{"TERM_PROGRAM": "ghostty"}},
		{name: "iterm2", command: "osascript", env: map[string]string{"ITERM_SESSION_ID": "w0t0p0:ABC"}},
		{name: "emacs", command: "emacsclient", env: map[string]string{"INSIDE_EMACS": "vterm"}},
	}
}

func fakeLauncherEnv(t *testing.T, r launcherRun) map[string]string {
	t.Helper()
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	writeExecutable(t, filepath.Join(binDir, "revdiff"), testFixtureScript(t, "fake-revdiff-output.sh"))
	writeExecutable(t, filepath.Join(binDir, r.backend.command), testFixtureScript(t, "fake-overlay-backend.sh"))

	env := cleanOverlayEnv()
	maps.Copy(env, r.backend.env)
	env["FAKE_OUTPUT"] = r.output
	env["FAKE_RC"] = strconv.Itoa(r.code)
	env["PATH"] = binDir + string(os.PathListSeparator) + os.Getenv("PATH")
	env["TMPDIR"] = tmp
	return env
}

func cleanOverlayEnv() map[string]string {
	return map[string]string{
		"TMUX":                  "",
		"ZELLIJ":                "",
		"KITTY_LISTEN_ON":       "",
		"KITTY_WINDOW_ID":       "",
		"WEZTERM_PANE":          "",
		"CMUX_SURFACE_ID":       "",
		"TERM_PROGRAM":          "",
		"GHOSTTY_SURFACE_ID":    "",
		"GHOSTTY_RESOURCES_DIR": "",
		"GHOSTTY_BIN_DIR":       "",
		"__CFBundleIdentifier":  "",
		"ITERM_SESSION_ID":      "",
		"INSIDE_EMACS":          "",
		"REVDIFF_CONFIG":        "",
	}
}

func resolverScript(launcher string) string {
	return "#!/bin/sh\nprintf '%s\\n' " + shQuote(launcher) + "\n"
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func planSnapshots(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "plan-rev-*.md"))
	require.NoError(t, err)
	return matches
}
