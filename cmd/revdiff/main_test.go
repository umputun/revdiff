package main

import (
	"testing"

	"github.com/jessevdk/go-flags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_Defaults(t *testing.T) {
	var o struct {
		Ref struct {
			Ref string `positional-arg-name:"ref"`
		} `positional-args:"yes"`
		Staged    bool `long:"staged"`
		TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3"`
		Version   bool `short:"V" long:"version"`
	}
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
			var o struct {
				Ref struct {
					Ref string `positional-arg-name:"ref"`
				} `positional-args:"yes"`
				Staged    bool `long:"staged"`
				TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3"`
				Version   bool `short:"V" long:"version"`
			}
			p := flags.NewParser(&o, flags.Default)
			_, err := p.ParseArgs(tc.args)
			require.NoError(t, err)
			assert.Equal(t, tc.want, o.TreeWidth)
		})
	}
}

func TestCLI_TreeWidthEnv(t *testing.T) {
	t.Setenv("TREE_WIDTH", "7")
	var o struct {
		Ref struct {
			Ref string `positional-arg-name:"ref"`
		} `positional-args:"yes"`
		Staged    bool `long:"staged"`
		TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3"`
		Version   bool `short:"V" long:"version"`
	}
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{})
	require.NoError(t, err)
	assert.Equal(t, 7, o.TreeWidth)
}

func TestCLI_StagedFlag(t *testing.T) {
	var o struct {
		Ref struct {
			Ref string `positional-arg-name:"ref"`
		} `positional-args:"yes"`
		Staged    bool `long:"staged"`
		TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3"`
		Version   bool `short:"V" long:"version"`
	}
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{"--staged"})
	require.NoError(t, err)
	assert.True(t, o.Staged)
}

func TestCLI_PositionalRef(t *testing.T) {
	var o struct {
		Ref struct {
			Ref string `positional-arg-name:"ref"`
		} `positional-args:"yes"`
		Staged    bool `long:"staged"`
		TreeWidth int  `long:"tree-width" env:"TREE_WIDTH" default:"3"`
		Version   bool `short:"V" long:"version"`
	}
	p := flags.NewParser(&o, flags.Default)
	_, err := p.ParseArgs([]string{"HEAD~3"})
	require.NoError(t, err)
	assert.Equal(t, "HEAD~3", o.Ref.Ref)
}
