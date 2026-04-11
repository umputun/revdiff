package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/app/diff"
	"github.com/umputun/revdiff/app/theme"
)

// noConfigArgs returns args that point to a nonexistent config file,
// isolating the test from user's real config.
// testThemeContent returns a valid theme file string with the given name.
func testThemeContent(name string) string {
	return "# name: " + name + "\n# description: test theme\n# author: Test\nchroma-style = monokai\n" +
		"color-accent = #bd93f9\ncolor-border = #6272a4\ncolor-normal = #f8f8f2\ncolor-muted = #6272a4\n" +
		"color-selected-fg = #f8f8f2\ncolor-selected-bg = #44475a\ncolor-annotation = #f1fa8c\n" +
		"color-cursor-fg = #282a36\ncolor-cursor-bg = #f8f8f2\ncolor-add-fg = #50fa7b\ncolor-add-bg = #2a4a2a\n" +
		"color-remove-fg = #ff5555\ncolor-remove-bg = #4a2a2a\ncolor-modify-fg = #ffb86c\ncolor-modify-bg = #3a3a2a\n" +
		"color-tree-bg = #21222c\ncolor-diff-bg = #282a36\ncolor-status-fg = #f8f8f2\ncolor-status-bg = #44475a\n" +
		"color-search-fg = #282a36\ncolor-search-bg = #f1fa8c\n"
}

func noConfigArgs(t *testing.T) []string {
	t.Helper()
	return []string{"--config", filepath.Join(t.TempDir(), "none")}
}

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

func TestParseArgs_Defaults(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, 2, opts.TreeWidth)
	assert.Equal(t, 4, opts.TabWidth)
	assert.Equal(t, "catppuccin-macchiato", opts.ChromaStyle)
	assert.False(t, opts.Staged)
	assert.False(t, opts.NoColors)
	assert.False(t, opts.NoStatusBar)
	assert.False(t, opts.NoConfirmDiscard)
	assert.False(t, opts.Wrap)
	assert.False(t, opts.Collapsed)
	assert.False(t, opts.CrossFileHunks)
	assert.False(t, opts.LineNumbers)
	assert.False(t, opts.Blame)
	assert.False(t, opts.Stdin)
	assert.Empty(t, opts.Output)
	assert.Empty(t, opts.StdinName)
	assert.Empty(t, opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
}

func TestParseArgs_NoConfirmDiscard(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--no-confirm-discard"))
		require.NoError(t, err)
		assert.True(t, opts.NoConfirmDiscard)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_NO_CONFIRM_DISCARD", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.NoConfirmDiscard)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nno-confirm-discard = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.NoConfirmDiscard)
	})
}

func TestParseArgs_Wrap(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--wrap"))
		require.NoError(t, err)
		assert.True(t, opts.Wrap)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_WRAP", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.Wrap)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nwrap = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.Wrap)
	})
}

func TestParseArgs_Collapsed(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--collapsed"))
		require.NoError(t, err)
		assert.True(t, opts.Collapsed)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_COLLAPSED", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.Collapsed)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\ncollapsed = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.Collapsed)
	})
}

func TestParseArgs_CrossFileHunks(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--cross-file-hunks"))
		require.NoError(t, err)
		assert.True(t, opts.CrossFileHunks)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_CROSS_FILE_HUNKS", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.CrossFileHunks)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\ncross-file-hunks = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.CrossFileHunks)
	})
}

func TestParseArgs_LineNumbers(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--line-numbers"))
		require.NoError(t, err)
		assert.True(t, opts.LineNumbers)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_LINE_NUMBERS", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.LineNumbers)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nline-numbers = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.LineNumbers)
	})
}

func TestParseArgs_Blame(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--blame"))
		require.NoError(t, err)
		assert.True(t, opts.Blame)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_BLAME", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.Blame)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nblame = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.Blame)
	})
}

func TestParseArgs_WordDiff(t *testing.T) {
	t.Run("default off", func(t *testing.T) {
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.False(t, opts.WordDiff)
	})

	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--word-diff"))
		require.NoError(t, err)
		assert.True(t, opts.WordDiff)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_WORD_DIFF", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.WordDiff)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nword-diff = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.WordDiff)
	})
}

func TestParseArgs_OutputFlag(t *testing.T) {
	opts, err := parseArgs([]string{"-o", "/tmp/out.txt"})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/out.txt", opts.Output)

	opts, err = parseArgs([]string{"--output=/tmp/out2.txt"})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/out2.txt", opts.Output)
}

