package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type markerWriter struct {
	bytes.Buffer
	marker string
	ready  chan struct{}
	found  bool
}

func (w *markerWriter) Write(p []byte) (int, error) {
	n, _ := w.Buffer.Write(p)
	if !w.found && bytes.Contains(w.Bytes(), []byte(w.marker)) {
		w.found = true
		close(w.ready)
	}
	return n, nil
}

func TestAnnotationExitCode(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		output  string
		want    int
	}{
		{name: "no output with flag disabled", enabled: false, output: "", want: 0},
		{name: "no output with flag enabled", enabled: true, output: "", want: 0},
		{name: "output with flag disabled", enabled: false, output: "## file.go:1 (+)\ncomment\n", want: 0},
		{name: "output with flag enabled", enabled: true, output: "## file.go:1 (+)\ncomment\n", want: exitCodeAnnotations},
		{name: "discarded empty output", enabled: true, output: "", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, annotationExitCode(tt.enabled, tt.output))
		})
	}
}

func TestFinalize(t *testing.T) {
	output := "## file.go:1 (+)\ncomment\n"
	tests := []struct {
		name           string
		output         string
		discarded      bool
		signaled       bool
		withOutputFile bool
		wantHistory    bool
		wantHandoff    bool
	}{
		{name: "signaled saves history only", output: output, signaled: true, wantHistory: true, wantHandoff: false},
		{name: "signaled with output file writes no handoff", output: output, signaled: true, withOutputFile: true, wantHistory: true, wantHandoff: false},
		{name: "graceful with output file", output: output, withOutputFile: true, wantHistory: true, wantHandoff: true},
		{name: "graceful to stdout", output: output, wantHistory: true, wantHandoff: true},
		{name: "discarded writes nothing", output: output, discarded: true, wantHistory: false, wantHandoff: false},
		{name: "discarded during signal still writes nothing", output: output, discarded: true, signaled: true, wantHistory: false, wantHandoff: false},
		{name: "empty output writes nothing", output: "", wantHistory: false, wantHandoff: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			histDir := t.TempDir()
			opts := options{HistoryDir: histDir}
			var outFile string
			if tt.withOutputFile {
				outFile = filepath.Join(t.TempDir(), "annotations.txt")
				opts.Output = outFile
			}

			var buf bytes.Buffer
			code, err := finalize(finalizeReq{
				opts:        opts,
				annotations: tt.output,
				files:       []string{"file.go"},
				discarded:   tt.discarded,
				gitRoot:     "",
				workDir:     "repo",
				signaled:    tt.signaled,
				stdout:      &buf,
			})
			require.NoError(t, err)
			assert.Equal(t, 0, code)
			assert.Equal(t, tt.wantHistory, historyFileCount(t, histDir) > 0)

			if tt.withOutputFile {
				assert.Empty(t, buf.String())
				if tt.wantHandoff {
					got, rerr := os.ReadFile(outFile) //nolint:gosec // test reads a file under t.TempDir
					require.NoError(t, rerr)
					assert.Equal(t, tt.output, string(got))
				} else {
					assert.NoFileExists(t, outFile)
				}
				return
			}
			if tt.wantHandoff {
				assert.Equal(t, tt.output, buf.String())
				return
			}
			assert.Empty(t, buf.String())
		})
	}
}

func historyFileCount(t *testing.T, dir string) int {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "*", "*.md"))
	require.NoError(t, err)
	return len(matches)
}

