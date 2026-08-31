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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cmdReq struct {
	dir   string
	name  string
	stdin string
	args  []string
	env   map[string]string
	// own process group: required whenever the command may signal its own group, or the
	// kill lands on `go test` itself and takes the whole run down
	setpgid bool
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
	stderr  string
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
	unknownFlagError := "revdiff: unknown flag `--bogus-flag'"

	launchers := []struct {
		name         string
		path         string
		args         []string
		relaysStderr bool
	}{
		{name: "claude", path: ".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh", relaysStderr: true},
		{name: "codex", path: "plugins/codex/skills/revdiff/scripts/launch-revdiff.sh", relaysStderr: true},
		{name: "plan review", path: "plugins/revdiff-planning/scripts/launch-plan-review.sh", args: []string{planFile}},
	}
	// stderr relay fires on failure only: 0 is a clean quit and 10 means
	// annotations were captured, and revdiff writes ordinary warnings to stderr,
	// so relaying either would put noise on every successful review
	cases := []struct {
		name       string
		code       int
		output     string
		wantStderr bool
	}{
		{name: "clean", code: 0},
		{name: "annotations", code: exitCodeAnnotations, output: "## file.go:1 (+)\ncomment\n"},
		{name: "failure", code: 1, output: "partial output\n", wantStderr: true},
	}

	for _, launcher := range launchers {
		for _, backend := range launcherBackends() {
			t.Run(launcher.name+"/"+backend.name, func(t *testing.T) {
				for _, tc := range cases {
					t.Run(tc.name, func(t *testing.T) {
						run := launcherRun{backend: backend, code: tc.code, output: tc.output}
						if launcher.relaysStderr {
							run.stderr = unknownFlagError
						}
						env := fakeLauncherEnv(t, run)
						script := filepath.Join(root, launcher.path)
						args := append([]string{script}, launcher.args...)
						res := runTestCmd(t, cmdReq{
							dir:  root,
							name: "bash",
							args: args,
							env:  env,
						})
						assert.Equal(t, tc.code, res.code)
						assert.Equal(t, tc.output, res.stdout)
						if launcher.relaysStderr && tc.wantStderr {
							assert.Contains(t, res.stderr, unknownFlagError)
						} else {
							assert.Empty(t, res.stderr)
						}
						// each backend that overrides the base EXIT trap has to
						// name the capture file; one that forgets leaks it
						assert.Empty(t, leftoverStderrCaptures(t, env["TMPDIR"]))
					})
				}
			})
		}
	}
}

// pins #314: an apostrophe in a heredoc nested inside a command substitution
// breaks the whole launcher under bash 3.2, the stock macOS /bin/bash. the
// launchers cannot be parse-checked for it here — CI and most dev machines run
// bash 5, which accepts the broken form — so the guard is textual and portable.
func TestLauncherNestedHeredocsHaveNoApostrophes(t *testing.T) {
	root := testRepoRoot(t)
	paths := []string{
		".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh",
		"plugins/codex/skills/revdiff/scripts/launch-revdiff.sh",
		"plugins/revdiff-planning/scripts/launch-plan-review.sh",
	}

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			bodies := nestedHeredocBodies(readRepoFile(t, root, path))
			require.NotEmpty(t, bodies, "no command-substitution heredoc found; the scan no longer matches the script")
			for _, hd := range bodies {
				for _, line := range hd.lines {
					assert.NotContains(t, line.text, "'",
						"%s:%d is inside the heredoc opened at line %d, which bash 3.2 scans for quotes as part of "+
							"the enclosing $( ); an apostrophe here fails the whole script. reword to avoid it",
						path, line.num, hd.openedAt)
				}
			}
		})
	}
}

type heredocLine struct {
	text string
	num  int
}

type nestedHeredoc struct {
	lines    []heredocLine
	openedAt int
}

// nestedHeredocBodies returns the body of every heredoc opened on a line that also
// opens a command substitution. a heredoc outside one is unaffected by the bash 3.2
// scan, so including it would ban apostrophes the shell handles correctly.
func nestedHeredocBodies(script string) []nestedHeredoc {
	opener := regexp.MustCompile(`\$\(.*<<-?\s*'?([A-Za-z_][A-Za-z0-9_]*)'?`)
	var found []nestedHeredoc
	var current *nestedHeredoc
	var terminator string

	for i, line := range strings.Split(script, "\n") {
		num := i + 1
		if current != nil {
			if strings.TrimSpace(line) == terminator {
				found = append(found, *current)
				current = nil
			} else {
				current.lines = append(current.lines, heredocLine{num: num, text: line})
			}
			continue
		}
		if m := opener.FindStringSubmatch(line); m != nil {
			current = &nestedHeredoc{openedAt: num}
			terminator = m[1]
		}
	}
	return found
}

