package style

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetConstants(t *testing.T) {
	assert.Equal(t, ResetFg, Color("\033[39m"), "ResetFg should be ESC[39m")
	assert.Equal(t, ResetBg, Color("\033[49m"), "ResetBg should be ESC[49m")
}

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
			if tc.hex == "" {
				assert.Empty(t, result)
				return
			}
			// for valid color inputs, verify the result is a valid hex color
			assert.Len(t, result, 7, "result should be #RRGGBB")
			assert.Equal(t, byte('#'), result[0], "result should start with #")
		})
	}
}

func TestShiftLightness_DarkGetsLighter(t *testing.T) {
	orig := "#123800"
	shifted := shiftLightness(orig, 0.15)
	assert.NotEqual(t, orig, shifted, "shifted color should differ from original")

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

func TestShiftLightness_SaturationZero(t *testing.T) {
	// pure gray has saturation 0 — should still shift
	orig := "#808080"
	shifted := shiftLightness(orig, 0.15)
	require.Len(t, shifted, 7)
	assert.NotEqual(t, orig, shifted)
}

func TestShiftLightness_BoundaryClamp(t *testing.T) {
	// huge amount should clamp to 0 or 1
	shifted := shiftLightness("#ffffff", 2.0) // l=1.0 - 2.0 = -1.0 → clamped to 0
	require.Len(t, shifted, 7)
	r, g, b, ok := parseHexRGB(shifted)
	require.True(t, ok)
	_, _, l := rgbToHSL(r, g, b)
	assert.InDelta(t, 0.0, l, 0.01, "over-shifted white should clamp to black")

	shifted = shiftLightness("#000000", 2.0) // l=0.0 + 2.0 = 2.0 → clamped to 1
	require.Len(t, shifted, 7)
	r, g, b, ok = parseHexRGB(shifted)
	require.True(t, ok)
	_, _, l = rgbToHSL(r, g, b)
	assert.InDelta(t, 1.0, l, 0.01, "over-shifted black should clamp to white")
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
		{name: "too long", hex: "#ff00001", ok: false},
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
		{name: "cyan", r: 0, g: 255, b: 255},
		{name: "magenta", r: 255, g: 0, b: 255},
		{name: "yellow", r: 255, g: 255, b: 0},
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

func TestRGBToHSL_KnownValues(t *testing.T) {
	// pure red: h=0, s=1, l=0.5
	h, s, l := rgbToHSL(255, 0, 0)
	assert.InDelta(t, 0, h, 0.1, "red hue")
	assert.InDelta(t, 1.0, s, 0.01, "red saturation")
	assert.InDelta(t, 0.5, l, 0.01, "red lightness")

	// pure green: h=120, s=1, l=0.5
	h, s, l = rgbToHSL(0, 255, 0)
	assert.InDelta(t, 120, h, 0.1, "green hue")
	assert.InDelta(t, 1.0, s, 0.01, "green saturation")
	assert.InDelta(t, 0.5, l, 0.01, "green lightness")

	// achromatic gray: h=0, s=0
	h, s, _ = rgbToHSL(128, 128, 128)
	assert.InDelta(t, 0, h, 0.1, "gray hue")
	assert.InDelta(t, 0, s, 0.01, "gray saturation")
}

func TestHueToRGB_AllBranches(t *testing.T) {
	tests := []struct {
		name string
		p, q float64
		t    float64
		want float64
	}{
		{name: "t < 0 wraps", p: 0.1, q: 0.5, t: -0.2},     // t becomes 0.8 → default branch
		{name: "t > 1 wraps", p: 0.1, q: 0.5, t: 1.3},      // t becomes 0.3 → t < 1/2 branch
		{name: "t < 1/6", p: 0.1, q: 0.5, t: 0.1},          // first branch: p + (q-p)*6*t
		{name: "t < 1/2", p: 0.1, q: 0.5, t: 0.3},          // second branch: q
		{name: "t < 2/3", p: 0.1, q: 0.5, t: 0.6},          // third branch: p + (q-p)*(2/3-t)*6
		{name: "t >= 2/3 default", p: 0.1, q: 0.5, t: 0.8}, // default branch: p
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := hueToRGB(tc.p, tc.q, tc.t)
			assert.GreaterOrEqual(t, result, 0.0, "result should be >= 0")
			assert.LessOrEqual(t, result, 1.0, "result should be <= 1")
		})
	}
}

