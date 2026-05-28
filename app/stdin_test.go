package main

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
)

type fakeFileInfo struct {
	mode os.FileMode
}

func (f fakeFileInfo) Name() string       { return "stdin" }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return f.mode }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

type fakeStdin struct {
	info os.FileInfo
	err  error
}

func (f fakeStdin) Stat() (os.FileInfo, error) {
	return f.info, f.err
}

func TestStdinName(t *testing.T) {
	assert.Equal(t, "scratch-buffer", stdinName(""), "empty name should use default")
	assert.Equal(t, "plan.md", stdinName("plan.md"), "explicit name should pass through")
}

func TestValidateStdinInput(t *testing.T) {
	t.Run("non tty stdin succeeds", func(t *testing.T) {
		err := validateStdinInput(options{Stdin: true}, fakeStdin{info: fakeFileInfo{mode: 0}})
		require.NoError(t, err)
	})

	t.Run("tty stdin errors", func(t *testing.T) {
		err := validateStdinInput(options{Stdin: true}, fakeStdin{info: fakeFileInfo{mode: os.ModeCharDevice}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--stdin requires piped or redirected input")
	})

	t.Run("stat error propagates", func(t *testing.T) {
		err := validateStdinInput(options{Stdin: true}, fakeStdin{err: errors.New("device gone")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "stat stdin")
		assert.Contains(t, err.Error(), "device gone")
	})
}

func TestReadStdinCapped(t *testing.T) {
	t.Run("under cap succeeds", func(t *testing.T) {
		got, err := readStdinCapped(strings.NewReader("hello\n"))
		require.NoError(t, err)
		assert.Equal(t, "hello\n", got)
	})

	t.Run("over cap rejected with clear error", func(t *testing.T) {
		// produce maxStdinSize+1 bytes without holding two copies
		r := io.LimitReader(zeroReader{}, maxStdinSize+1)
		_, err := readStdinCapped(r)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds")
	})
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	return len(p), nil
}

func TestSelectStdinRenderer(t *testing.T) {
	multiFile := `diff --git a/file1.go b/file1.go
index abc..def
--- a/file1.go
+++ b/file1.go
@@ -1,1 +1,2 @@
 line1
+line2

diff --git a/file2.go b/file2.go
index ghi..jkl
--- a/file2.go
+++ b/file2.go
@@ -1,1 +1,1 @@
-old
+new
`

	t.Run("multi-file diff selects MultiFileStdinReader", func(t *testing.T) {
		r, err := selectStdinRenderer(options{Stdin: true}, multiFile)
		require.NoError(t, err)
		_, ok := r.(*diff.MultiFileStdinReader)
		assert.True(t, ok, "expected *MultiFileStdinReader, got %T", r)
	})

	t.Run("plain text falls back to StdinReader silently", func(t *testing.T) {
		r, err := selectStdinRenderer(options{Stdin: true, StdinName: "note.txt"}, "just plain text\nno diff markers\n")
		require.NoError(t, err)
		_, ok := r.(*diff.StdinReader)
		assert.True(t, ok, "expected *StdinReader, got %T", r)
	})

	t.Run("sniff true but parse fails falls back to raw text", func(t *testing.T) {
		// sniff matches "diff --git a/", parseUnifiedDiff fails on malformed hunk header.
		// caller must still get a working raw-text renderer (locks in #1's fallback contract).
		bad := `diff --git a/bad.go b/bad.go
index abc..def
--- a/bad.go
+++ b/bad.go
@@ -99999999999999999999999,1 +1,1 @@
-old
+new
`
		r, err := selectStdinRenderer(options{Stdin: true, StdinName: "bad.diff"}, bad)
		require.NoError(t, err)
		_, ok := r.(*diff.StdinReader)
		assert.True(t, ok, "expected *StdinReader fallback, got %T", r)
	})

	t.Run("markdown with diff snippet inside prose stays raw text", func(t *testing.T) {
		// the marker appears mid-line inside prose; sniffer must NOT classify this
		// as a diff or surrounding prose would be dropped from rendering.
		md := "# Title\n\nSome prose mentioning `diff --git a/x b/x` and `@@ -1,1 +1,1 @@` markers.\n\nMore prose.\n"
		r, err := selectStdinRenderer(options{Stdin: true, StdinName: "doc.md"}, md)
		require.NoError(t, err)
		_, ok := r.(*diff.StdinReader)
		assert.True(t, ok, "expected *StdinReader, got %T", r)
	})
}