func TestParseArgs_Flags(t *testing.T) {
	opts, err := parseArgs([]string{"--staged", "--tree-width=5", "--tab-width=8", "--no-colors", "--chroma-style=dracula", "HEAD~3"})
	require.NoError(t, err)
	assert.True(t, opts.Staged)
	assert.Equal(t, 5, opts.TreeWidth)
	assert.Equal(t, 8, opts.TabWidth)
	assert.True(t, opts.NoColors)
	assert.Equal(t, "dracula", opts.ChromaStyle)
	assert.Equal(t, "HEAD~3", opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
}

func TestParseArgs_TwoRefs(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "main", "feature"))
	require.NoError(t, err)
	assert.Equal(t, "main", opts.Refs.Base)
	assert.Equal(t, "feature", opts.Refs.Against)
	assert.Equal(t, "main..feature", opts.ref())
}

func TestParseArgs_SingleRef(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "HEAD~3"))
	require.NoError(t, err)
	assert.Equal(t, "HEAD~3", opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
	assert.Equal(t, "HEAD~3", opts.ref())
}

func TestParseArgs_DotDotRef(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "main..feature"))
	require.NoError(t, err)
	assert.Equal(t, "main..feature", opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
	assert.Equal(t, "main..feature", opts.ref())
}

func TestParseArgs_NoRef(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Empty(t, opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
	assert.Empty(t, opts.ref())
}

func TestParseArgs_StagedWithTwoRefs(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--staged", "main", "feature"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--staged cannot be used with two-ref diff")
}

func TestParseArgs_StagedWithDotDotRef(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--staged", "main..feature"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--staged cannot be used with two-ref diff")
}

func TestParseArgs_StagedWithTripleDotRef(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--staged", "main...feature"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--staged cannot be used with two-ref diff")
}

func TestParseArgs_StagedWithSingleRef(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--staged", "HEAD~3"))
	require.NoError(t, err)
	assert.True(t, opts.Staged)
	assert.Equal(t, "HEAD~3", opts.Refs.Base)
	assert.Empty(t, opts.Refs.Against)
}

func TestParseArgs_ColorDefaults(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, "#D5895F", opts.Colors.Accent)
	assert.Equal(t, "#585858", opts.Colors.Border)
	assert.Equal(t, "#d0d0d0", opts.Colors.Normal)
	assert.Equal(t, "#585858", opts.Colors.Muted)
	assert.Equal(t, "#87d787", opts.Colors.AddFg)
	assert.Equal(t, "#123800", opts.Colors.AddBg)
	assert.Equal(t, "#ff8787", opts.Colors.RemoveFg)
	assert.Equal(t, "#4D1100", opts.Colors.RemoveBg)
	assert.Equal(t, "#bbbb44", opts.Colors.CursorFg)
	assert.Empty(t, opts.Colors.TreeBg, "tree bg should be empty by default")
	assert.Empty(t, opts.Colors.DiffBg, "diff bg should be empty by default")
	assert.Equal(t, "#202020", opts.Colors.StatusFg)
	assert.Equal(t, "#C5794F", opts.Colors.StatusBg)
}

func TestParseArgs_ColorFlags(t *testing.T) {
	opts, err := parseArgs([]string{"--color-accent=#aabbcc", "--color-remove-bg=#220000"})
	require.NoError(t, err)
	assert.Equal(t, "#aabbcc", opts.Colors.Accent)
	assert.Equal(t, "#220000", opts.Colors.RemoveBg)
}

func TestParseArgs_EnvVars(t *testing.T) {
	t.Setenv("REVDIFF_TREE_WIDTH", "7")
	t.Setenv("REVDIFF_COLOR_ACCENT", "#ff0000")
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, 7, opts.TreeWidth)
	assert.Equal(t, "#ff0000", opts.Colors.Accent)
}

func TestParseArgs_CLIOverridesEnv(t *testing.T) {
	t.Setenv("REVDIFF_TREE_WIDTH", "7")
	opts, err := parseArgs([]string{"--tree-width=9"})
	require.NoError(t, err)
	assert.Equal(t, 9, opts.TreeWidth)
}