func TestHueToRGB_BranchValues(t *testing.T) {
	// verify exact results for known branch computations
	t.Run("t < 1/6 formula", func(t *testing.T) {
		// p + (q-p)*6*t = 0.1 + (0.5-0.1)*6*0.1 = 0.1 + 0.24 = 0.34
		result := hueToRGB(0.1, 0.5, 0.1)
		assert.InDelta(t, 0.34, result, 0.001)
	})

	t.Run("t < 1/2 returns q", func(t *testing.T) {
		result := hueToRGB(0.1, 0.5, 0.3)
		assert.InDelta(t, 0.5, result, 0.001)
	})

	t.Run("t >= 2/3 returns p", func(t *testing.T) {
		result := hueToRGB(0.1, 0.5, 0.8)
		assert.InDelta(t, 0.1, result, 0.001)
	})
}

func TestHexVal(t *testing.T) {
	tests := []struct {
		name string
		c    byte
		want byte
	}{
		{name: "digit 0", c: '0', want: 0},
		{name: "digit 5", c: '5', want: 5},
		{name: "digit 9", c: '9', want: 9},
		{name: "lowercase a", c: 'a', want: 10},
		{name: "lowercase c", c: 'c', want: 12},
		{name: "lowercase f", c: 'f', want: 15},
		{name: "uppercase A", c: 'A', want: 10},
		{name: "uppercase C", c: 'C', want: 12},
		{name: "uppercase F", c: 'F', want: 15},
		{name: "invalid g", c: 'g', want: 0},
		{name: "invalid space", c: ' ', want: 0},
		{name: "invalid z", c: 'z', want: 0},
		{name: "invalid G", c: 'G', want: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hexVal(tc.c))
		})
	}
}

func TestAnsiColor(t *testing.T) {
	tests := []struct {
		name string
		hex  string
		code int
		want string
	}{
		{name: "fg red with hash", hex: "#ff0000", code: 38, want: "\033[38;2;255;0;0m"},
		{name: "bg green with hash", hex: "#00ff00", code: 48, want: "\033[48;2;0;255;0m"},
		{name: "fg blue without hash", hex: "0000ff", code: 38, want: "\033[38;2;0;0;255m"},
		{name: "mixed case hex", hex: "#aAbBcC", code: 38, want: "\033[38;2;170;187;204m"},
		{name: "uppercase hex", hex: "#AABBCC", code: 48, want: "\033[48;2;170;187;204m"},
		{name: "black", hex: "#000000", code: 38, want: "\033[38;2;0;0;0m"},
		{name: "white", hex: "#ffffff", code: 38, want: "\033[38;2;255;255;255m"},
		{name: "empty string", hex: "", code: 38, want: ""},
		{name: "too short", hex: "#fff", code: 38, want: ""},
		{name: "too long", hex: "#ff00001", code: 38, want: ""},
		{name: "invalid chars", hex: "#gggggg", code: 38, want: ""},
		{name: "one invalid char", hex: "#12345z", code: 38, want: ""},
		{name: "just hash", hex: "#", code: 38, want: ""},
		{name: "no hash wrong length", hex: "fff", code: 38, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ansiColor(tc.hex, tc.code))
		})
	}
}

func TestWrite_EmptyColor(t *testing.T) {
	var buf bytes.Buffer
	n, err := Write(&buf, "")
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	assert.Empty(t, buf.String())
}

func TestWrite_NonEmptyColor(t *testing.T) {
	var buf bytes.Buffer
	c := Color("\033[38;2;255;0;0m")
	n, err := Write(&buf, c)
	require.NoError(t, err)
	assert.Equal(t, len(string(c)), n)
	assert.Equal(t, string(c), buf.String())
}

func TestWrite_PlainText(t *testing.T) {
	var buf bytes.Buffer
	c := Color("hello")
	n, err := Write(&buf, c)
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
}

// errWriter is a writer that always returns an error.
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestWrite_WriterError(t *testing.T) {
	testErr := errors.New("write failed")
	w := errWriter{err: testErr}
	n, err := Write(w, Color("data"))
	assert.Equal(t, 0, n)
	require.ErrorIs(t, err, testErr)
	assert.Contains(t, err.Error(), "write color")
}
