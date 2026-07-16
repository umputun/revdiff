package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

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