func TestParseArgs_ConfigFile(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte(`[Application Options]
tab-width = 2
chroma-style = nord

[color options]
color-accent = #112233
`), 0o600)
	require.NoError(t, err)

	opts, err := parseArgs([]string{"--config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, 2, opts.TabWidth)
	assert.Equal(t, "nord", opts.ChromaStyle)
	assert.Equal(t, "#112233", opts.Colors.Accent)
	// unset values keep defaults
	assert.Equal(t, 2, opts.TreeWidth)
	assert.Equal(t, "#585858", opts.Colors.Border)
}

func TestParseArgs_CLIOverridesConfig(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte(`[Application Options]
tab-width = 2
chroma-style = nord
`), 0o600)
	require.NoError(t, err)

	opts, err := parseArgs([]string{"--config", cfgPath, "--tab-width=6"})
	require.NoError(t, err)
	assert.Equal(t, 6, opts.TabWidth, "CLI flag should override config")
	assert.Equal(t, "nord", opts.ChromaStyle, "config value should be kept when no CLI override")
}

func TestParseArgs_ConfigFileNotFound(t *testing.T) {
	opts, err := parseArgs([]string{"--config", "/nonexistent/path/config"})
	require.NoError(t, err)
	// should use defaults when config not found
	assert.Equal(t, 4, opts.TabWidth)
	assert.Equal(t, "catppuccin-macchiato", opts.ChromaStyle)
}

func TestParseArgs_ConfigFileInvalid(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte(`[invalid
this is not valid ini`), 0o600)
	require.NoError(t, err)

	// should still work, just warn on stderr
	opts, err := parseArgs([]string{"--config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, 4, opts.TabWidth)
}

func TestParseArgs_ConfigColorsOnly(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte(`[color options]
color-add-fg = #00ff00
color-remove-fg = #ff0000
`), 0o600)
	require.NoError(t, err)

	opts, err := parseArgs([]string{"--config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, "#00ff00", opts.Colors.AddFg)
	assert.Equal(t, "#ff0000", opts.Colors.RemoveFg)
	// other colors keep defaults
	assert.Equal(t, "#D5895F", opts.Colors.Accent)
}

func TestResolveFlagPath(t *testing.T) {
	t.Run("from args space form", func(t *testing.T) {
		path := resolveFlagPath([]string{"--myf", "/val"}, "myf", "MY_ENV", func() string { return "/default" })
		assert.Equal(t, "/val", path)
	})
	t.Run("from args equals form", func(t *testing.T) {
		path := resolveFlagPath([]string{"--myf=/val2"}, "myf", "MY_ENV", func() string { return "/default" })
		assert.Equal(t, "/val2", path)
	})
	t.Run("from env", func(t *testing.T) {
		t.Setenv("TEST_FLAG_ENV", "/env/val")
		path := resolveFlagPath([]string{}, "myf", "TEST_FLAG_ENV", func() string { return "/default" })
		assert.Equal(t, "/env/val", path)
	})
	t.Run("args override env", func(t *testing.T) {
		t.Setenv("TEST_FLAG_ENV2", "/env/val")
		path := resolveFlagPath([]string{"--myf", "/args/val"}, "myf", "TEST_FLAG_ENV2", func() string { return "/default" })
		assert.Equal(t, "/args/val", path)
	})
	t.Run("falls back to default", func(t *testing.T) {
		path := resolveFlagPath([]string{}, "myf", "NONEXISTENT_ENV_VAR_12345", func() string { return "/default" })
		assert.Equal(t, "/default", path)
	})
}

func TestResolveFlagPath_configWiring(t *testing.T) {
	path := resolveFlagPath([]string{"--config", "/custom/path"}, "config", "REVDIFF_CONFIG", defaultConfigPath)
	assert.Equal(t, "/custom/path", path)

	t.Setenv("REVDIFF_CONFIG", "/env/config")
	path = resolveFlagPath([]string{}, "config", "REVDIFF_CONFIG", defaultConfigPath)
	assert.Equal(t, "/env/config", path)
}

func TestResolveFlagPath_keysWiring(t *testing.T) {
	path := resolveFlagPath([]string{"--keys", "/custom/keys"}, "keys", "REVDIFF_KEYS", defaultKeysPath)
	assert.Equal(t, "/custom/keys", path)

	t.Setenv("REVDIFF_KEYS", "/env/keys")
	path = resolveFlagPath([]string{}, "keys", "REVDIFF_KEYS", defaultKeysPath)
	assert.Equal(t, "/env/keys", path)
}

func TestParseArgs_ConfigEqualsForm(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte("[Application Options]\ntab-width = 2\n"), 0o600)
	require.NoError(t, err)

	opts, err := parseArgs([]string{"--config=" + cfgPath})
	require.NoError(t, err)
	assert.Equal(t, 2, opts.TabWidth, "config with equals form should be loaded")
}

func TestDumpConfig(t *testing.T) {
	var buf bytes.Buffer
	dumpConfig([]string{"--config", filepath.Join(t.TempDir(), "nonexistent")}, &buf)
	output := buf.String()

	assert.Contains(t, output, "[Application Options]")
	assert.Contains(t, output, "chroma-style = catppuccin-macchiato")
	assert.Contains(t, output, "cross-file-hunks = false")
	assert.Contains(t, output, "[color options]")
	assert.Contains(t, output, "color-accent = #D5895F")
	assert.NotContains(t, output, "\ncolors =", "should not have spurious colors= line")
}

func TestDefaultConfigPath(t *testing.T) {
	path := defaultConfigPath()
	assert.Contains(t, path, ".config")
	assert.Contains(t, path, "revdiff")
	assert.Contains(t, path, "config")
}

func TestMakeGitRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, []string{"file.md"}, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithoutOnly(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// with no --only, returns *diff.Git directly without FallbackRenderer wrapper
	assert.IsType(t, &diff.Git{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeNoVCSRenderer_WithOnly(t *testing.T) {
	tmpDir := t.TempDir()

	renderer, workDir, err := makeNoVCSRenderer([]string{"file.md"}, nil, tmpDir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FileReader{}, renderer)
	assert.Equal(t, tmpDir, workDir)
}

func TestMakeNoVCSRenderer_NoOnly(t *testing.T) {
	renderer, workDir, err := makeNoVCSRenderer(nil, nil, "/tmp")
	require.Error(t, err)
	assert.Nil(t, renderer)
	assert.Empty(t, workDir)
	assert.Contains(t, err.Error(), "no git or mercurial repository found")
}

func TestParseArgs_AllFilesFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--all-files"))
	require.NoError(t, err)
	assert.True(t, opts.AllFiles)
}

func TestParseArgs_AllFilesShortFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "-A"))
	require.NoError(t, err)
	assert.True(t, opts.AllFiles)
}

