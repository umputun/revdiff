package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShiftLightness_DarkColorGetsLighter(t *testing.T) {
	// dark green bg (#1a3a1a, L≈0.16) should get lighter
	result := ShiftLightness("#1a3a1a", 0.15)
	assert.NotEqual(t, "#1a3a1a", result)

	_, _, lOrig := rgbToHSL(hexToFloats("1a3a1a"))
	_, _, lNew := rgbToHSL(hexToFloats(result[1:])) // strip #
	assert.Greater(t, lNew, lOrig, "dark color should become lighter")
}

func TestShiftLightness_LightColorGetsDarker(t *testing.T) {
	// light pink bg (#f5d0d8, L≈0.89) should get darker
	result := ShiftLightness("#f5d0d8", 0.15)
	assert.NotEqual(t, "#f5d0d8", result)

	_, _, lOrig := rgbToHSL(hexToFloats("f5d0d8"))
	_, _, lNew := rgbToHSL(hexToFloats(result[1:]))
	assert.Less(t, lNew, lOrig, "light color should become darker")
}

func TestShiftLightness_PreservesHue(t *testing.T) {
	// green should stay green
	result := ShiftLightness("#1a3a1a", 0.15)
	hOrig, _, _ := rgbToHSL(hexToFloats("1a3a1a"))
	hNew, _, _ := rgbToHSL(hexToFloats(result[1:]))
	assert.InDelta(t, hOrig, hNew, 0.01, "hue should be preserved")
}

func TestShiftLightness_KnownValues(t *testing.T) {
	tests := []struct {
		name  string
		input string
		delta float64
		want  string
	}{
		{"pure black up", "#000000", 0.15, "#262626"},
		{"pure white down", "#ffffff", 0.15, "#d9d9d9"},
		{"mid gray down", "#808080", 0.15, "#5a5a5a"}, // L=0.5 → darker (L>=0.5 branch)
		{"small delta", "#1a3a1a", 0.05, "#224c22"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShiftLightness(tt.input, tt.delta)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestShiftLightness_InvalidHex(t *testing.T) {
	assert.Equal(t, "#abc", ShiftLightness("#abc", 0.15))    // too short, returned as-is
	assert.Equal(t, "#", ShiftLightness("", 0.15))            // empty
	assert.Equal(t, "#262626", ShiftLightness("#zzzzzz", 0.15)) // non-hex chars, Sscanf fails → treated as black → shifted
}

func TestShiftLightness_NoHashPrefix(t *testing.T) {
	// should work with or without # prefix
	with := ShiftLightness("#1a3a1a", 0.15)
	without := ShiftLightness("1a3a1a", 0.15)
	assert.Equal(t, with, without)
}

func TestRgbToHSL_Roundtrip(t *testing.T) {
	colors := []string{"ff0000", "00ff00", "0000ff", "1a3a1a", "f5d0d8", "4a7a1a", "a03838"}
	for _, hex := range colors {
		t.Run(hex, func(t *testing.T) {
			r, g, b := hexToFloats(hex)
			h, s, l := rgbToHSL(r, g, b)
			rr, gg, bb := hslToRGB(h, s, l)
			assert.InDelta(t, r, rr, 0.005)
			assert.InDelta(t, g, gg, 0.005)
			assert.InDelta(t, b, bb, 0.005)
		})
	}
}