func TestWriteAnnotationOutput(t *testing.T) {
	output := "## file.go:1 (+)\ncomment\n"
	t.Run("stdout default exit zero", func(t *testing.T) {
		var buf bytes.Buffer
		code, err := writeAnnotationOutput(annotationOutputReq{opts: options{}, output: output, stdout: &buf})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Equal(t, output, buf.String())
	})

	t.Run("stdout annotations exit code", func(t *testing.T) {
		var buf bytes.Buffer
		code, err := writeAnnotationOutput(annotationOutputReq{opts: options{ExitCodeOnAnnotations: true}, output: output, stdout: &buf})
		require.NoError(t, err)
		assert.Equal(t, exitCodeAnnotations, code)
		assert.Equal(t, output, buf.String())
	})

	t.Run("output file annotations exit code", func(t *testing.T) {
		dir := t.TempDir()
		outFile := filepath.Join(dir, "annotations.txt")

		code, err := writeAnnotationOutput(annotationOutputReq{
			opts:   options{ExitCodeOnAnnotations: true, Output: outFile},
			output: output,
			stdout: &bytes.Buffer{},
		})
		require.NoError(t, err)
		assert.Equal(t, exitCodeAnnotations, code)
		got, err := os.ReadFile(outFile) //nolint:gosec // test reads a file created under t.TempDir
		require.NoError(t, err)
		assert.Equal(t, output, string(got))

		// atomic write leaves no temp file behind in the target directory
		entries, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, entries, 1)
		assert.Equal(t, "annotations.txt", entries[0].Name())
	})

	t.Run("output file write error", func(t *testing.T) {
		badPath := filepath.Join(t.TempDir(), "missing", "annotations.txt")
		code, err := writeAnnotationOutput(annotationOutputReq{
			opts:   options{ExitCodeOnAnnotations: true, Output: badPath},
			output: output,
			stdout: &bytes.Buffer{},
		})
		assert.Equal(t, 0, code)
		require.Error(t, err)
		assert.ErrorContains(t, err, "write output")
	})

	t.Run("stdout write error", func(t *testing.T) {
		code, err := writeAnnotationOutput(annotationOutputReq{
			opts:   options{ExitCodeOnAnnotations: true},
			output: output,
			stdout: errWriter{},
		})
		assert.Equal(t, 0, code)
		require.Error(t, err)
		assert.ErrorContains(t, err, "write output")
	})

	t.Run("empty output keeps zero", func(t *testing.T) {
		var buf bytes.Buffer
		code, err := writeAnnotationOutput(annotationOutputReq{
			opts:   options{ExitCodeOnAnnotations: true},
			output: "",
			stdout: &buf,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, code)
		assert.Empty(t, buf.String())
	})
}

func TestTUIOutput_Open(t *testing.T) {
	stdout, err := os.CreateTemp(t.TempDir(), "stdout-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, stdout.Close()) })

	tty, err := os.CreateTemp(t.TempDir(), "tty-*")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, tty.Close()) })

	tests := []struct {
		name       string
		terminal   bool
		openErr    error
		wantOutput *os.File
		wantOpened bool
		wantErr    bool
	}{
		{name: "terminal stdout remains TUI output", terminal: true, wantOutput: stdout},
		{name: "redirected stdout uses tty", terminal: false, wantOutput: tty, wantOpened: true},
		{name: "redirected stdout requires tty", terminal: false, openErr: errors.New("no tty"), wantOpened: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			req := tuiOutput{
				stdout:     stdout,
				isTerminal: func(uintptr) bool { return tt.terminal },
				openTTY: func() (*os.File, error) {
					opened = true
					return tty, tt.openErr
				},
			}

			got, openErr := req.open()
			if tt.wantErr {
				require.Error(t, openErr)
				require.ErrorContains(t, openErr, "revdiff requires an interactive terminal")
			} else {
				require.NoError(t, openErr)
			}
			assert.Same(t, tt.wantOutput, got)
			assert.Equal(t, tt.wantOpened, opened)
		})
	}
}