func TestParseArgs_ExcludeFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--exclude", "vendor", "--exclude", "mocks"))
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor", "mocks"}, opts.Exclude)
}

func TestParseArgs_ExcludeShortFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "-X", "vendor"))
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor"}, opts.Exclude)
}

func TestParseArgs_ExcludeEnvVar(t *testing.T) {
	t.Setenv("REVDIFF_EXCLUDE", "vendor,mocks,testdata")
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor", "mocks", "testdata"}, opts.Exclude)
}

func TestParseArgs_ExcludeConfigFile(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte("[Application Options]\nexclude = vendor\nexclude = mocks\n"), 0o600)
	require.NoError(t, err)
	opts, err := parseArgs([]string{"--config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, []string{"vendor", "mocks"}, opts.Exclude)
}

func TestParseArgs_AllFilesConflictsWithRefs(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--all-files", "HEAD~3"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files cannot be used with refs")
}

func TestParseArgs_AllFilesConflictsWithTwoRefs(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--all-files", "main", "feature"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files cannot be used with refs")
}

func TestParseArgs_AllFilesConflictsWithStaged(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--all-files", "--staged"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files cannot be used with --staged")
}

func TestParseArgs_AllFilesConflictsWithOnly(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--all-files", "--only", "file.go"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files cannot be used with --only")
}

func TestParseArgs_StdinFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--stdin"))
	require.NoError(t, err)
	assert.True(t, opts.Stdin)
}

func TestParseArgs_StdinNameFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--stdin", "--stdin-name", "plan.md"))
	require.NoError(t, err)
	assert.True(t, opts.Stdin)
	assert.Equal(t, "plan.md", opts.StdinName)
}

func TestParseArgs_StdinNameRequiresStdin(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--stdin-name", "plan.md"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--stdin-name requires --stdin")
}

