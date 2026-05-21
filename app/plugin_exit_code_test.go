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
	assert.Contains(t, src, "const env = withAnnotationExitCode(withRevdiffOnPath(process.env, revdiffBin));")
	assert.Contains(t, src, "spawnSync(launcher, launch.args, {")
	assert.Contains(t, src, "return exitCode === 0 || exitCode === EXIT_CODE_ANNOTATIONS;")
	assert.Contains(t, src, "if (!outputExists && exitCode === EXIT_CODE_ANNOTATIONS)")
	assert.Contains(t, src, "return buildResult(launch, rawOutput);")
	assert.Contains(t, src, "return buildResult(launch, stdout);")
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
		"TMUX":             "",
		"ZELLIJ":           "",
		"KITTY_LISTEN_ON":  "",
		"KITTY_WINDOW_ID":  "",
		"WEZTERM_PANE":     "",
		"CMUX_SURFACE_ID":  "",
		"TERM_PROGRAM":     "",
		"ITERM_SESSION_ID": "",
		"INSIDE_EMACS":     "",
		"REVDIFF_CONFIG":   "",
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