// every other backend labels its overlay through a flag the exit-code matrix
// already runs (tmux -T, kitty --title); iTerm2 names the session it splits from
// an AppleScript argv, which nothing else in the suite reads
func TestIterm2OverlayNamesSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell launchers are not used on windows")
	}

	root := testRepoRoot(t)
	planFile := filepath.Join(t.TempDir(), "plan.md")
	writeTestFile(t, planFile, "# Plan\n")
	diffTitle := "rd: " + filepath.Base(root) + " [HEAD~1]"

	launchers := []struct {
		name  string
		path  string
		args  []string
		title string
	}{
		{
			name:  "claude",
			path:  ".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh",
			args:  []string{"HEAD~1"},
			title: diffTitle,
		},
		{
			name:  "codex",
			path:  "plugins/codex/skills/revdiff/scripts/launch-revdiff.sh",
			args:  []string{"HEAD~1"},
			title: diffTitle,
		},
		{
			name:  "plan review",
			path:  "plugins/revdiff-planning/scripts/launch-plan-review.sh",
			args:  []string{planFile},
			title: "plan: plan.md",
		},
	}

	output := "## file.go:1 (+)\ncomment\n"
	backend := launcherBackend{name: "iterm2", command: "osascript", env: map[string]string{"ITERM_SESSION_ID": "w0t0p0:ABC"}}
	for _, launcher := range launchers {
		t.Run(launcher.name, func(t *testing.T) {
			env := fakeLauncherEnv(t, launcherRun{backend: backend, code: exitCodeAnnotations, output: output})
			argsFile := filepath.Join(env["TMPDIR"], "osascript-args")
			env["FAKE_OSASCRIPT_ARGS_FILE"] = argsFile

			res := runTestCmd(t, cmdReq{
				dir:  root,
				name: "bash",
				args: append([]string{filepath.Join(root, launcher.path)}, launcher.args...),
				env:  env,
			})
			assert.Equal(t, exitCodeAnnotations, res.code)
			assert.Equal(t, output, res.stdout)

			raw, err := os.ReadFile(argsFile) //nolint:gosec // path is a test-owned temp file
			require.NoError(t, err)
			calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
			require.NotEmpty(t, calls)
			// the title is the last argv item, so the launch script stays at position 3 and the
			// stub's -x "$3" test still matches. the two launch-revdiff.sh calls go 5 -> 6 args and
			// stay in the >=5 branch where the trailing title is dropped; launch-plan-review.sh goes
			// 4 -> 5 and crosses into that branch, so the stub hands its launch script the title as
			// $2 while production passes the sentinel alone — that heredoc must keep reading $1 only
			assert.True(t, strings.HasSuffix(calls[0], " "+launcher.title),
				"split osascript argv should end with the overlay title, got %q", calls[0])
		})
	}
}

func TestAgtermPaneOverlayOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell launchers are not used on windows")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("the launcher's split check needs jq")
	}

	root := testRepoRoot(t)
	launchers := []struct {
		name string
		path string
	}{
		{name: "claude", path: ".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh"},
		{name: "codex", path: "plugins/codex/skills/revdiff/scripts/launch-revdiff.sh"},
	}
	cases := []struct {
		name      string
		env       map[string]string
		wantPane  bool
		wantOpens int
	}{
		{
			name:      "opt-in in a split scopes the overlay to the pane",
			env:       map[string]string{"REVDIFF_AGTERM_PANE": "1", "FAKE_AGTERM_PANE": "1", "FAKE_AGTERM_SPLIT": "true"},
			wantPane:  true,
			wantOpens: 1,
		},
		{
			name:      "without the opt-in the overlay stays session-wide",
			env:       map[string]string{"FAKE_AGTERM_PANE": "1", "FAKE_AGTERM_SPLIT": "true"},
			wantOpens: 1,
		},
		{
			name:      "opt-in without a split stays session-wide",
			env:       map[string]string{"REVDIFF_AGTERM_PANE": "1", "FAKE_AGTERM_PANE": "1", "FAKE_AGTERM_SPLIT": "false"},
			wantOpens: 1,
		},
		{
			name:      "opt-in on an older cli stays session-wide",
			env:       map[string]string{"REVDIFF_AGTERM_PANE": "1", "FAKE_AGTERM_SPLIT": "true"},
			wantOpens: 1,
		},
		{
			name: "a refused pane overlay retries session-wide",
			env: map[string]string{"REVDIFF_AGTERM_PANE": "1", "FAKE_AGTERM_PANE": "1", "FAKE_AGTERM_SPLIT": "true",
				"FAKE_AGTERM_PANE_REFUSE": "1"},
			wantPane:  true,
			wantOpens: 2,
		},
	}

	output := "## file.go:1 (+)\ncomment\n"
	for _, launcher := range launchers {
		for _, tc := range cases {
			t.Run(launcher.name+"/"+tc.name, func(t *testing.T) {
				backend := launcherBackend{name: "agterm", command: "agtermctl", env: map[string]string{
					"AGTERM_SESSION_ID": "sess-1",
					"AGTERM_WINDOW_ID":  "win-1",
					"AGTERM_PANE":       "left",
				}}
				env := fakeLauncherEnv(t, launcherRun{backend: backend, code: exitCodeAnnotations, output: output})
				argsFile := filepath.Join(env["TMPDIR"], "agterm-args")
				env["FAKE_AGTERM_ARGS_FILE"] = argsFile
				maps.Copy(env, tc.env)

				res := runTestCmd(t, cmdReq{
					dir:  root,
					name: "bash",
					args: []string{filepath.Join(root, launcher.path)},
					env:  env,
				})
				assert.Equal(t, exitCodeAnnotations, res.code)
				assert.Equal(t, output, res.stdout)

				raw, err := os.ReadFile(argsFile) //nolint:gosec // path is a test-owned temp file
				require.NoError(t, err)
				opens := strings.Split(strings.TrimSpace(string(raw)), "\n")
				require.Len(t, opens, tc.wantOpens)
				if tc.wantPane {
					assert.Contains(t, opens[0], "--pane left")
				} else {
					assert.NotContains(t, opens[0], "--pane")
				}
				if len(opens) > 1 {
					// the retry drops the pane scoping, otherwise agterm refuses it again
					assert.NotContains(t, opens[len(opens)-1], "--pane")
				}
			})
		}
	}
}