func TestParseArgs_StdinConflicts(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "refs", args: []string{"--stdin", "HEAD~1"}, want: "--stdin cannot be used with refs"},
		{name: "staged", args: []string{"--stdin", "--staged"}, want: "--stdin cannot be used with --staged"},
		{name: "only", args: []string{"--stdin", "--only", "main.go"}, want: "--stdin cannot be used with --only"},
		{name: "all files", args: []string{"--stdin", "--all-files"}, want: "--stdin cannot be used with --all-files"},
		{name: "exclude", args: []string{"--stdin", "--exclude", "vendor"}, want: "--stdin cannot be used with --exclude"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestMakeGitRenderer_AllFiles(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, nil, true, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.DirectoryReader{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_AllFilesUnsupported(t *testing.T) {
	_, _, err := makeHgRenderer(diff.NewHg(""), nil, nil, true, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files is not supported in mercurial")
}

func TestMakeHgRenderer_Default(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, nil, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.Hg{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithOnly(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, []string{"file.go"}, nil, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeHgRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	h := diff.NewHg(dir)
	renderer, workDir, err := makeHgRenderer(h, nil, []string{"vendor"}, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, []string{"vendor"}, false, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeGitRenderer_AllFilesWithExclude(t *testing.T) {
	dir := t.TempDir()
	g := diff.NewGit(dir)
	renderer, workDir, err := makeGitRenderer(g, nil, []string{"vendor", "mocks"}, true, dir)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be ExcludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
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

func TestParseArgs_KeysFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--keys", "/custom/keybindings"))
	require.NoError(t, err)
	assert.Equal(t, "/custom/keybindings", opts.Keys)
}

func TestParseArgs_KeysEqualsForm(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--keys=/custom/keybindings"))
	require.NoError(t, err)
	assert.Equal(t, "/custom/keybindings", opts.Keys)
}

func TestParseArgs_DumpKeysFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--dump-keys"))
	require.NoError(t, err)
	assert.True(t, opts.DumpKeys)
}

func TestDefaultKeysPath(t *testing.T) {
	path := defaultKeysPath()
	assert.Contains(t, path, ".config")
	assert.Contains(t, path, "revdiff")
	assert.Contains(t, path, "keybindings")
}

func TestDetectVCS_Git(t *testing.T) {
	// this test runs from inside the revdiff repo (which is a git repo)
	vcsType, root := diff.DetectVCS(".")
	assert.Equal(t, diff.VCSGit, vcsType)
	assert.DirExists(t, root)
	assert.NotEmpty(t, root)
}

func TestDetectVCS_None(t *testing.T) {
	t.Chdir(t.TempDir())
	vcsType, root := diff.DetectVCS(".")
	assert.Equal(t, diff.VCSNone, vcsType)
	assert.Empty(t, root)
}

func TestApplyTheme(t *testing.T) {
	t.Run("overwrites all 23 fields and chroma-style", func(t *testing.T) {
		opts := options{}
		opts.Colors.Accent = "#original"
		opts.ChromaStyle = "original-style"

		th := theme.Theme{
			ChromaStyle: "dracula",
			Colors: map[string]string{
				"color-accent": "#bd93f9", "color-border": "#6272a4", "color-normal": "#f8f8f2",
				"color-muted": "#6272a4", "color-selected-fg": "#f8f8f2", "color-selected-bg": "#44475a",
				"color-annotation": "#f1fa8c", "color-cursor-fg": "#f8f8f2", "color-cursor-bg": "#44475a",
				"color-add-fg": "#50fa7b", "color-add-bg": "#1a3a1a", "color-remove-fg": "#ff5555",
				"color-remove-bg": "#3a1a1a", "color-word-add-bg": "#2a4a2a", "color-word-remove-bg": "#4a2a2a",
				"color-modify-fg": "#ffb86c", "color-modify-bg": "#3a2a1a",
				"color-tree-bg": "#282a36", "color-diff-bg": "#282a36", "color-status-fg": "#282a36",
				"color-status-bg": "#bd93f9", "color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
			},
		}
		applyTheme(&opts, th)

		// verify chroma-style
		assert.Equal(t, "dracula", opts.ChromaStyle)

		// verify all 23 color fields
		assert.Equal(t, "#bd93f9", opts.Colors.Accent)
		assert.Equal(t, "#6272a4", opts.Colors.Border)
		assert.Equal(t, "#f8f8f2", opts.Colors.Normal)
		assert.Equal(t, "#6272a4", opts.Colors.Muted)
		assert.Equal(t, "#f8f8f2", opts.Colors.SelectedFg)
		assert.Equal(t, "#44475a", opts.Colors.SelectedBg)
		assert.Equal(t, "#f1fa8c", opts.Colors.Annotation)
		assert.Equal(t, "#f8f8f2", opts.Colors.CursorFg)
		assert.Equal(t, "#44475a", opts.Colors.CursorBg)
		assert.Equal(t, "#50fa7b", opts.Colors.AddFg)
		assert.Equal(t, "#1a3a1a", opts.Colors.AddBg)
		assert.Equal(t, "#ff5555", opts.Colors.RemoveFg)
		assert.Equal(t, "#3a1a1a", opts.Colors.RemoveBg)
		assert.Equal(t, "#2a4a2a", opts.Colors.WordAddBg)
		assert.Equal(t, "#4a2a2a", opts.Colors.WordRemoveBg)
		assert.Equal(t, "#ffb86c", opts.Colors.ModifyFg)
		assert.Equal(t, "#3a2a1a", opts.Colors.ModifyBg)
		assert.Equal(t, "#282a36", opts.Colors.TreeBg)
		assert.Equal(t, "#282a36", opts.Colors.DiffBg)
		assert.Equal(t, "#282a36", opts.Colors.StatusFg)
		assert.Equal(t, "#bd93f9", opts.Colors.StatusBg)
		assert.Equal(t, "#282a36", opts.Colors.SearchFg)
		assert.Equal(t, "#f1fa8c", opts.Colors.SearchBg)
	})

	t.Run("chroma-style always overwritten", func(t *testing.T) {
		opts := options{}
		opts.ChromaStyle = "original-style"
		th := theme.Theme{ChromaStyle: "new-style", Colors: map[string]string{}}
		applyTheme(&opts, th)
		assert.Equal(t, "new-style", opts.ChromaStyle)
	})

	t.Run("partial theme overwrites only present keys", func(t *testing.T) {
		opts := options{}
		opts.Colors.Accent = "#original-accent"
		opts.Colors.Border = "#original-border"
		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-accent": "#new-accent"}}
		applyTheme(&opts, th)
		assert.Equal(t, "#new-accent", opts.Colors.Accent)
		assert.Equal(t, "#original-border", opts.Colors.Border, "unset required key should not change opts")
	})

	t.Run("optional keys cleared when absent from theme", func(t *testing.T) {
		opts := options{}
		opts.Colors.CursorBg = "#111111"
		opts.Colors.TreeBg = "#222222"
		opts.Colors.DiffBg = "#333333"
		opts.Colors.WordAddBg = "#444444"
		opts.Colors.WordRemoveBg = "#555555"
		opts.Colors.Accent = "#original-accent"

		// theme has accent but omits all optional keys
		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-accent": "#new-accent"}}
		applyTheme(&opts, th)

		assert.Equal(t, "#new-accent", opts.Colors.Accent)
		assert.Empty(t, opts.Colors.CursorBg, "optional cursor-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.TreeBg, "optional tree-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.DiffBg, "optional diff-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.WordAddBg, "optional word-add-bg should be cleared when theme omits it")
		assert.Empty(t, opts.Colors.WordRemoveBg, "optional word-remove-bg should be cleared when theme omits it")
	})

	t.Run("optional keys preserved when present in theme", func(t *testing.T) {
		opts := options{}
		opts.Colors.TreeBg = "#old-value"

		th := theme.Theme{ChromaStyle: "style", Colors: map[string]string{"color-tree-bg": "#new-tree-bg"}}
		applyTheme(&opts, th)

		assert.Equal(t, "#new-tree-bg", opts.Colors.TreeBg, "optional key present in theme should overwrite")
	})
}

func TestParseArgs_ThemeFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--theme=dracula"))
	require.NoError(t, err)
	assert.Equal(t, "dracula", opts.Theme)
}

func TestParseArgs_ThemeEnv(t *testing.T) {
	t.Setenv("REVDIFF_THEME", "nord")
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, "nord", opts.Theme)
}

func TestParseArgs_DumpThemeFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--dump-theme"))
	require.NoError(t, err)
	assert.True(t, opts.DumpTheme)
}

func TestParseArgs_ListThemesFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--list-themes"))
	require.NoError(t, err)
	assert.True(t, opts.ListThemes)
}

func TestParseArgs_InitThemesFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--init-themes"))
	require.NoError(t, err)
	assert.True(t, opts.InitThemes)
}

func TestDumpThemeOutput(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)

	colors := collectColors(opts)
	var buf bytes.Buffer
	require.NoError(t, theme.Dump(theme.Theme{Colors: colors, ChromaStyle: opts.ChromaStyle}, &buf))
	output := buf.String()
	assert.Contains(t, output, "chroma-style = catppuccin-macchiato")
	assert.Contains(t, output, "color-accent = #D5895F")
	assert.Contains(t, output, "color-add-fg = #87d787")

	// verify dump output can be parsed back (roundtrip)
	th, err := theme.Parse(strings.NewReader(output))
	require.NoError(t, err, "dump-theme output must be parseable")
	assert.Equal(t, opts.ChromaStyle, th.ChromaStyle)
	assert.Equal(t, colors["color-accent"], th.Colors["color-accent"])
}

func TestListThemesOutput(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-latte", "catppuccin-mocha", "dracula", "gruvbox", "nord", "revdiff", "solarized-dark"}, names)
}

func TestCollectColors(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	colors := collectColors(opts)
	assert.Equal(t, "#D5895F", colors["color-accent"])
	assert.Equal(t, "#585858", colors["color-border"])
	assert.Equal(t, "#87d787", colors["color-add-fg"])
	// 5 optional keys (cursor-bg, tree-bg, diff-bg, word-add-bg, word-remove-bg) have no default and are omitted
	assert.Len(t, colors, 18)
	assert.Empty(t, colors["color-cursor-bg"])
	assert.Empty(t, colors["color-tree-bg"])
	assert.Empty(t, colors["color-diff-bg"])
}

func TestColorFieldPtrs(t *testing.T) {
	t.Run("returns 23 entries matching theme.ColorKeys", func(t *testing.T) {
		opts := options{}
		ptrs := colorFieldPtrs(&opts)
		assert.Len(t, ptrs, 23)
		for _, key := range theme.ColorKeys() {
			_, ok := ptrs[key]
			assert.True(t, ok, "missing key %q in colorFieldPtrs", key)
		}
	})

	t.Run("pointers write to correct fields", func(t *testing.T) {
		opts := options{}
		ptrs := colorFieldPtrs(&opts)
		*ptrs["color-accent"] = "#aaa"
		*ptrs["color-search-bg"] = "#bbb"
		assert.Equal(t, "#aaa", opts.Colors.Accent)
		assert.Equal(t, "#bbb", opts.Colors.SearchBg)
	})

	t.Run("applyTheme and collectColors round-trip", func(t *testing.T) {
		// set all colors via applyTheme, then collectColors should return same values
		opts := options{}
		th := theme.Theme{ChromaStyle: "test-style", Colors: map[string]string{}}
		for i, key := range theme.ColorKeys() {
			th.Colors[key] = fmt.Sprintf("#%02x%02x%02x", i*10, i*11, i*12)
		}
		applyTheme(&opts, th)
		collected := collectColors(opts)
		for _, key := range theme.ColorKeys() {
			assert.Equal(t, th.Colors[key], collected[key], "round-trip mismatch for %q", key)
		}
	})
}

func TestThemeOverridesColorFlags(t *testing.T) {
	// simulate the full flow: parseArgs sets --color-accent from CLI,
	// then applyTheme overwrites it with theme value
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	// parse args with explicit --color-accent flag
	opts, err := parseArgs(append(noConfigArgs(t), "--color-accent=#ffffff"))
	require.NoError(t, err)
	assert.Equal(t, "#ffffff", opts.Colors.Accent, "CLI flag sets accent before theme")

	// load theme and overwrite — theme wins unconditionally
	th, err := theme.Load("dracula", themesDir)
	require.NoError(t, err)
	applyTheme(&opts, th)
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override CLI --color-accent")
	assert.Equal(t, "dracula", opts.ChromaStyle, "theme should set chroma-style")
}

func TestResolveThemeConflicts(t *testing.T) {
	t.Run("theme with no-colors clears no-colors", func(t *testing.T) {
		opts := options{Theme: "dracula", NoColors: true}
		resolveThemeConflicts(&opts)
		assert.False(t, opts.NoColors, "--no-colors should be cleared when --theme is set")
	})

	t.Run("no theme keeps no-colors", func(t *testing.T) {
		opts := options{NoColors: true}
		resolveThemeConflicts(&opts)
		assert.True(t, opts.NoColors, "--no-colors should remain when no theme is set")
	})

	t.Run("theme without no-colors is noop", func(t *testing.T) {
		opts := options{Theme: "dracula"}
		resolveThemeConflicts(&opts)
		assert.False(t, opts.NoColors)
	})
}

func TestHandleThemes_InitThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-themes should signal exit")
	assert.Contains(t, stdout.String(), "bundled themes written to")
	assert.Contains(t, stdout.String(), themesDir)
}

func TestHandleThemes_InitThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InitThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_ListThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "list-themes should signal exit")
	output := stdout.String()
	assert.Contains(t, output, "dracula")
	assert.Contains(t, output, "nord")
	assert.NotContains(t, output, "■")
	assert.NotContains(t, output, "\u2713")
}

