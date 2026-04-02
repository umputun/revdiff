package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noConfigArgs returns args that point to a nonexistent config file,
// isolating the test from user's real config.
func noConfigArgs(t *testing.T) []string {
	t.Helper()
	return []string{"--config", filepath.Join(t.TempDir(), "none")}
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
	assert.Empty(t, opts.Output)
	assert.Empty(t, opts.Ref.Ref)
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
	assert.Equal(t, "HEAD~3", opts.Ref.Ref)
}

func TestParseArgs_ColorDefaults(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, "#D5895F", opts.Colors.Accent)
	assert.Equal(t, "#585858", opts.Colors.Border)
	assert.Equal(t, "#d0d0d0", opts.Colors.Normal)
	assert.Equal(t, "#6c6c6c", opts.Colors.Muted)
	assert.Equal(t, "#87d787", opts.Colors.AddFg)
	assert.Equal(t, "#123800", opts.Colors.AddBg)
	assert.Equal(t, "#ff8787", opts.Colors.RemoveFg)
	assert.Equal(t, "#4D1100", opts.Colors.RemoveBg)
	assert.Equal(t, "#bbbb44", opts.Colors.CursorFg)
	assert.Empty(t, opts.Colors.TreeBg, "tree bg should be empty by default")
	assert.Empty(t, opts.Colors.DiffBg, "diff bg should be empty by default")
	assert.Equal(t, "#2D2D2D", opts.Colors.StatusFg)
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

func TestResolveConfigPath_FromArgs(t *testing.T) {
	path := resolveConfigPath([]string{"--config", "/custom/path"})
	assert.Equal(t, "/custom/path", path)
}

func TestResolveConfigPath_EqualsForm(t *testing.T) {
	path := resolveConfigPath([]string{"--config=/custom/path"})
	assert.Equal(t, "/custom/path", path)
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

func TestResolveConfigPath_FromEnv(t *testing.T) {
	t.Setenv("REVDIFF_CONFIG", "/env/config/path")
	path := resolveConfigPath([]string{})
	assert.Equal(t, "/env/config/path", path)
}

func TestResolveConfigPath_ArgsOverrideEnv(t *testing.T) {
	t.Setenv("REVDIFF_CONFIG", "/env/path")
	path := resolveConfigPath([]string{"--config", "/args/path"})
	assert.Equal(t, "/args/path", path, "args should take precedence over env")
}

func TestResolveConfigPath_Default(t *testing.T) {
	t.Setenv("REVDIFF_CONFIG", "") // clear env
	path := resolveConfigPath([]string{})
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "revdiff", "config"), path)
}

func TestDumpConfig(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "config-dump")
	require.NoError(t, err)
	defer tmpFile.Close()

	dumpConfig([]string{"--config", filepath.Join(t.TempDir(), "nonexistent")}, tmpFile)

	data, err := os.ReadFile(tmpFile.Name())
	require.NoError(t, err)
	output := string(data)

	assert.Contains(t, output, "[Application Options]")
	assert.Contains(t, output, "chroma-style = catppuccin-macchiato")
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

func TestGitTopLevel(t *testing.T) {
	t.Run("inside repo", func(t *testing.T) {
		root, err := gitTopLevel()
		require.NoError(t, err)
		assert.DirExists(t, root)
		assert.NotEmpty(t, root)
	})

	t.Run("outside repo", func(t *testing.T) {
		t.Chdir(t.TempDir())
		_, err := gitTopLevel()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "git rev-parse --show-toplevel")
	})
}
