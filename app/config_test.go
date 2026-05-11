package main

import (
	"bytes"
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
	assert.False(t, opts.NoMouse)
	assert.False(t, opts.Wrap)
	assert.False(t, opts.Collapsed)
	assert.False(t, opts.Compact)
	assert.Equal(t, 5, opts.CompactContext)
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

func TestParseArgs_NoMouse(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--no-mouse"))
		require.NoError(t, err)
		assert.True(t, opts.NoMouse)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_NO_MOUSE", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.NoMouse)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nno-mouse = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.NoMouse)
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

func TestParseArgs_WrapIndent(t *testing.T) {
	t.Run("default is zero", func(t *testing.T) {
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.Equal(t, 0, opts.WrapIndent)
	})

	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--wrap-indent=4"))
		require.NoError(t, err)
		assert.Equal(t, 4, opts.WrapIndent)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_WRAP_INDENT", "2")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.Equal(t, 2, opts.WrapIndent)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nwrap-indent = 3\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.Equal(t, 3, opts.WrapIndent)
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

func TestParseArgs_Compact(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--compact"))
		require.NoError(t, err)
		assert.True(t, opts.Compact)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_COMPACT", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.Compact)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\ncompact = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.Compact)
	})
}

func TestParseArgs_CompactContext(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.Equal(t, 5, opts.CompactContext)
	})

	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--compact-context=10"))
		require.NoError(t, err)
		assert.Equal(t, 10, opts.CompactContext)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_COMPACT_CONTEXT", "7")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.Equal(t, 7, opts.CompactContext)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\ncompact-context = 3\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.Equal(t, 3, opts.CompactContext)
	})
}

func TestParseArgs_CompactContextRejectsZero(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "zero with compact", args: []string{"--compact", "--compact-context=0"}},
		{name: "negative with compact", args: []string{"--compact", "--compact-context=-1"}},
		{name: "zero without compact", args: []string{"--compact-context=0"}},
		{name: "negative without compact", args: []string{"--compact-context=-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "--compact-context must be >= 1")
		})
	}
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

func TestParseArgs_Untracked(t *testing.T) {
	t.Run("default off", func(t *testing.T) {
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.False(t, opts.Untracked)
	})

	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--untracked"))
		require.NoError(t, err)
		assert.True(t, opts.Untracked)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_UNTRACKED", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.Untracked)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nuntracked = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.Untracked)
	})
}

func TestOptions_StartupUntracked(t *testing.T) {
	mk := func(untracked, staged bool, base, against string) options {
		var o options
		o.Untracked = untracked
		o.Staged = staged
		o.Refs.Base = base
		o.Refs.Against = against
		return o
	}
	cases := []struct {
		name string
		opts options
		want bool
	}{
		{"flag off", mk(false, false, "", ""), false},
		{"flag on, no ref", mk(true, false, "", ""), true},
		{"flag on with --staged (no positional ref)", mk(true, true, "", ""), true},
		{"flag on, single ref", mk(true, false, "main", ""), true},
		{"flag on, two refs (a b form)", mk(true, false, "main", "feature"), false},
		{"flag on, dot-dot ref (a..b form)", mk(true, false, "main..feature", ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.opts.startupUntracked())
		})
	}
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

func TestParseArgs_VimMotion(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.False(t, opts.VimMotion)
	})

	t.Run("flag", func(t *testing.T) {
		opts, err := parseArgs(append(noConfigArgs(t), "--vim-motion"))
		require.NoError(t, err)
		assert.True(t, opts.VimMotion)
	})

	t.Run("env", func(t *testing.T) {
		t.Setenv("REVDIFF_VIM_MOTION", "true")
		opts, err := parseArgs(noConfigArgs(t))
		require.NoError(t, err)
		assert.True(t, opts.VimMotion)
	})

	t.Run("config file", func(t *testing.T) {
		cfgDir := t.TempDir()
		cfgPath := filepath.Join(cfgDir, "config")
		err := os.WriteFile(cfgPath, []byte("[Application Options]\nvim-motion = true\n"), 0o600)
		require.NoError(t, err)
		opts, err := parseArgs([]string{"--config", cfgPath})
		require.NoError(t, err)
		assert.True(t, opts.VimMotion)
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
	assert.Contains(t, output, "no-mouse = false")
	assert.Contains(t, output, "wrap-indent = 0")
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

func TestParseArgs_IncludeFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "--include", "src", "--include", "lib"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src", "lib"}, opts.Include)
}

func TestParseArgs_IncludeShortFlag(t *testing.T) {
	opts, err := parseArgs(append(noConfigArgs(t), "-I", "src"))
	require.NoError(t, err)
	assert.Equal(t, []string{"src"}, opts.Include)
}

func TestParseArgs_IncludeEnvVar(t *testing.T) {
	t.Setenv("REVDIFF_INCLUDE", "src,lib,cmd")
	opts, err := parseArgs(noConfigArgs(t))
	require.NoError(t, err)
	assert.Equal(t, []string{"src", "lib", "cmd"}, opts.Include)
}

func TestParseArgs_IncludeConfigFile(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config")
	err := os.WriteFile(cfgPath, []byte("[Application Options]\ninclude = src\ninclude = lib\n"), 0o600)
	require.NoError(t, err)
	opts, err := parseArgs([]string{"--config", cfgPath})
	require.NoError(t, err)
	assert.Equal(t, []string{"src", "lib"}, opts.Include)
}

func TestParseArgs_IncludeConflictsWithOnly(t *testing.T) {
	_, err := parseArgs(append(noConfigArgs(t), "--include", "src", "--only", "main.go"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--include cannot be used with --only")
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
		{name: "include", args: []string{"--stdin", "--include", "src"}, want: "--stdin cannot be used with --include"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseArgs(append(noConfigArgs(t), tt.args...))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.want)
		})
	}
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