// TestHerdrSignalPaneOwnership pins both halves of the EXIT-trap contract by actually
// signaling the launcher. While the pane provably holds nothing but a shell -- created, but
// not yet reached by a dispatch -- a killed launcher must close it, or a zoomed shell pane
// is stranded over the agent's own. From the dispatch onward the state is no longer provable
// and the review may be the user's, so it must survive us, per SKILL.md. The pre-dispatch
// cases -- wedged in the split, interrupted in the split, wedged in the zoom -- are the ones
// that reach the trap's close; the tab fallthrough closes nothing, and every case from the
// dispatch onward is the preserve half.
func TestHerdrSignalPaneOwnership(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell launchers are not used on windows")
	}
	root := testRepoRoot(t)

	launchers := map[string]string{
		"claude": filepath.Join(".claude-plugin", "skills", "revdiff", "scripts", "launch-revdiff.sh"),
		"codex":  filepath.Join("plugins", "codex", "skills", "revdiff", "scripts", "launch-revdiff.sh"),
	}
	tests := []struct {
		name     string
		extraEnv map[string]string
		// the fake logs every call BEFORE acting on it, so `pane run` marks "pane exists,
		// not yet started". For the live case a logged call is not enough: ownership is
		// evidence-based now, so the test must see the pane's own start marker on disk.
		waitFor      string // a recorded herdr call
		waitForStart bool   // the launch script's .started marker, sentinel still absent
		// the hang/delay only has to outlast the signal, which lands in milliseconds. Kept
		// short because killing the launcher orphans the fake's sleep.
		wantClose bool
		sig       syscall.Signal // defaults to SIGTERM
		wantCode  int            // 0 skips the check

	}{
		{
			// the split is declined, so the launcher falls through to the tab overlay with
			// no pane to protect. The recording trap must not survive into that path: the
			// bounded wait above is what catches it if it does.
			name: "killed on the tab fallthrough still exits",
			extraEnv: map[string]string{
				"FAKE_HERDR_SPLIT_FAIL": "1", "FAKE_HERDR_NO_SENTINEL": "1",
			},
			waitFor:   "tab create",
			wantClose: false,
		},
		{
			// wedged in `pane split` itself: the pane comes into existence during the call,
			// so a signal deferred across it would otherwise be paid before HERDR_TARGET
			// names the pane, leaving the EXIT trap nothing to close. The launcher records
			// the signal instead and honors it once ownership is held.
			name:      "killed during the split closes the pane it created",
			extraEnv:  map[string]string{"FAKE_HERDR_SPLIT_HANG": "3"},
			waitFor:   "pane split --pane",
			wantClose: true,
		},
		{
			// same window, but SIGINT: the recording trap keeps each signal's own status,
			// so this must exit 130. A boolean flag paid as a fixed `exit 143` reports the
			// wrong signal here while still closing the pane, which no close assertion sees.
			name:      "an interrupt during the split reports its own status",
			extraEnv:  map[string]string{"FAKE_HERDR_SPLIT_HANG": "3"},
			waitFor:   "pane split --pane",
			sig:       syscall.SIGINT,
			wantCode:  130,
			wantClose: true,
		},
		{
			// wedged in `pane zoom`, which runs after the split and before any dispatch.
			// Nothing can be running in that pane yet, so this is the one state where the
			// trap has to destroy what it created -- the half no other case reaches, and
			// the reason master carried a textual guard over the trap's close.
			name:      "killed before any dispatch closes the pane",
			extraEnv:  map[string]string{"FAKE_HERDR_ZOOM_HANG": "3"},
			waitFor:   "pane zoom " + fakeHerdrPaneID,
			wantClose: true,
		},
		{
			// herdr is wedged inside `pane run`: it may or may not have delivered the
			// command, so the state is unknown and must not be destroyed. Ownership is
			// claimed before the call precisely so this window is preserved.
			name:      "killed while herdr is wedged preserves the pane",
			extraEnv:  map[string]string{"FAKE_HERDR_RUN_HANG": "3"},
			waitFor:   "pane run " + fakeHerdrPaneID,
			wantClose: false,
		},
		{
			// dispatch was accepted but the pane's shell has not touched the marker yet.
			// Unknown state must not be destroyed: this is the window HERDR_DISPATCHED
			// covers, and requiring the marker alone would close a review that may exist.
			name:      "killed after dispatch but before the marker leaves the pane open",
			extraEnv:  map[string]string{"FAKE_HERDR_NO_SENTINEL": "1"},
			waitFor:   "pane run " + fakeHerdrPaneID,
			wantClose: false,
		},
		{
			// `pane run` returns, so the review is live and must outlive us
			name:         "killed during a live review leaves the pane open",
			extraEnv:     map[string]string{"FAKE_HERDR_ASYNC": "1", "FAKE_REVDIFF_DELAY": "5"},
			waitForStart: true,
			wantClose:    false,
		},
	}

	for lname, lpath := range launchers {
		for _, tc := range tests {
			t.Run(lname+"/"+tc.name, func(t *testing.T) {
				backend := launcherBackend{name: "herdr", command: "herdr", env: map[string]string{"HERDR_ENV": "1"}}
				env := fakeLauncherEnv(t, launcherRun{backend: backend, code: exitCodeAnnotations, output: "x\n"})
				argsFile := filepath.Join(env["TMPDIR"], "herdr-args")
				env["FAKE_HERDR_ARGS_FILE"] = argsFile
				env["HERDR_PANE_ID"] = fakeHerdrCallerPaneID
				env["REVDIFF_HERDR_PANE"] = "1"
				env["FAKE_HERDR_PANE"] = "1"
				maps.Copy(env, tc.extraEnv)

				cmd := exec.Command("bash", filepath.Join(root, lpath)) //nolint:gosec // fixed repo path
				cmd.Dir, cmd.Env = root, mergeEnv(env)
				require.NoError(t, cmd.Start())

				require.Eventually(t, func() bool {
					if tc.waitForStart {
						// the review is genuinely mid-flight: started, not yet finished
						started, _ := filepath.Glob(filepath.Join(env["TMPDIR"], "revdiff-done-*.started"))
						done, _ := filepath.Glob(filepath.Join(env["TMPDIR"], "revdiff-done-??????"))
						return len(started) > 0 && len(done) == 0
					}
					raw, err := os.ReadFile(argsFile) //nolint:gosec // test-owned temp file
					return err == nil && strings.Contains(string(raw), tc.waitFor)
				}, 15*time.Second, 20*time.Millisecond, "launcher never reached the phase under test")
				sig := tc.sig
				if sig == 0 {
					sig = syscall.SIGTERM
				}
				require.NoError(t, cmd.Process.Signal(sig))
				// bounded on purpose: the launcher records signals across the windows where
				// it owns an unnamed pane, so a restore it forgets would swallow the SIGTERM
				// and hang here rather than fail. The longest fake hang is 3s.
				done := make(chan struct{})
				var waitErr error
				go func() { waitErr = cmd.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					_ = cmd.Process.Kill()
					t.Fatal("launcher did not exit after SIGTERM")
				}

				if tc.wantCode != 0 {
					// the recording trap stores the signal's own status, so a recorded INT
					// must still report 130 and not the TERM value it is paid alongside
					assert.Equal(t, tc.wantCode, commandExitCode(waitErr), "exit status after %v", sig)
				}

				raw, err := os.ReadFile(argsFile) //nolint:gosec // test-owned temp file
				require.NoError(t, err)
				calls := strings.Split(strings.TrimSpace(string(raw)), "\n")
				assert.Equal(t, tc.wantClose, countHerdrCalls(calls, "pane close "+fakeHerdrPaneID) > 0,
					"pane close after %v; calls=%v", sig, calls)
			})
		}
	}
}

