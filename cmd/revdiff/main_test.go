package main

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cliOpts struct {
	Ref struct {
		Ref string `positional-arg-name:"ref"`
	} `positional-args:"yes"`
	Staged    bool `long:"staged" env:"REVDIFF_STAGED"`
	TreeWidth int  `long:"tree-width" env:"REVDIFF_TREE_WIDTH" default:"3"`
	Version   bool `short:"V" long:"version"`
	Colors    struct {
		Accent     string `long:"color-accent"      env:"REVDIFF_COLOR_ACCENT"      default:"#5f87ff"`
		Border     string `long:"color-border"      env:"REVDIFF_COLOR_BORDER"      default:"#585858"`
		Normal     string `long:"color-normal"      env:"REVDIFF_COLOR_NORMAL"      default:"#d0d0d0"`
		Muted      string `long:"color-muted"       env:"REVDIFF_COLOR_MUTED"       default:"#6c6c6c"`
		SelectedFg string `long:"color-selected-fg" env:"REVDIFF_COLOR_SELECTED_FG" default:"#ffffaf"`
		SelectedBg string `long:"color-selected-bg" env:"REVDIFF_COLOR_SELECTED_BG" default:"#303030"`
		Annotation string `long:"color-annotation"  env:"REVDIFF_COLOR_ANNOTATION"  default:"#ffd700"`
		CursorBg   string `long:"color-cursor-bg"   env:"REVDIFF_COLOR_CURSOR_BG"   default:"#3a3a3a"`
		AddFg      string `long:"color-add-fg"      env:"REVDIFF_COLOR_ADD_FG"      default:"#87d787"`
		AddBg      string `long:"color-add-bg"      env:"REVDIFF_COLOR_ADD_BG"      default:"#022800"`
		RemoveFg   string `long:"color-remove-fg"   env:"REVDIFF_COLOR_REMOVE_FG"   default:"#ff8787"`
		RemoveBg   string `long:"color-remove-bg"   env:"REVDIFF_COLOR_REMOVE_BG"   default:"#3D0100"`
	} `group:"color options"`
}

func TestCLI_Defaults(t *testing.T) {
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, 3, o.TreeWidth)
	assert.False(t, o.Staged)
	assert.False(t, o.Version)
	assert.Empty(t, o.Ref.Ref)
}

func TestCLI_TreeWidth(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "default", args: []string{}, want: 3},
		{name: "set to 5", args: []string{"--tree-width=5"}, want: 5},
		{name: "set to 1", args: []string{"--tree-width=1"}, want: 1},
		{name: "set to 10", args: []string{"--tree-width=10"}, want: 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var o cliOpts
			p := flags.NewParser(&o, flags.Default)
			_, err := p.ParseArgs(tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.want, o.TreeWidth)
		})
	}
}

func TestCLI_TreeWidthEnv(t *testing.T) {
	t.Setenv("REVDIFF_TREE_WIDTH", "7")
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, 7, o.TreeWidth)
}

func TestCLI_StagedFlag(t *testing.T) {
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{"--staged"})
	require.NoError(t, err)
	assert.True(t, o.Staged)
}

func TestCLI_PositionalRef(t *testing.T) {
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{"HEAD~3"})
	require.NoError(t, err)
	assert.Equal(t, "HEAD~3", o.Ref.Ref)
}

func TestCLI_ColorDefaults(t *testing.T) {
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, "#5f87ff", o.Colors.Accent)
	assert.Equal(t, "#022800", o.Colors.AddBg)
	assert.Equal(t, "#3D0100", o.Colors.RemoveBg)
}

func TestCLI_ColorEnv(t *testing.T) {
	t.Setenv("REVDIFF_COLOR_ACCENT", "#ff0000")
	t.Setenv("REVDIFF_COLOR_ADD_BG", "#001100")
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, "#ff0000", o.Colors.Accent)
	assert.Equal(t, "#001100", o.Colors.AddBg)
}

func TestCLI_ColorFlag(t *testing.T) {
	var o cliOpts
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{"--color-accent=#aabbcc", "--color-remove-bg=#220000"})
	require.NoError(t, err)
	assert.Equal(t, "#aabbcc", o.Colors.Accent)
	assert.Equal(t, "#220000", o.Colors.RemoveBg)
}
