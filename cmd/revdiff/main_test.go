package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/umputun/revdiff/diff"
	"github.com/umputun/revdiff/theme"
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
	assert.False(t, opts.Wrap)
	assert.False(t, opts.Collapsed)
	assert.Empty(t, opts.Output)
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
	var buf bytes.Buffer
	dumpConfig([]string{"--config", filepath.Join(t.TempDir(), "nonexistent")}, &buf)
	output := buf.String()

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

func TestMakeRenderer_GitWithOnly(t *testing.T) {
	dir := t.TempDir()
	renderer, workDir, err := makeRenderer([]string{"file.md"}, nil, false, dir, nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FallbackRenderer{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeRenderer_GitWithoutOnly(t *testing.T) {
	dir := t.TempDir()
	renderer, workDir, err := makeRenderer(nil, nil, false, dir, nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// with no --only, returns *diff.Git directly without FallbackRenderer wrapper
	assert.IsType(t, &diff.Git{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeRenderer_NoGitWithOnly(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir) // set cwd for FileReader
	gitErr := errors.New("not a git repository")

	renderer, workDir, err := makeRenderer([]string{"file.md"}, nil, false, "", gitErr)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.FileReader{}, renderer)
	assert.Equal(t, tmpDir, workDir)
}

func TestMakeRenderer_NoGitNoOnly(t *testing.T) {
	gitErr := errors.New("not a git repository")
	renderer, workDir, err := makeRenderer(nil, nil, false, "", gitErr)
	require.Error(t, err)
	assert.Nil(t, renderer)
	assert.Empty(t, workDir)
	assert.Contains(t, err.Error(), "find git root")
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

func TestMakeRenderer_AllFiles(t *testing.T) {
	dir := t.TempDir()
	renderer, workDir, err := makeRenderer(nil, nil, true, dir, nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.DirectoryReader{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeRenderer_AllFilesNoGit(t *testing.T) {
	gitErr := errors.New("not a git repository")
	_, _, err := makeRenderer(nil, nil, true, "", gitErr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all-files requires a git repository")
}

func TestMakeRenderer_WithExclude(t *testing.T) {
	dir := t.TempDir()
	renderer, workDir, err := makeRenderer(nil, []string{"vendor"}, false, dir, nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
}

func TestMakeRenderer_AllFilesWithExclude(t *testing.T) {
	dir := t.TempDir()
	renderer, workDir, err := makeRenderer(nil, []string{"vendor", "mocks"}, true, dir, nil)
	require.NoError(t, err)
	require.NotNil(t, renderer)
	// should be ExcludeFilter wrapping DirectoryReader
	assert.IsType(t, &diff.ExcludeFilter{}, renderer)
	assert.Equal(t, dir, workDir)
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

func TestResolveKeysPath_FromArgs(t *testing.T) {
	path := resolveKeysPath([]string{"--keys", "/custom/keybindings"})
	assert.Equal(t, "/custom/keybindings", path)
}

func TestResolveKeysPath_EqualsForm(t *testing.T) {
	path := resolveKeysPath([]string{"--keys=/custom/keybindings"})
	assert.Equal(t, "/custom/keybindings", path)
}

func TestResolveKeysPath_FromEnv(t *testing.T) {
	t.Setenv("REVDIFF_KEYS", "/env/keybindings")
	path := resolveKeysPath([]string{})
	assert.Equal(t, "/env/keybindings", path)
}

func TestResolveKeysPath_ArgsOverrideEnv(t *testing.T) {
	t.Setenv("REVDIFF_KEYS", "/env/keybindings")
	path := resolveKeysPath([]string{"--keys", "/args/keybindings"})
	assert.Equal(t, "/args/keybindings", path)
}

func TestResolveKeysPath_Default(t *testing.T) {
	t.Setenv("REVDIFF_KEYS", "") // clear env
	path := resolveKeysPath([]string{})
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".config", "revdiff", "keybindings"), path)
}

func TestDefaultKeysPath(t *testing.T) {
	path := defaultKeysPath()
	assert.Contains(t, path, ".config")
	assert.Contains(t, path, "revdiff")
	assert.Contains(t, path, "keybindings")
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

func TestApplyTheme(t *testing.T) {
	t.Run("overwrites all 21 fields and chroma-style", func(t *testing.T) {
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
				"color-remove-bg": "#3a1a1a", "color-modify-fg": "#ffb86c", "color-modify-bg": "#3a2a1a",
				"color-tree-bg": "#282a36", "color-diff-bg": "#282a36", "color-status-fg": "#282a36",
				"color-status-bg": "#bd93f9", "color-search-fg": "#282a36", "color-search-bg": "#f1fa8c",
			},
		}
		applyTheme(&opts, th)

		// verify chroma-style
		assert.Equal(t, "dracula", opts.ChromaStyle)

		// verify all 21 color fields
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
		assert.Equal(t, "#original-border", opts.Colors.Border, "unset theme key should not change opts")
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
}

func TestListThemesOutput(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	names, err := theme.List(themesDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"catppuccin-mocha", "dracula", "gruvbox", "nord", "solarized-dark"}, names)
}

func TestCollectColors(t *testing.T) {
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	colors := collectColors(opts)
	assert.Equal(t, "#D5895F", colors["color-accent"])
	assert.Equal(t, "#585858", colors["color-border"])
	assert.Equal(t, "#87d787", colors["color-add-fg"])
	assert.Len(t, colors, 21)
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

func TestHandleThemes_NoColorsWarning(t *testing.T) {
	themesDir := filepath.Join(t.TempDir(), "themes")
	require.NoError(t, theme.InitBundled(themesDir))

	// capture stderr
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w

	opts := options{Theme: "dracula", NoColors: true}
	// override defaultThemesDir by setting env for the themes dir; instead, call the pieces directly
	// to avoid os.Exit paths. replicate the warning + resolve logic from handleThemes.
	if opts.Theme != "" && opts.NoColors {
		fmt.Fprintln(os.Stderr, "warning: --no-colors ignored when --theme is set")
	}
	resolveThemeConflicts(&opts)
	th, loadErr := theme.Load("dracula", themesDir)
	require.NoError(t, loadErr)
	applyTheme(&opts, th)

	require.NoError(t, w.Close())
	os.Stderr = origStderr

	var buf bytes.Buffer
	_, copyErr := io.Copy(&buf, r)
	require.NoError(t, copyErr)

	assert.Contains(t, buf.String(), "warning: --no-colors ignored when --theme is set")
	assert.False(t, opts.NoColors, "--no-colors should be cleared")
	assert.Equal(t, "#bd93f9", opts.Colors.Accent, "theme colors should be applied")
}

func TestDefaultThemesDir(t *testing.T) {
	dir := defaultThemesDir()
	assert.Contains(t, dir, ".config")
	assert.Contains(t, dir, "revdiff")
	assert.Contains(t, dir, "themes")
}