func TestRun_RedirectedStdoutUsesTTY(t *testing.T) {
	if os.Getenv("REVDIFF_TUI_TEST_HELPER") == "1" {
		opts, err := parseArgs([]string{
			"--config=" + os.Getenv("REVDIFF_TUI_TEST_CONFIG"),
			"--annotations=" + os.Getenv("REVDIFF_TUI_TEST_ANNOTATIONS"),
			"--history-dir=" + os.Getenv("REVDIFF_TUI_TEST_HISTORY"),
			"--no-colors",
			"--no-mouse",
		})
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "parse helper args: %v\n", err)
			os.Exit(1)
		}
		code, runErr := run(opts)
		if runErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "run helper: %v\n", runErr)
			os.Exit(1)
		}
		os.Exit(code)
	}

	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("PTY integration is supported on darwin and linux")
	}
	scriptBin, err := exec.LookPath("script")
	if err != nil {
		t.Skip("script utility is unavailable")
	}
	mkfifoBin, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo utility is unavailable")
	}
	probePath := filepath.Join(t.TempDir(), "probe-tty.sh")
	probe := "#!/bin/sh\n( : > /dev/tty ) 2>/dev/null || exit 77\n"
	require.NoError(t, os.WriteFile(probePath, []byte(probe), 0o600))
	require.NoError(t, os.Chmod(probePath, 0o700)) //nolint:gosec // executable test fixture
	probeArgs := []string{"-q", "/dev/null", probePath}
	if runtime.GOOS == "linux" {
		probeArgs = []string{"-q", "-e", "-c", probePath, "/dev/null"}
	}
	probeCmd := exec.Command(scriptBin, probeArgs...) //nolint:gosec // fixed utility with test-controlled arguments
	probeCmd.Stdout, probeCmd.Stderr = io.Discard, io.Discard
	if probeErr := probeCmd.Run(); probeErr != nil {
		if probeCmd.ProcessState != nil && probeCmd.ProcessState.ExitCode() == 77 {
			t.Skip("sandbox denies access to the test PTY")
		}
		require.NoError(t, probeErr)
	}

	repo := t.TempDir()
	runMainTestGit(t, repo, "init", "-q")
	runMainTestGit(t, repo, "config", "user.email", "test@example.com")
	runMainTestGit(t, repo, "config", "user.name", "test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\ntwo\n"), 0o600))
	runMainTestGit(t, repo, "add", "a.txt")
	runMainTestGit(t, repo, "commit", "-qm", "initial")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\nchanged\n"), 0o600))

	const annotations = "## a.txt:2 (+)\npipe output stays clean\n"
	notesPath := filepath.Join(t.TempDir(), "annotations.md")
	require.NoError(t, os.WriteFile(notesPath, []byte(annotations), 0o600))
	capturePath := filepath.Join(t.TempDir(), "stdout.txt")
	wrapperPath := filepath.Join(t.TempDir(), "run-revdiff.sh")
	wrapper := "#!/bin/sh\n\"$REVDIFF_TEST_BINARY\" -test.run '^TestRun_RedirectedStdoutUsesTTY$' | cat > \"$REVDIFF_TEST_CAPTURE\"\n"
	require.NoError(t, os.WriteFile(wrapperPath, []byte(wrapper), 0o600))
	require.NoError(t, os.Chmod(wrapperPath, 0o700)) //nolint:gosec // executable test fixture
	transcriptPath := filepath.Join(t.TempDir(), "terminal.fifo")
	require.NoError(t, exec.Command(mkfifoBin, transcriptPath).Run()) //nolint:gosec // fixed utility with test-controlled path

	args := []string{"-q", "-F", transcriptPath, wrapperPath}
	if runtime.GOOS == "linux" {
		args = []string{"-q", "-e", "-f", "-c", wrapperPath, transcriptPath}
	}
	cmd := exec.Command(scriptBin, args...) //nolint:gosec // fixed utility with test-controlled arguments
	cmd.Dir = repo
	cmd.Env = mergeEnv(map[string]string{
		"REVDIFF_TUI_TEST_HELPER":      "1",
		"REVDIFF_TUI_TEST_CONFIG":      filepath.Join(t.TempDir(), "missing-config"),
		"REVDIFF_TUI_TEST_ANNOTATIONS": notesPath,
		"REVDIFF_TUI_TEST_HISTORY":     filepath.Join(t.TempDir(), "history"),
		"REVDIFF_TEST_BINARY":          os.Args[0],
		"REVDIFF_TEST_CAPTURE":         capturePath,
	})
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	terminal := &markerWriter{marker: "\x1b[?1049h", ready: make(chan struct{})}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.Discard, &stderr
	require.NoError(t, cmd.Start())
	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	proc := ptyTestProcess{cmd: cmd, waitDone: waitDone, stderr: &stderr}
	transcript := proc.openFIFO(t, transcriptPath)
	readDone := make(chan error, 1)
	go func() {
		// hide bytes.Buffer.ReadFrom so io.Copy calls markerWriter.Write and
		// closes ready when the terminal marker arrives.
		_, copyErr := io.Copy(struct{ io.Writer }{terminal}, transcript)
		readDone <- copyErr
	}()

	select {
	case <-terminal.ready:
	case waitErr := <-waitDone:
		readErr := <-readDone
		_ = transcript.Close()
		t.Fatalf("script exited before TUI output\nstdout: %q\nstderr: %s\nwait: %v\nread: %v",
			terminal.String(), stderr.String(), waitErr, readErr)
	case <-time.After(5 * time.Second):
		_, _ = io.WriteString(stdin, "q")
		_ = stdin.Close()
		waitErr := <-waitDone
		readErr := <-readDone
		_ = transcript.Close()
		t.Fatalf("TUI never rendered on the terminal\nstdout: %q\nstderr: %s\nwait: %v\nread: %v",
			terminal.String(), stderr.String(), waitErr, readErr)
	}
	_, err = io.WriteString(stdin, "q")
	require.NoError(t, err)
	require.NoError(t, stdin.Close())
	waitErr := <-waitDone
	readErr := <-readDone
	require.NoError(t, transcript.Close())
	require.NoError(t, waitErr, "terminal: %q\nstderr: %s", terminal.String(), stderr.String())
	require.NoError(t, readErr)

	captured, err := os.ReadFile(capturePath) //nolint:gosec // test-owned path
	require.NoError(t, err)
	assert.Equal(t, annotations, string(captured))
	assert.NotContains(t, string(captured), "\x1b[")
}