// ids the fake herdr backend hands out; the caller pane is what the test injects as
// $HERDR_PANE_ID, so asserting against these pins who each call actually targets
const (
	fakeHerdrCallerPaneID  = "w1:p0" // the agent's own pane, must never be operated on
	fakeHerdrPaneID        = "w1-2"  // returned by `pane split`
	fakeHerdrTabRootPaneID = "w1-1"  // returned by `tab create`
)

// counts recorded calls matching prefix on a TOKEN boundary, so "pane close w1-2" is not
// satisfied by a call that targeted w1-20. A bare strings.HasPrefix would match it.
func countHerdrCalls(calls []string, prefix string) int {
	n := 0
	for _, c := range calls {
		if strings.Contains(c, "--help") {
			continue
		}
		if c == prefix || strings.HasPrefix(c, prefix+" ") {
			n++
		}
	}
	return n
}

// TestHerdrPaneOverlayOptIn covers REVDIFF_HERDR_PANE=1, which splits the caller's own
// herdr pane instead of opening a fullscreen tab. The failure paths carry the weight:
// with the synchronous fake the sentinel exists before the wait loop starts, so the
// liveness probe is only observable in the deferred-sentinel cases.
func TestHerdrPaneOverlayOptIn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell launchers are not used on windows")
	}

	root := testRepoRoot(t)
	launchers := []struct {
		name string
		path string
	}{
		{name: "claude", path: ".claude-plugin/skills/revdiff/scripts/launch-revdiff.sh"},
		{name: "codex", path: "plugins/codex/skills/revdiff/scripts/launch-revdiff.sh"},
	}

	cases := []struct {
		name string
		env  map[string]string
		// wantCode defaults to exitCodeAnnotations when zero
		wantCode       int
		wantTabCreate  bool
		wantSplit      bool // a non---help `pane split` was attempted
		wantClose      bool
		wantZoom       bool // the split must be zoomed, not just created
		wantGet        bool
		wantRetriedGet bool
		// upper bound on `pane get` calls, to pin the poll backoff. Near-fail-safe: a
		// loaded runner makes FEWER polls per second, so slowness alone lowers the count.
		// It could only flake the other way if the fake's sentinel timer were delayed
		// disproportionately more than the poll loop -- ~4s of one-sided skew.
		maxGets    int
		wantStderr string
		notStderr  string
		// a preserved live review keeps its launch script on purpose: the pane still has
		// to execute it, so the usual no-temp-files-left assertion does not apply
		keepsScript bool
		// lower bound on the launcher's own runtime. Used to pin that the refusal grace was
		// actually served: a close proves nothing on its own, since a launcher that skipped
		// the wait closes too. Only a lower bound is safe -- a loaded runner can only make
		// the run longer, never shorter.
		minDuration time.Duration
	}{
		{
			name:          "without the opt-in the review opens a tab",
			env:           map[string]string{"FAKE_HERDR_PANE": "1"},
			wantTabCreate: true,
		},
		{
			name:      "opt-in splits the caller pane and zooms the review pane",
			env:       map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1"},
			wantSplit: true,
			wantZoom:  true,
			wantClose: true,
		},
		{
			name:          "opt-in on an older cli falls back to a tab",
			env:           map[string]string{"REVDIFF_HERDR_PANE": "1"},
			wantTabCreate: true,
		},
		{
			// both vars are required: with SPLIT_FAIL alone the probe gate would divert
			// this down the older-cli path and no split would ever be attempted
			name:          "a refused split falls back to a tab",
			env:           map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1", "FAKE_HERDR_SPLIT_FAIL": "1"},
			wantSplit:     true,
			wantTabCreate: true,
		},
		{
			name:      "a deferred sentinel exercises the liveness probe",
			env:       map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1", "FAKE_HERDR_ASYNC": "1", "FAKE_REVDIFF_DELAY": "0.5"},
			wantSplit: true,
			wantZoom:  true,
			wantGet:   true,
			wantClose: true,
		},
		{
			// pane_not_found is authoritative: stop at once, do not close a pane that is
			// provably gone, and do not complain about failing to
			name: "a confirmed dead pane stops without closing",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_NO_SENTINEL": "1", "FAKE_HERDR_GET_GONE": "1"},
			wantCode:  1,
			wantSplit: true,
			wantZoom:  true,
			wantGet:   true,
			notStderr: "could not close",
		},
		{
			// a generic error is not evidence the pane died, and there is no safe deadline
			// when the API cannot report liveness: the loop must keep waiting through a
			// control-plane outage and still finish the review normally. Sustained failures
			// outlast the warn threshold here, then the sentinel lands. A 0.3s poll puts the
			// tenth miss at ~3.2s against a 5s sentinel, so the warning lands with roughly
			// 1.5x headroom; iterations slower than ~0.5s would flake it (fail, never
			// false-pass). After the warn the poll backs off to 2s, so the run totals ~11
			// gets, not the ~16 a flat poll would make.
			name: "a control-plane outage does not end a live review",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_GET_FAIL": "1", "FAKE_HERDR_ASYNC": "1", "FAKE_REVDIFF_DELAY": "5"},
			wantSplit:      true,
			wantZoom:       true,
			wantGet:        true,
			wantClose:      true,
			wantRetriedGet: true,
			// ~11 with the 2s backoff after miss 10; a flat 0.3s poll would make ~16
			maxGets:    13,
			wantStderr: "still waiting for the review",
		},
		{
			// the only state where nothing was dispatched: herdr refused and the pane never
			// announced itself, so the pane it created is ours to clean up
			name: "a refused pane run closes the pane it created",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_RUN_FAIL": "1"},
			wantCode:  1,
			wantSplit: true,
			wantZoom:  true,
			wantClose: true,
		},
		{
			// the refusal grace, signaled deterministically: the injected `sleep` raises
			// the signal before sleeping, so it is already pending when the grace sleep
			// becomes the foreground command. Launcher-directed, so the grace sleep itself
			// survives -- this pins the designed behavior, that a signal mid-grace still
			// completes the evidence check and closes an unannounced pane.
			name: "a signal during the refusal grace still closes the pane",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_RUN_FAIL": "1", "FAKE_SLEEP_KILL": "parent"},
			wantCode:  1,
			wantSplit: true,
			wantZoom:  true,
			wantClose: true,
		},
		{
			// same, but the signal goes to the process group, so it kills the grace `sleep`
			// too. Without the retry the killed sleep returns nonzero, `set -e` aborts
			// before the evidence check, and the trap's preserve rule strands the pane.
			name: "a process-group signal during the grace still closes the pane",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_RUN_FAIL": "1", "FAKE_SLEEP_KILL": "group"},
			wantCode:  1,
			wantSplit: true,
			wantZoom:  true,
			wantClose: true,
		},
		{
			// every sleep in the grace is killed, so nothing is ever waited out by counting
			// sleeps -- but the grace is measured on the wall clock, so the interval is
			// served regardless and the absent marker is still real evidence. Pins that no
			// amount of signaling can talk the launcher out of serving the grace.
			name: "a relentlessly signaled grace is still served",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_RUN_FAIL": "1", "FAKE_SLEEP_KILL": "group-always"},
			wantCode:  1,
			wantSplit: true,
			wantZoom:  true,
			wantClose: true,
			// the close alone would also pass against a launcher that never waited, so the
			// duration is the assertion that matters here
			minDuration: 900 * time.Millisecond,
		},
		{
			// a nonzero `pane run` is not evidence the review did not start -- the response
			// can be lost after delivery. The launcher must not close a pane that already
			// announced itself, the same rule the wait loop applies to `pane get`.
			name: "a lost pane run response does not close a started review",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_RUN_FAIL_AFTER_START": "1", "FAKE_REVDIFF_DELAY": "3"},
			wantCode:    1,
			wantSplit:   true,
			wantZoom:    true,
			keepsScript: true,
		},
		{
			// the worst failure this feature could have: adopting the caller's own id and
			// closing the agent's terminal. The grep fallback is positional, so a response
			// naming the source pane first must be refused.
			name: "a split that returns the caller's own pane is refused",
			env: map[string]string{"REVDIFF_HERDR_PANE": "1", "FAKE_HERDR_PANE": "1",
				"FAKE_HERDR_SPLIT_JSON": `{"result":{"pane":{"pane_id":"` + fakeHerdrCallerPaneID + `"}}}`},
			wantCode:   1,
			wantSplit:  true,
			wantStderr: "stray review pane may remain",
		},
	}

	output := "## file.go:1 (+)\ncomment\n"
	for _, launcher := range launchers {
		for _, tc := range cases {
			t.Run(launcher.name+"/"+tc.name, func(t *testing.T) {
				backend := launcherBackend{name: "herdr", command: "herdr", env: map[string]string{
					"HERDR_ENV": "1",
				}}
				env := fakeLauncherEnv(t, launcherRun{backend: backend, code: exitCodeAnnotations, output: output})
				argsFile := filepath.Join(env["TMPDIR"], "herdr-args")
				env["FAKE_HERDR_ARGS_FILE"] = argsFile
				// the pane gate needs a caller pane id; cleanOverlayEnv blanks it, so
				// without this every opt-in case would silently degrade to tab mode
				env["HERDR_PANE_ID"] = fakeHerdrCallerPaneID
				maps.Copy(env, tc.env)

				started := time.Now()
				res := runTestCmd(t, cmdReq{
					dir:  root,
					name: "bash",
					args: []string{filepath.Join(root, launcher.path)},
					env:  env,
					// the group-kill case signals its own process group; without this the
					// signal reaches the test binary instead of the launcher
					setpgid: strings.HasPrefix(tc.env["FAKE_SLEEP_KILL"], "group"),
				})
				if tc.minDuration > 0 {
					assert.GreaterOrEqual(t, time.Since(started), tc.minDuration,
						"the refusal grace was not served")
				}

				wantCode := tc.wantCode
				if wantCode == 0 {
					wantCode = exitCodeAnnotations
				}
				assert.Equal(t, wantCode, res.code)
				if wantCode == exitCodeAnnotations {
					assert.Equal(t, output, res.stdout)
				}
				if tc.wantStderr != "" {
					assert.Contains(t, res.stderr, tc.wantStderr)
				}
				if tc.notStderr != "" {
					assert.NotContains(t, res.stderr, tc.notStderr)
				}

				raw, err := os.ReadFile(argsFile) //nolint:gosec // path is a test-owned temp file
				require.NoError(t, err)
				calls := strings.Split(strings.TrimSpace(string(raw)), "\n")

				closes, gets := countHerdrCalls(calls, "pane close"), countHerdrCalls(calls, "pane get")
				assert.Equal(t, tc.wantSplit, countHerdrCalls(calls, "pane split") > 0, "pane split; calls=%v", calls)
				assert.Equal(t, tc.wantTabCreate, countHerdrCalls(calls, "tab create") > 0, "tab create; calls=%v", calls)
				assert.Equal(t, tc.wantClose, closes > 0, "pane close; calls=%v", calls)
				assert.Equal(t, tc.wantGet, gets > 0, "pane get; calls=%v", calls)

				// teardown is idempotent: the inline call and the EXIT trap's call must not
				// both close, and a boolean cannot see a double close
				assert.LessOrEqual(t, closes, 1, "pane closed more than once; calls=%v", calls)
				if tc.wantTabCreate {
					assert.Equal(t, 1, countHerdrCalls(calls, "tab close"), "tab create needs exactly one close; calls=%v", calls)
				}
				if tc.maxGets > 0 {
					assert.LessOrEqual(t, gets, tc.maxGets,
						"poll did not back off during the outage; calls=%v", calls)
				}
				if tc.wantRetriedGet {
					// the transient path must retry, not give up on the first miss
					assert.Greater(t, gets, 1, "transient failures must be retried; calls=%v", calls)
				}
				if tc.wantSplit {
					// anchored to the caller, or herdr splits whichever pane is focused
					assert.Equal(t, 1, countHerdrCalls(calls, "pane split --pane "+fakeHerdrCallerPaneID), "split must anchor to the caller; calls=%v", calls)
				}
				if tc.wantZoom {
					assert.Equal(t, 1, countHerdrCalls(calls, "pane zoom "+fakeHerdrPaneID+" --on"), "the split must be zoomed; calls=%v", calls)
				}
				if closes > 0 {
					// the review pane herdr returned, never the agent's own
					assert.Equal(t, 1, countHerdrCalls(calls, "pane close "+fakeHerdrPaneID), "close must target the returned pane; calls=%v", calls)
				}
				// never-guess: the launcher must not enumerate panes to find one
				assert.Zero(t, countHerdrCalls(calls, "pane list"), "must not enumerate panes; calls=%v", calls)

				assert.Empty(t, leftoverStderrCaptures(t, env["TMPDIR"]))
				// a launcher that ran to completion owns no temp files: the dispatched script
				// removes itself, and the completion path removes it for a pane that died
				if !tc.keepsScript {
					left, err := filepath.Glob(filepath.Join(env["TMPDIR"], "revdiff-launch-*"))
					require.NoError(t, err)
					assert.Empty(t, left, "leaked launch script; calls=%v", calls)
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
	if r.setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
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
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
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
	// A `sleep` that signals before it sleeps. Used to reach the refusal path's 300ms
	// evidence grace deterministically: the signal is already pending when the grace sleep
	// becomes the foreground command, so there is no window to race. FAKE_SLEEP_KILL picks
	// the target -- "parent" is a launcher-directed signal, "group" emulates Ctrl-C, which
	// kills this sleep too, and "group-always" keeps doing it so every retry dies.
	//
	// The once-marker makes "group" hit only the FIRST sleep of the run, which on the
	// refusal path is the grace. That assumption is load-bearing: combining FAKE_SLEEP_KILL
	// with anything that sleeps earlier (a backend with its own wait loop, or
	// FAKE_REVDIFF_DELAY) spends the marker on the wrong sleep and the case passes while
	// testing nothing. Keep these cases on the refusal path.
	//
	// The marker is written BEFORE the kill so the retry that follows runs a real sleep.
	writeExecutable(t, filepath.Join(binDir, "sleep"), `#!/bin/sh
if [ -n "${FAKE_SLEEP_KILL:-}" ] &&
    { [ "$FAKE_SLEEP_KILL" = group-always ] || [ ! -f "$TMPDIR/sleep-killed" ]; }; then
    : > "$TMPDIR/sleep-killed"
    case "$FAKE_SLEEP_KILL" in
        group | group-always) kill -TERM 0 ;;
        *) kill -TERM "$PPID" ;;
    esac
fi
exec /bin/sleep "$@"
`)

	env := cleanOverlayEnv()
	maps.Copy(env, r.backend.env)
	env["FAKE_OUTPUT"] = r.output
	env["FAKE_STDERR"] = r.stderr
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
		"HERDR_WORKSPACE_ID":    "",
		"REVDIFF_HERDR_PANE":    "",
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
		"AGTERM_PANE":           "",
		"AGTERM_WINDOW_ID":      "",
		"REVDIFF_AGTERM_PANE":   "",
		"REVDIFF_TMUX_WINDOW":   "",
		"AGENTDECK_INSTANCE_ID": "",
		"REVDIFF_CONFIG":        "",
	}
}

func resolverScript(launcher string) string {
	return "#!/bin/sh\nprintf '%s\\n' " + shQuote(launcher) + "\n"
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func leftoverStderrCaptures(t *testing.T, dir string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "revdiff-err-*"))
	require.NoError(t, err)
	return matches
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
