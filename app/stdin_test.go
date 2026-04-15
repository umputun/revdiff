package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
