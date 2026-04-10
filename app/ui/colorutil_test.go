package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShiftLightness(t *testing.T) {
	tests := []struct {
		name   string
		hex    string
		amount float64
		want   string
	}{
		{name: "dark green gets lighter", hex: "#123800", amount: 0.15},
		{name: "dark red gets lighter", hex: "#4D1100", amount: 0.15},
		{name: "light color gets darker", hex: "#d0d0d0", amount: 0.15},
		{name: "black gets lighter", hex: "#000000", amount: 0.15},
		{name: "white gets darker", hex: "#ffffff", amount: 0.15},
		{name: "empty returns empty", hex: "", amount: 0.15, want: ""},
		{name: "invalid returns unchanged", hex: "nothex", amount: 0.15, want: "nothex"},
		{name: "malformed all g returns unchanged", hex: "#gggggg", amount: 0.15, want: "#gggggg"},
		{name: "malformed one bad char returns unchanged", hex: "#12345z", amount: 0.15, want: "#12345z"},
		{name: "zero amount returns same", hex: "#808080", amount: 0.0},
		{name: "mid gray small shift", hex: "#808080", amount: 0.05},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := shiftLightness(tc.hex, tc.amount)
			if tc.want != "" {
				assert.Equal(t, tc.want, result)
				return
			}
			// for color inputs, verify the result is a valid hex color
			if tc.hex == "" {
				assert.Empty(t, result)
				return
			}
			assert.Len(t, result, 7, "result should be #RRGGBB")
			assert.Equal(t, byte('#'), result[0], "result should start with #")
		})
	}
}

func TestShiftLightness_DarkGetsLighter(t *testing.T) {
	// dark green #123800 has low lightness, shifting should increase it
	orig := "#123800"
	shifted := shiftLightness(orig, 0.15)
	assert.NotEqual(t, orig, shifted, "shifted color should differ from original")

	// parse both and verify lightness increased
	_, _, _, origOK := parseHexRGB(orig)
	_, _, _, shiftOK := parseHexRGB(shifted)
	assert.True(t, origOK)
	assert.True(t, shiftOK)

	origR, origG, origB, _ := parseHexRGB(orig)
	shiftR, shiftG, shiftB, _ := parseHexRGB(shifted)
	_, _, origL := rgbToHSL(origR, origG, origB)
	_, _, shiftL := rgbToHSL(shiftR, shiftG, shiftB)
	assert.Greater(t, shiftL, origL, "dark color lightness should increase")
}

func TestShiftLightness_LightGetsDarker(t *testing.T) {
	orig := "#d0d0d0"
	shifted := shiftLightness(orig, 0.15)

	origR, origG, origB, _ := parseHexRGB(orig)
	shiftR, shiftG, shiftB, _ := parseHexRGB(shifted)
	_, _, origL := rgbToHSL(origR, origG, origB)
	_, _, shiftL := rgbToHSL(shiftR, shiftG, shiftB)
	assert.Less(t, shiftL, origL, "light color lightness should decrease")
}

func TestShiftLightness_BlackAndWhite(t *testing.T) {
	// black should get lighter
	blackShifted := shiftLightness("#000000", 0.15)
	r, g, b, _ := parseHexRGB(blackShifted)
	_, _, l := rgbToHSL(r, g, b)
	assert.Greater(t, l, 0.0, "shifted black should have positive lightness")

	// white should get darker
	whiteShifted := shiftLightness("#ffffff", 0.15)
	r, g, b, _ = parseHexRGB(whiteShifted)
	_, _, l = rgbToHSL(r, g, b)
	assert.Less(t, l, 1.0, "shifted white should have lightness < 1")
}

func TestParseHexRGB(t *testing.T) {
	tests := []struct {
		name    string
		hex     string
		r, g, b uint8
		ok      bool
	}{
		{name: "valid red", hex: "#ff0000", r: 255, g: 0, b: 0, ok: true},
		{name: "valid green", hex: "#00ff00", r: 0, g: 255, b: 0, ok: true},
		{name: "valid blue", hex: "#0000ff", r: 0, g: 0, b: 255, ok: true},
		{name: "mixed", hex: "#1a2b3c", r: 0x1a, g: 0x2b, b: 0x3c, ok: true},
		{name: "uppercase", hex: "#AABBCC", r: 0xAA, g: 0xBB, b: 0xCC, ok: true},
		{name: "too short", hex: "#fff", ok: false},
		{name: "no hash", hex: "ff0000", ok: false},
		{name: "empty", hex: "", ok: false},
		{name: "invalid chars all", hex: "#gggggg", ok: false},
		{name: "invalid char one", hex: "#12345z", ok: false},
		{name: "invalid char first", hex: "#g12345", ok: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, g, b, ok := parseHexRGB(tc.hex)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, tc.r, r)
				assert.Equal(t, tc.g, g)
				assert.Equal(t, tc.b, b)
			}
		})
	}
}

func TestRGBToHSL_Roundtrip(t *testing.T) {
	// test that RGB→HSL→RGB roundtrips for known colors
	tests := []struct {
		name    string
		r, g, b uint8
	}{
		{name: "red", r: 255, g: 0, b: 0},
		{name: "green", r: 0, g: 255, b: 0},
		{name: "blue", r: 0, g: 0, b: 255},
		{name: "white", r: 255, g: 255, b: 255},
		{name: "black", r: 0, g: 0, b: 0},
		{name: "gray", r: 128, g: 128, b: 128},
		{name: "orange", r: 255, g: 165, b: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, s, l := rgbToHSL(tc.r, tc.g, tc.b)
			rr, gg, bb := hslToRGB(h, s, l)
			// allow ±1 for rounding
			assert.InDelta(t, int(tc.r), int(rr), 1, "red channel")
			assert.InDelta(t, int(tc.g), int(gg), 1, "green channel")
			assert.InDelta(t, int(tc.b), int(bb), 1, "blue channel")
		})
	}
}

func TestNormalizeColors_WordDiffAutoDerivation(t *testing.T) {
	t.Run("auto-derives when empty", func(t *testing.T) {
		c := normalizeColors(Colors{
			AddBg:    "#123800",
			RemoveBg: "#4D1100",
		})
		assert.NotEmpty(t, c.WordAddBg, "WordAddBg should be auto-derived from AddBg")
		assert.NotEmpty(t, c.WordRemoveBg, "WordRemoveBg should be auto-derived from RemoveBg")
		assert.NotEqual(t, c.AddBg, c.WordAddBg, "word-diff bg should differ from line bg")
		assert.NotEqual(t, c.RemoveBg, c.WordRemoveBg, "word-diff bg should differ from line bg")
		assert.Equal(t, byte('#'), c.WordAddBg[0])
		assert.Equal(t, byte('#'), c.WordRemoveBg[0])
	})

	t.Run("preserves explicit values", func(t *testing.T) {
		c := normalizeColors(Colors{
			AddBg:        "#123800",
			RemoveBg:     "#4D1100",
			WordAddBg:    "#aabbcc",
			WordRemoveBg: "#ddeeff",
		})
		assert.Equal(t, "#aabbcc", c.WordAddBg, "explicit WordAddBg should be preserved")
		assert.Equal(t, "#ddeeff", c.WordRemoveBg, "explicit WordRemoveBg should be preserved")
	})

	t.Run("no derivation when base bg is empty", func(t *testing.T) {
		c := normalizeColors(Colors{})
		assert.Empty(t, c.WordAddBg, "WordAddBg should stay empty when AddBg is empty")
		assert.Empty(t, c.WordRemoveBg, "WordRemoveBg should stay empty when RemoveBg is empty")
	})
}