func TestHoldMainTestFIFOWriter_UnblocksLateReader(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("FIFO test is supported on darwin and linux")
	}
	mkfifoBin, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo utility is unavailable")
	}
	path := filepath.Join(t.TempDir(), "late-reader.fifo")
	require.NoError(t, exec.Command(mkfifoBin, path).Run()) //nolint:gosec // fixed utility with test-controlled path

	writer, err := holdMainTestFIFOWriter(path)
	require.NoError(t, err)
	readerDone := make(chan *os.File, 1)
	go func() {
		reader, _ := os.Open(path) //nolint:gosec // test-owned FIFO
		readerDone <- reader
	}()

	select {
	case reader := <-readerDone:
		require.NotNil(t, reader)
		require.NoError(t, reader.Close())
	case <-time.After(time.Second):
		t.Fatal("late FIFO reader stayed blocked after writer helper returned")
	}
	require.NoError(t, writer.Close())
}

func runMainTestGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // test-controlled arguments
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
}

type ptyTestProcess struct {
	cmd      *exec.Cmd
	waitDone chan error
	stderr   *bytes.Buffer
}

func (p ptyTestProcess) openFIFO(t *testing.T, path string) *os.File {
	t.Helper()
	type openResult struct {
		file *os.File
		err  error
	}
	openDone := make(chan openResult, 1)
	go func() {
		file, err := os.Open(path) //nolint:gosec // test-owned FIFO
		openDone <- openResult{file: file, err: err}
	}()

	select {
	case result := <-openDone:
		if result.err != nil {
			_ = p.cmd.Process.Kill()
			waitErr := <-p.waitDone
			t.Fatalf("open terminal transcript: %v; wait: %v; stderr: %s", result.err, waitErr, p.stderr.String())
		}
		return result.file
	case waitErr := <-p.waitDone:
		writer, writerErr := holdMainTestFIFOWriter(path)
		if writerErr != nil {
			t.Fatalf("open FIFO writer after script exit: %v; wait: %v; stderr: %s", writerErr, waitErr, p.stderr.String())
		}
		result := <-openDone
		_ = writer.Close()
		if result.file != nil {
			_ = result.file.Close()
		}
		t.Fatalf("script exited before opening the terminal transcript: %v; stderr: %s", waitErr, p.stderr.String())
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		waitErr := <-p.waitDone
		writer, writerErr := holdMainTestFIFOWriter(path)
		if writerErr != nil {
			t.Fatalf("open FIFO writer after timeout: %v; wait: %v; stderr: %s", writerErr, waitErr, p.stderr.String())
		}
		result := <-openDone
		_ = writer.Close()
		if result.file != nil {
			_ = result.file.Close()
		}
		t.Fatalf("timed out opening terminal transcript; wait: %v; stderr: %s", waitErr, p.stderr.String())
	}
	return nil
}

func holdMainTestFIFOWriter(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR, 0) //nolint:gosec // test-owned FIFO
	if err != nil {
		return nil, fmt.Errorf("open FIFO read-write: %w", err)
	}
	return file, nil
}