func TestHandleThemes_ListThemes_localOnly(t *testing.T) {
	themesDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(themesDir, "my-custom"), []byte("custom\n"), 0o600))
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "my-custom")
	assert.NotContains(t, stdout.String(), "◇")
}

func TestHandleThemes_ListThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{ListThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_LoadTheme(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}
	opts.Colors.Accent = "#original"

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done, "load theme should not signal exit")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme should override accent color")
	assert.Equal(t, "dracula", opts.ChromaStyle)
}

func TestHandleThemes_LoadThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "nonexistent"}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
}

func TestHandleThemes_LoadThemeEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula"}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_NoColorsWarning(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))
	var stdout, stderr bytes.Buffer
	opts := options{Theme: "dracula", NoColors: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Contains(t, stderr.String(), "warning: --no-colors ignored when --theme is set")
	assert.False(t, opts.NoColors, "--no-colors should be cleared")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme colors should be applied")
}

func TestHandleThemes_InitAllThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "init-all-themes should signal exit")
	assert.Contains(t, stdout.String(), "all themes written to")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	galleryNames, err := theme.GalleryNames()
	require.NoError(t, err)
	assert.Equal(t, galleryNames, names)
}

func TestHandleThemes_InitAllThemesEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InitAllThemes: true}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_InstallTheme(t *testing.T) {
	themesDir := t.TempDir() // pre-existing dir prevents auto-init
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula", "nord"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done, "install-theme should signal exit")
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord"}, names)
}

