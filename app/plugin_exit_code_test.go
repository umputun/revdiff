package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
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

type pluginManifest struct {
	Hooks string `json:"hooks"`
}

type hookCommand struct {
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout"`
	StatusMessage string `json:"statusMessage"`
}

type hookRegistration struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookCommand `json:"hooks"`
}

type hookManifest struct {
	Hooks map[string][]hookRegistration `json:"hooks"`
}

// python3Path freezes the interpreter selected by the current PATH before a
// test replaces the child PATH. LookPath alone may return a version-manager
// shim whose target changes under the restricted environment.
func python3Path(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not found")
	}

	cmd := exec.Command(python, "-c", "import os, sys; print(os.path.realpath(sys.executable))") //nolint:gosec // executable comes from PATH; arguments are fixed
	output, err := cmd.Output()
	require.NoError(t, err, "resolve python3 executable")
	resolved := strings.TrimSpace(string(output))
	require.FileExists(t, resolved)
	return resolved
}

func assistantTranscriptLine(t *testing.T, phase, text string) string {
	t.Helper()
	message := map[string]any{
		"type": "message",
		"role": "assistant",
		"content": []map[string]string{{
			"type": "output_text",
			"text": text,
		}},
		"internal_chat_message_metadata_passthrough": map[string]string{
			"turn_id": "turn-current",
		},
	}
	if phase != "" {
		message["phase"] = phase
	}
	item := map[string]any{
		"type":    "response_item",
		"payload": message,
	}
	data, err := json.Marshal(item)
	require.NoError(t, err)
	return string(data) + "\n"
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
	python := python3Path(t)

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

func TestCodexPlanReviewHook(t *testing.T) {
	python := python3Path(t)

	root := testRepoRoot(t)
	hook := filepath.Join(root, "plugins", "revdiff-planning", "scripts", "codex-plan-review-hook.py")
	baseEvent := map[string]any{
		"hook_event_name":        "Stop",
		"permission_mode":        "plan",
		"stop_hook_active":       false,
		"cwd":                    root,
		"last_assistant_message": "<proposed_plan>\n# Plan\n- item\n</proposed_plan>",
	}
	payload := func(overrides map[string]any) string {
		event := maps.Clone(baseEvent)
		maps.Copy(event, overrides)
		data, err := json.Marshal(event)
		require.NoError(t, err)
		return string(data)
	}
	fallbackEvent := func(overrides map[string]any) map[string]any {
		event := map[string]any{
			"session_id":             "session-current",
			"turn_id":                "turn-current",
			"transcript_path":        "$TRANSCRIPT",
			"last_assistant_message": nil,
		}
		maps.Copy(event, overrides)
		return event
	}
	planTranscript := assistantTranscriptLine(
		t,
		"",
		"<proposed_plan>\n# Plan from transcript\n- item\n</proposed_plan>",
	)
	liveTranscript := filepath.Join(
		root,
		"app",
		"testdata",
		"plugin-exit-code",
		"rollout-2026-07-16T10-54-26-session-current.jsonl",
	)
	require.FileExists(t, liveTranscript)
	cases := []struct {
		name           string
		payload        string
		transcript     string
		transcriptPath string
		code           int
		output         string
		withBinary     bool
		wantWarning    bool
		wantDecision   string
		wantLaunch     bool
		wantPlan       string
	}{
		{name: "non Stop event", payload: payload(map[string]any{"hook_event_name": "SubagentStop"}), withBinary: true},
		{name: "default mode quoted plan skip", payload: payload(map[string]any{"permission_mode": "default"}), withBinary: true},
		{name: "build mode quoted plan skip", payload: payload(map[string]any{"permission_mode": "acceptEdits"}), withBinary: true},
		{name: "bypass mode quoted plan skip", payload: payload(map[string]any{"permission_mode": "bypassPermissions"}), withBinary: true},
		{name: "clean message plan", payload: payload(nil), withBinary: true, wantLaunch: true, wantPlan: "# Plan\n- item"},
		{name: "invalid cwd type ignored", payload: payload(map[string]any{"cwd": 123}), withBinary: true, wantLaunch: true, wantPlan: "# Plan\n- item"},
		{name: "active revise loop still launches", payload: payload(map[string]any{"stop_hook_active": true}), withBinary: true, wantLaunch: true, wantPlan: "# Plan\n- item"},
		{name: "annotations", payload: payload(nil), code: exitCodeAnnotations, output: "## plan.md:2 (+)\nrevise this\n", withBinary: true, wantDecision: "block", wantLaunch: true, wantPlan: "# Plan\n- item"},
		{name: "mixed prose falls back to transcript", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "I completed the plan; it follows below."})), transcript: planTranscript, withBinary: true, wantLaunch: true, wantPlan: "# Plan from transcript\n- item"},
		{name: "null message uses exact turn transcript", payload: payload(fallbackEvent(nil)), transcript: planTranscript, withBinary: true, wantLaunch: true, wantPlan: "# Plan from transcript\n- item"},
		{name: "last assistant message wins without phase", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "stripped plan"})), transcript: assistantTranscriptLine(t, "analysis", "<proposed_plan>\n# Old plan\n</proposed_plan>") + assistantTranscriptLine(t, "", "<proposed_plan>\n# New plan\n- later\n</proposed_plan>"), withBinary: true, wantLaunch: true, wantPlan: "# New plan\n- later"},
		{name: "plan message wins over planless closer", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "stripped plan"})), transcript: planTranscript + assistantTranscriptLine(t, "", "Plan is ready for review."), withBinary: true, wantLaunch: true, wantPlan: "# Plan from transcript\n- item"},
		{name: "plan message wins over empty block closer", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "stripped plan"})), transcript: planTranscript + assistantTranscriptLine(t, "", "<proposed_plan>\n</proposed_plan>"), withBinary: true, wantLaunch: true, wantPlan: "# Plan from transcript\n- item"},
		{name: "last non-empty block wins within message", payload: payload(map[string]any{"last_assistant_message": "<proposed_plan>\n# Valid plan\n- item\n</proposed_plan>\n<proposed_plan>\n</proposed_plan>"}), withBinary: true, wantLaunch: true, wantPlan: "# Valid plan\n- item"},
		{name: "sanitized live Codex rollout", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "stripped plan"})), transcriptPath: liveTranscript, withBinary: true, wantLaunch: true, wantPlan: "# Sanitized live Codex plan\n- verify transcript fallback"},
		{name: "clarification transcript skips", payload: payload(fallbackEvent(map[string]any{"last_assistant_message": "Need one clarification"})), transcript: assistantTranscriptLine(t, "", "Which database should this use?"), withBinary: true},
		{name: "transcript has no matching turn", payload: payload(fallbackEvent(map[string]any{"turn_id": "turn-missing"})), transcript: planTranscript, withBinary: true, wantWarning: true},
		{name: "transcript from another session", payload: payload(fallbackEvent(map[string]any{"session_id": "session-other"})), transcript: planTranscript, withBinary: true, wantWarning: true},
		{name: "missing transcript", payload: payload(fallbackEvent(map[string]any{"transcript_path": "$MISSING_TRANSCRIPT"})), withBinary: true, wantWarning: true},
		{name: "malformed transcript", payload: payload(fallbackEvent(nil)), transcript: "{not-json\n", withBinary: true, wantWarning: true},
		{name: "missing fallback identifiers", payload: payload(map[string]any{"last_assistant_message": "No plan block"}), withBinary: true, wantWarning: true},
		{name: "missing revdiff", payload: payload(nil), wantWarning: true},
		{name: "launcher failure", payload: payload(nil), code: 1, withBinary: true, wantWarning: true, wantLaunch: true, wantPlan: "# Plan\n- item"},
		{name: "malformed json", payload: `{`, withBinary: true, wantWarning: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmp := t.TempDir()
			payload := tc.payload
			if tc.transcriptPath != "" {
				payload = strings.Replace(payload, "$TRANSCRIPT", tc.transcriptPath, 1)
			} else if tc.transcript != "" {
				transcript := filepath.Join(tmp, "rollout-session-current.jsonl")
				writeTestFile(t, transcript, tc.transcript)
				payload = strings.Replace(payload, "$TRANSCRIPT", transcript, 1)
			}
			missingTranscript := filepath.Join(tmp, "missing-session-current.jsonl")
			payload = strings.Replace(payload, "$MISSING_TRANSCRIPT", missingTranscript, 1)
			pluginRoot := filepath.Join(tmp, "plugin")
			launcher := filepath.Join(tmp, "launch-plan-review.sh")
			launchLog := filepath.Join(tmp, "launch.log")
			argsLog := filepath.Join(tmp, "args.log")
			binDir := filepath.Join(tmp, "bin")
			require.NoError(t, os.MkdirAll(binDir, 0o700))
			writeExecutable(t, launcher, "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$#\" > \"$FAKE_ARGS\"\ncp \"$1\" \"$FAKE_LOG\"\nprintf \"%s\" \"${FAKE_OUTPUT:-}\"\nexit \"${FAKE_RC:-0}\"\n")
			writeExecutable(t, filepath.Join(pluginRoot, "scripts", "resolve-launcher.sh"), resolverScript(launcher))
			if tc.withBinary {
				writeExecutable(t, filepath.Join(binDir, "revdiff"), "#!/bin/sh\nexit 0\n")
			}
			pathValue := binDir
			if tc.withBinary {
				pathValue = binDir + string(os.PathListSeparator) + os.Getenv("PATH")
			}

			res := runTestCmd(t, cmdReq{
				dir:   root,
				name:  python,
				args:  []string{hook},
				stdin: payload,
				env: map[string]string{
					"PLUGIN_ROOT": pluginRoot,
					"FAKE_OUTPUT": tc.output,
					"FAKE_RC":     strconv.Itoa(tc.code),
					"FAKE_LOG":    launchLog,
					"FAKE_ARGS":   argsLog,
					"PATH":        pathValue,
					"TMPDIR":      tmp,
				},
			})
			require.Equal(t, 0, res.code, "stderr: %s", res.stderr)
			var got map[string]any
			require.NoError(t, json.Unmarshal([]byte(res.stdout), &got), "stdout: %s", res.stdout)
			switch {
			case tc.wantWarning:
				require.Len(t, got, 1)
				assert.NotEmpty(t, got["systemMessage"])
			case tc.wantDecision != "":
				require.Len(t, got, 2)
				assert.Equal(t, tc.wantDecision, got["decision"])
				assert.Contains(t, got["reason"], strings.TrimSpace(tc.output))
				assert.Contains(t, got["reason"], "Do NOT substitute any other plan-rev-*.md path")
			default:
				assert.Empty(t, got)
			}
			_, logErr := os.Stat(launchLog)
			if tc.wantLaunch {
				require.NoError(t, logErr)
				plan, readErr := os.ReadFile(launchLog) //nolint:gosec // path is a test-owned temp file
				require.NoError(t, readErr)
				assert.Equal(t, tc.wantPlan, string(plan))
				args, readArgsErr := os.ReadFile(argsLog) //nolint:gosec // path is a test-owned temp file
				require.NoError(t, readArgsErr)
				assert.Equal(t, "1\n", string(args))
			} else {
				assert.ErrorIs(t, logErr, fs.ErrNotExist)
			}
		})
	}
}

