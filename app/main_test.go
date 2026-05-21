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
		outFile := filepath.Join(t.TempDir(), "annotations.txt")
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
