package ui

import (
	"fmt"
	"math"
	"strings"
)

// ShiftLightness moves a hex color's HSL lightness toward the midpoint by delta.
// dark colors (L < 0.5) get lighter; light colors (L >= 0.5) get darker.
// returns the adjusted color as a "#rrggbb" string.
func ShiftLightness(hex string, delta float64) string {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return "#" + hex
	}
	r, g, b := hexToFloats(hex)
	h, s, l := rgbToHSL(r, g, b)
	if l < 0.5 {
		l = math.Min(l+delta, 1.0)
	} else {
		l = math.Max(l-delta, 0.0)
	}
	rr, gg, bb := hslToRGB(h, s, l)
	return fmt.Sprintf("#%02x%02x%02x", int(math.Round(rr*255)), int(math.Round(gg*255)), int(math.Round(bb*255)))
}

func hexToFloats(hex string) (r, g, b float64) {
	var ri, gi, bi int
	_, _ = fmt.Sscanf(hex, "%02x%02x%02x", &ri, &gi, &bi)
	return float64(ri) / 255, float64(gi) / 255, float64(bi) / 255
}

func rgbToHSL(r, g, b float64) (h, s, l float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, 0, l
	}
	d := mx - mn
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	case b:
		h = (r-g)/d + 4
	}
	h /= 6
	return h, s, l
}

func hslToRGB(h, s, l float64) (r, g, b float64) {
	if s == 0 {
		return l, l, l
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	r = hueToRGB(p, q, h+1.0/3.0)
	g = hueToRGB(p, q, h)
	b = hueToRGB(p, q, h-1.0/3.0)
	return r, g, b
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6.0:
		return p + (q-p)*6*t
	case t < 1.0/2.0:
		return q
	case t < 2.0/3.0:
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}