func TestCodexPlanReviewHookRollingReview(t *testing.T) {
	python := python3Path(t)

	root := testRepoRoot(t)
	hook := filepath.Join(root, "plugins", "revdiff-planning", "scripts", "codex-plan-review-hook.py")
	tmp := t.TempDir()
	pluginRoot := filepath.Join(tmp, "plugin")
	launcher := filepath.Join(tmp, "launch-plan-review.sh")
	launchLog := filepath.Join(tmp, "launch.log")
	oldLog := filepath.Join(tmp, "old.log")
	argsLog := filepath.Join(tmp, "args.log")
	binDir := filepath.Join(tmp, "bin")
	writeExecutable(t, launcher, "#!/usr/bin/env bash\nset -euo pipefail\nprintf '%s\\n' \"$#\" > \"$FAKE_ARGS\"\ncp \"$1\" \"$FAKE_LOG\"\nif [ \"$#\" -eq 2 ]; then cp \"$2\" \"$FAKE_OLD\"; fi\nprintf \"%s\" \"${FAKE_OUTPUT:-}\"\nexit \"${FAKE_RC:-0}\"\n")
	writeExecutable(t, filepath.Join(pluginRoot, "scripts", "resolve-launcher.sh"), resolverScript(launcher))
	writeExecutable(t, filepath.Join(binDir, "revdiff"), "#!/bin/sh\nexit 0\n")

	runHook := func(payload string, code int, output string) map[string]any {
		t.Helper()
		res := runTestCmd(t, cmdReq{
			dir:   root,
			name:  python,
			args:  []string{hook},
			stdin: payload,
			env: map[string]string{
				"PLUGIN_ROOT": pluginRoot,
				"FAKE_OUTPUT": output,
				"FAKE_RC":     strconv.Itoa(code),
				"FAKE_LOG":    launchLog,
				"FAKE_OLD":    oldLog,
				"FAKE_ARGS":   argsLog,
				"PATH":        binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				"TMPDIR":      tmp,
			},
		})
		require.Equal(t, 0, res.code, "stderr: %s", res.stderr)
		var response map[string]any
		require.NoError(t, json.Unmarshal([]byte(res.stdout), &response), "stdout: %s", res.stdout)
		return response
	}

	untrustedSnapshot := filepath.Join(tmp, "untrusted.md")
	writeTestFile(t, untrustedSnapshot, "# Untrusted plan\n")
	untrustedPayload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"<proposed_plan>\n<!-- previous revision: ` + untrustedSnapshot + ` -->\n# Plan with untrusted marker\n</proposed_plan>"}`
	untrusted := runHook(untrustedPayload, 0, "")
	assert.Empty(t, untrusted)
	assertFileContent(t, argsLog, "1\n")
	assertFileContent(t, launchLog, "# Plan with untrusted marker")
	assertFileContent(t, untrustedSnapshot, "# Untrusted plan\n")

	outsideDir, err := os.MkdirTemp("", "revdiff-plan-review-")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, os.RemoveAll(outsideDir)) })
	outsideSnapshot := filepath.Join(outsideDir, "plan-rev-outside.md")
	writeTestFile(t, outsideSnapshot, "# Outside plan\n")
	outsidePayload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"<proposed_plan>\n<!-- previous revision: ` + outsideSnapshot + ` -->\n# Plan with outside marker\n</proposed_plan>"}`
	outside := runHook(outsidePayload, 0, "")
	assert.Empty(t, outside)
	assertFileContent(t, argsLog, "1\n")
	assertFileContent(t, launchLog, "# Plan with outside marker")
	assertFileContent(t, outsideSnapshot, "# Outside plan\n")

	first := runHook(`{"hook_event_name":"Stop","permission_mode":"plan","stop_hook_active":false,"last_assistant_message":"<proposed_plan>\n# Plan\n- first\n</proposed_plan>"}`, exitCodeAnnotations, "revise first")
	assert.Equal(t, "block", first["decision"])
	assert.Contains(t, first["reason"], "Do NOT substitute any other plan-rev-*.md path")
	firstSnapshots := planSnapshots(t, tmp)
	require.Len(t, firstSnapshots, 1)
	firstSnapshot := firstSnapshots[0]
	assert.Contains(t, first["reason"], "<!-- previous revision: "+firstSnapshot+" -->")
	assertFileContent(t, argsLog, "1\n")

	secondPayload := `{"hook_event_name":"Stop","permission_mode":"plan","stop_hook_active":true,"last_assistant_message":"<proposed_plan>\n<!-- previous revision: ` + firstSnapshot + ` -->\n# Revised plan\n- second\n</proposed_plan>"}`
	second := runHook(secondPayload, exitCodeAnnotations, "revise second")
	assert.Equal(t, "block", second["decision"])
	secondSnapshots := planSnapshots(t, tmp)
	require.Len(t, secondSnapshots, 1)
	secondSnapshot := secondSnapshots[0]
	assert.NotEqual(t, firstSnapshot, secondSnapshot)
	assert.NoFileExists(t, firstSnapshot)
	assert.Contains(t, second["reason"], "<!-- previous revision: "+secondSnapshot+" -->")
	assertFileContent(t, argsLog, "2\n")
	assertFileContent(t, launchLog, "# Revised plan\n- second")
	assertFileContent(t, oldLog, "# Plan\n- first")

	failedPayload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"<proposed_plan>\n<!-- previous revision: ` + secondSnapshot + ` -->\n# Failed attempt\n</proposed_plan>"}`
	failed := runHook(failedPayload, 1, "")
	assert.NotEmpty(t, failed["systemMessage"])
	assert.FileExists(t, secondSnapshot)
	assert.Equal(t, []string{secondSnapshot}, planSnapshots(t, tmp))

	cleanPayload := `{"hook_event_name":"Stop","permission_mode":"plan","last_assistant_message":"<proposed_plan>\n<!-- previous revision: ` + secondSnapshot + ` -->\n# Final plan\n</proposed_plan>"}`
	clean := runHook(cleanPayload, 0, "")
	assert.Empty(t, clean)
	assert.Empty(t, planSnapshots(t, tmp))
	assertFileContent(t, argsLog, "2\n")
	assertFileContent(t, launchLog, "# Final plan")
	assertFileContent(t, oldLog, "# Revised plan\n- second")
}

func TestPlanningPluginHookWiring(t *testing.T) {
	root := testRepoRoot(t)
	claudeManifest := readRepoJSON[pluginManifest](t, root, "plugins", "revdiff-planning", ".claude-plugin", "plugin.json")
	claudeHooks := readRepoJSON[hookManifest](t, root, "plugins", "revdiff-planning", "hooks", "hooks.json")
	codexManifest := readRepoJSON[pluginManifest](t, root, "plugins", "revdiff-planning", ".codex-plugin", "plugin.json")
	codexHooks := readRepoJSON[hookManifest](t, root, "plugins", "revdiff-planning", "hooks", "codex-hooks.json")

	assert.Empty(t, claudeManifest.Hooks)
	require.Len(t, claudeHooks.Hooks, 1)
	require.Len(t, claudeHooks.Hooks["PreToolUse"], 1)
	assert.Equal(t, "ExitPlanMode", claudeHooks.Hooks["PreToolUse"][0].Matcher)
	require.Len(t, claudeHooks.Hooks["PreToolUse"][0].Hooks, 1)
	assert.Equal(t, hookCommand{
		Type:    "command",
		Command: `python3 "${CLAUDE_PLUGIN_ROOT}/scripts/plan-review-hook.py"`,
		Timeout: 345600,
	}, claudeHooks.Hooks["PreToolUse"][0].Hooks[0])

	assert.Equal(t, "./hooks/codex-hooks.json", codexManifest.Hooks)
	require.Len(t, codexHooks.Hooks, 1)
	require.Len(t, codexHooks.Hooks["Stop"], 1)
	assert.Empty(t, codexHooks.Hooks["Stop"][0].Matcher)
	require.Len(t, codexHooks.Hooks["Stop"][0].Hooks, 1)
	assert.Equal(t, hookCommand{
		Type:          "command",
		Command:       `python3 "${PLUGIN_ROOT}/scripts/codex-plan-review-hook.py"`,
		Timeout:       345600,
		StatusMessage: "Reviewing proposed plan with RevDiff",
	}, codexHooks.Hooks["Stop"][0].Hooks[0])
	assert.NoFileExists(t, filepath.Join(root, "plugins", "revdiff-planning", "hooks", "claude-hooks.json"))
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
	assert.Contains(t, src, "return buildResult(launch, rawOutput, cwd);")
}

func TestPiExecutableRegressionHasCIBunSetup(t *testing.T) {
	root := testRepoRoot(t)
	ci := readRepoFile(t, root, ".github", "workflows", "ci.yml")
	assert.Contains(t, ci, "oven-sh/setup-bun")
	assert.Contains(t, ci, "go test -race")
}

func TestPiExtensionExecutableBehavior(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is not installed locally; CI installs Bun via oven-sh/setup-bun and runs this regression")
	}

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
import { chmodSync as testChmodSync, mkdirSync as testMkdirSync, realpathSync as testRealpathSync, writeFileSync as testWriteFileSync } from "node:fs";

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
		cwd: process.cwd(),
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
		"[ -z \"${FAKE_CWD_FILE:-}\" ] || pwd > \"$FAKE_CWD_FILE\"",
		"exit 10",
		"",
	].join("\n");
}

function fakeSignalRevdiffScript(): string {
	return ["#!/bin/sh", "kill -TERM $$", ""].join("\n");
}

async function testCommandRoutesToSkill(): Promise<void> {
	const pi = fakePi();
	revdiffExtension(pi);

	await pi.commands.get("revdiff").handler("last tag", fakeCtx());
	testAssert(pi.sentMessages.length === 1, "expected /revdiff to route through the skill");
	testAssert(pi.sentMessages[0] === "/skill:revdiff last tag", "expected /revdiff args to be passed to /skill:revdiff");

	await pi.commands.get("revdiff").handler("", fakeCtx());
	testAssert(pi.sentMessages.length === 2, "expected no-arg /revdiff to route through the skill");
	testAssert(pi.sentMessages[1] === "/skill:revdiff", "expected no-arg /revdiff to call the skill without args");
}

async function testToolReturnsAnnotations(): Promise<void> {
	const tempDir = mkdtempSync(path.join(tmpdir(), "pi-revdiff-tool-"));
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
		const result = await pi.tools.get("revdiff_review").execute("call-1", { args: "--only README.md" }, undefined, undefined, fakeCtx());
		const text = result.content[0].text;
		testAssert(text.includes("Captured 1 annotation for README.md."), "tool result should summarize captured annotations");
		testAssert(text.includes("Annotations:\n## src/app.go:12-14 (+)\nfix it"), "tool result should include raw annotation text");
		testAssert(result.details.rawOutput.includes("## src/app.go:12-14 (+)"), "tool details should preserve raw output");
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

function testReviewCwdResolution(): void {
	const base = mkdtempSync(path.join(tmpdir(), "pi-revdiff-cwd-base-"));
	const child = path.join(base, "child");
	testMkdirSync(child, { recursive: true });
	try {
		const resolvedHome = resolveReviewCwd("~/", base);
		testAssert(resolvedHome === homedir(), "~/ should expand to the home directory");
		testAssert(resolveReviewCwd("child", base) === child, "relative cwd should resolve against ctx cwd");
	} finally {
		rmSync(base, { recursive: true, force: true });
	}
}

async function testToolRejectsInvalidCwd(): Promise<void> {
	const tempDir = mkdtempSync(path.join(tmpdir(), "pi-revdiff-invalid-cwd-"));
	const file = path.join(tempDir, "not-a-dir.txt");
	testWriteFileSync(file, "not a directory\n");
	try {
		const pi = fakePi();
		revdiffExtension(pi);
		const fileResult = await pi.tools.get("revdiff_review").execute("call-1", { cwd: file }, undefined, undefined, fakeCtx());
		testAssert(fileResult.content[0].text === "Could not resolve revdiff working directory.", "file cwd should return a clear error");
		const missingResult = await pi.tools
			.get("revdiff_review")
			.execute("call-2", { cwd: path.join(tempDir, "missing") }, undefined, undefined, fakeCtx());
		testAssert(missingResult.content[0].text === "Could not resolve revdiff working directory.", "missing cwd should return a clear error");
	} finally {
		rmSync(tempDir, { recursive: true, force: true });
	}
}

async function testToolCwdParameter(): Promise<void> {
	const tempDir = mkdtempSync(path.join(tmpdir(), "pi-revdiff-cwd-"));
	const fakeBin = path.join(tempDir, "revdiff");
	const targetRepo = path.join(tempDir, "repo with spaces");
	const argFile = path.join(tempDir, "args.txt");
	const cwdFile = path.join(tempDir, "cwd.txt");
	writeExecutable(fakeBin, fakeRevdiffScript());
	testMkdirSync(targetRepo, { recursive: true });
	testWriteFileSync(path.join(targetRepo, "README.md"), "hello\n");

	const oldBin = process.env.REVDIFF_BIN;
	const oldArgFile = process.env.FAKE_ARG_FILE;
	const oldCwdFile = process.env.FAKE_CWD_FILE;
	process.env.REVDIFF_BIN = fakeBin;
	process.env.FAKE_ARG_FILE = argFile;
	process.env.FAKE_CWD_FILE = cwdFile;
	try {
		const pi = fakePi();
		revdiffExtension(pi);
		const result = await pi.tools.get("revdiff_review").execute("call-1", { args: "README.md", cwd: targetRepo }, undefined, undefined, fakeCtx());
		const text = result.content[0].text;
		testAssert(text.includes("Captured 1 annotation for README.md."), "cwd parameter should still review target file");
		testAssert(result.details.cwd === targetRepo, "tool details should preserve cwd for reruns");
		testAssert(
			testRealpathSync(readFileSync(cwdFile, "utf8").trim()) === testRealpathSync(targetRepo),
			"revdiff should launch in requested directory",
		);
		let argText = readFileSync(argFile, "utf8");
		testAssert(argText.includes("--only\nREADME.md\n"), "file target in requested directory should resolve to --only README.md");
		testAssert(!argText.includes(targetRepo), "cwd parameter should not be passed as a revdiff argument");

		rmSync(cwdFile, { force: true });
		const rerun = await pi.tools
			.get("revdiff_review")
			.execute("call-2", { args: result.details.argsText, cwd: result.details.cwd }, undefined, undefined, fakeCtx());
		testAssert(rerun.details.cwd === targetRepo, "rerun details should preserve cwd");
		testAssert(
			testRealpathSync(readFileSync(cwdFile, "utf8").trim()) === testRealpathSync(targetRepo),
			"rerun should launch in requested directory",
		);
		argText = readFileSync(argFile, "utf8");
		testAssert(argText.includes("--only\nREADME.md\n"), "rerun should use returned args with preserved cwd");
		testAssert(!argText.includes(targetRepo), "rerun cwd should not be passed as a revdiff argument");
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
		if (oldCwdFile === undefined) {
			delete process.env.FAKE_CWD_FILE;
		} else {
			process.env.FAKE_CWD_FILE = oldCwdFile;
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
		const result = await pi.tools.get("revdiff_review").execute("call-1", { args: "--only README.md" }, undefined, undefined, ctx);

		testAssert(result.content[0].text === "revdiff review did not complete.", "signal-terminated revdiff should return incomplete result");
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
	const cwd = process.cwd();
	let launch = await resolveLaunchSpec("--output ignored --only 'docs/my plan.md'", fakeCtx(), cwd);
	testAssert(Boolean(launch), "expected launch after stripping --output");
	assertArray(launch!.args, ["--only", "docs/my plan.md"], "--output stripping should preserve remaining args");
	testAssert(launch!.label === "docs/my plan.md", "--only label should use target path");

	launch = await resolveLaunchSpec("all-files exclude vendor and dist", fakeCtx(), cwd);
	testAssert(Boolean(launch), "expected all-files shortcut launch");
	assertArray(launch!.args, ["--all-files", "--exclude=vendor", "--exclude=dist"], "all-files shortcut should expand excludes");

	launch = await resolveLaunchSpec("./docs/new-file.md", fakeCtx(), cwd);
	testAssert(Boolean(launch), "expected explicit path file launch");
	assertArray(launch!.args, ["--only", "./docs/new-file.md"], "explicit path arg should map to --only");

	launch = await resolveLaunchSpec("release/v1.2.3", fakeCtx(), cwd);
	testAssert(Boolean(launch), "expected slash-dot token launch");
	assertArray(launch!.args, ["release/v1.2.3"], "slash-dot token should stay a ref-like arg when path does not exist");

	const roundTrip = ["--description=why this matters", "--only", "docs/it's mine.md"];
	assertArray(shellSplit(shellJoin(roundTrip)), roundTrip, "shellJoin output should shellSplit back to original args");
}

async function testRefLikePathArgKeepsRef(): Promise<void> {
	const oldCwd = process.cwd();
	const repo = initGitRepo();
	try {
		runGit(repo, ["checkout", "-b", "release/v1.2.3"]);
		process.chdir(repo);
		const launch = await resolveLaunchSpec("release/v1.2.3", fakeCtx(), repo);
		testAssert(Boolean(launch), "expected ref-like launch");
		assertArray(launch!.args, ["release/v1.2.3"], "ref-like path arg should stay a ref when the git ref exists");
		testAssert(launch!.label === "release/v1.2.3", "ref-like path label should stay the ref");
	} finally {
		process.chdir(oldCwd);
		rmSync(repo, { recursive: true, force: true });
	}
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
	const detectScript = path.resolve("plugins", "pi", "scripts", "detect-ref.sh");
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

	try {
		const ctx = fakeCtx();
		const launch = await detectSmartLaunch(ctx, process.cwd());
		testAssert(launch === undefined, "needsAsk without a main branch should not launch uncommitted review");
		testAssert(
			ctx.ui.notifications.some((message: string) => message.includes("Could not determine a revdiff target")),
			"needsAsk without a main branch should notify a clear target error",
		);
	} finally {
		rmSync(path.resolve("plugins", "pi", "scripts"), { recursive: true, force: true });
	}
}

async function testStagedSmartDetection(): Promise<void> {
	const oldCwd = process.cwd();
	const mainRepo = initGitRepo();
	const featureRepo = initGitRepo();
	try {
		testWriteFileSync(path.join(mainRepo, "file.txt"), "main staged\n");
		runGit(mainRepo, ["add", "file.txt"]);
		let launch = await detectSmartLaunch(fakeCtx(), mainRepo);
		testAssert(Boolean(launch), "expected staged launch on main");
		assertArray(launch!.args, ["--staged"], "main staged-only should launch --staged");
		testAssert(launch!.label === "staged changes", "main staged-only label should be staged changes");

		runGit(featureRepo, ["checkout", "-b", "feature"]);
		testWriteFileSync(path.join(featureRepo, "file.txt"), "feature staged\n");
		runGit(featureRepo, ["add", "file.txt"]);
		launch = await detectSmartLaunch(fakeCtx("uncommitted"), featureRepo);
		testAssert(Boolean(launch), "expected dirty feature uncommitted launch");
		assertArray(launch!.args, ["--staged"], "dirty feature uncommitted choice should launch --staged");

		launch = await detectSmartLaunch(fakeCtx("branch"), featureRepo);
		testAssert(Boolean(launch), "expected dirty feature branch launch");
		assertArray(launch!.args, ["main"], "dirty feature branch choice should preserve branch diff");
		testAssert(launch!.label === "feature vs main", "dirty feature branch label should identify main branch");
	} finally {
		process.chdir(oldCwd);
		rmSync(mainRepo, { recursive: true, force: true });
		rmSync(featureRepo, { recursive: true, force: true });
	}
}

await testCommandRoutesToSkill();
await testToolReturnsAnnotations();
testReviewCwdResolution();
await testToolRejectsInvalidCwd();
await testToolCwdParameter();
await testSignalTerminatedReviewFails();
await testArgumentResolution();
await testRefLikePathArgKeepsRef();
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

func readRepoJSON[T any](t *testing.T, root string, elems ...string) T {
	t.Helper()
	var value T
	require.NoError(t, json.Unmarshal([]byte(readRepoFile(t, root, elems...)), &value))
	return value
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
		{name: "herdr", command: "herdr", env: map[string]string{"HERDR_ENV": "1"}},
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
		{name: "agterm", command: "agtermctl", env: map[string]string{"AGTERM_SESSION_ID": "sess-1"}},
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
		"HERDR_ENV":             "",
		"HERDR_SOCKET_PATH":     "",
		"HERDR_PANE_ID":         "",
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
		"AGTERM_SESSION_ID":     "",
		"AGTERM_SOCKET":         "",
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

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path) //nolint:gosec // path is a test-owned temp file
	require.NoError(t, err)
	assert.Equal(t, want, string(content))
}
