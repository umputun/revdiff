package theme

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDump_authorAndBundled(t *testing.T) {
	colors := map[string]string{"color-accent": "#bd93f9"}
	var buf bytes.Buffer
	err := (Theme{Name: "test", Author: "Jane Doe", Bundled: true, Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "# author: Jane Doe")
	assert.Contains(t, output, "# bundled: true")
}

func TestDump_noBundledWhenFalse(t *testing.T) {
	colors := map[string]string{"color-accent": "#bd93f9"}
	var buf bytes.Buffer
	err := (Theme{Name: "test", Colors: colors}).Dump(&buf)
	require.NoError(t, err)
	assert.NotContains(t, buf.String(), "bundled")
}

func TestDump_withMetadata(t *testing.T) {
	colors := map[string]string{
		"color-accent": "#bd93f9",
		"color-border": "#6272a4",
	}
	var buf bytes.Buffer
	err := (Theme{Name: "dracula", Description: "purple accent theme", ChromaStyle: "dracula", Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "# name: dracula")
	assert.Contains(t, output, "# description: purple accent theme")
	assert.Contains(t, output, "chroma-style = dracula")
	assert.Contains(t, output, "color-accent = #bd93f9")
	assert.Contains(t, output, "color-border = #6272a4")
}

func TestDump_withoutMetadata(t *testing.T) {
	colors := map[string]string{"color-accent": "#aaa"}
	var buf bytes.Buffer
	err := (Theme{Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "# name:")
	assert.NotContains(t, output, "# description:")
	assert.NotContains(t, output, "chroma-style")
	assert.Contains(t, output, "color-accent = #aaa")
}

func TestDump_canonicalOrder(t *testing.T) {
	colors := map[string]string{
		"color-search-bg": "#111",
		"color-accent":    "#222",
		"color-border":    "#333",
	}
	var buf bytes.Buffer
	err := (Theme{Colors: colors}).Dump(&buf)
	require.NoError(t, err)

	output := buf.String()
	accentIdx := strings.Index(output, "color-accent")
	borderIdx := strings.Index(output, "color-border")
	searchIdx := strings.Index(output, "color-search-bg")
	assert.Less(t, accentIdx, borderIdx, "accent should come before border")
	assert.Less(t, borderIdx, searchIdx, "border should come before search-bg")
}

func Test_colorKeys(t *testing.T) {
	assert.Len(t, colorKeys, 23)
	assert.Equal(t, "color-accent", colorKeys[0])
	assert.Equal(t, "color-search-bg", colorKeys[len(colorKeys)-1])
}