func TestHandleThemes_InstallThemeSkipsAutoInitOnFirstRun(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula"}, names)
}

func TestHandleThemes_InstallThemeNotFound(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")
}

func TestHandleThemes_InstallThemeNotFoundDoesNotWriteBundledThemes(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"nonexistent"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "not found in gallery")

	names, listErr := theme.List(themesDir)
	require.NoError(t, listErr)
	assert.Empty(t, names)
}

func TestHandleThemes_InstallThemeEmptyDir(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{"dracula"}}

	done, err := handleThemes(&opts, "", &stdout, &stderr)
	require.Error(t, err)
	assert.False(t, done)
	assert.Contains(t, err.Error(), "cannot determine home directory for themes")
}

func TestHandleThemes_InstallThemeLocalFile(t *testing.T) {
	// create a valid theme file at a local path
	srcDir := t.TempDir()
	content := testThemeContent("my-local")
	srcPath := filepath.Join(srcDir, "my-local")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	themesDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "installed my-local from")
	assert.Contains(t, stdout.String(), "1 theme(s) installed")

	// verify the theme was installed
	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Contains(t, names, "my-local")
}

func TestHandleThemes_InstallThemeMixed(t *testing.T) {
	// mix a gallery name and a local path in one invocation
	srcDir := t.TempDir()
	content := testThemeContent("custom")
	srcPath := filepath.Join(srcDir, "custom")
	require.NoError(t, os.WriteFile(srcPath, []byte(content), 0o600))

	themesDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	opts := options{InstallTheme: []string{srcPath, "dracula"}}

	done, err := handleThemes(&opts, themesDir, &stdout, &stderr)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Contains(t, stdout.String(), "2 theme(s) installed")

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Contains(t, names, "custom")
	assert.Contains(t, names, "dracula")
}

func TestParseArgs_InitAllThemesFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--init-all-themes"))
	require.NoError(t, err)
	assert.True(t, opts.InitAllThemes)
}

func TestParseArgs_InstallThemeFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--install-theme", "dracula", "--install-theme", "nord"))
	require.NoError(t, err)
	assert.Equal(t, []string{"dracula", "nord"}, opts.InstallTheme)
}

func TestHandleThemes_NoOp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	opts := options{}

	done, err := handleThemes(&opts, t.TempDir(), &stdout, &stderr)
	require.NoError(t, err)
	assert.False(t, done)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestDefaultThemesDir(t *testing.T) {
	dir := defaultThemesDir()
	assert.Contains(t, dir, ".config")
	assert.Contains(t, dir, "revdiff")
	assert.Contains(t, dir, "themes")
}
