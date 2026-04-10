package ui

import (
	"fmt"
	"math"
)

// shiftLightness shifts a hex color's lightness toward 0.5 by the given amount.
// dark colors get lighter, light colors get darker. returns "#RRGGBB" format.
// returns the input unchanged if it's empty or unparseable.
func shiftLightness(hexColor string, amount float64) string {
	if hexColor == "" {
		return ""
	}
	r, g, b, ok := parseHexRGB(hexColor)
	if !ok {
		return hexColor
	}
	h, s, l := rgbToHSL(r, g, b)
	if l < 0.5 {
		l += amount // dark color → make lighter
	} else {
		l -= amount // light color → make darker
	}
	l = math.Max(0, math.Min(1, l))
	nr, ng, nb := hslToRGB(h, s, l)
	return fmt.Sprintf("#%02x%02x%02x", nr, ng, nb)
}

// parseHexRGB parses "#RRGGBB" into 0-255 components.
// returns ok=false if the string is not exactly 7 chars, missing the leading #,
// or contains any non-hex digits.
func parseHexRGB(hex string) (r, g, b uint8, ok bool) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, 0, 0, false
	}
	for i := 1; i < 7; i++ {
		c := hex[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return 0, 0, 0, false
		}
	}
	rv := hexVal(hex[1])<<4 | hexVal(hex[2])
	gv := hexVal(hex[3])<<4 | hexVal(hex[4])
	bv := hexVal(hex[5])<<4 | hexVal(hex[6])
	return rv, gv, bv, true
}

// rgbToHSL converts RGB (0-255) to HSL (h: 0-360, s/l: 0-1).
func rgbToHSL(r, g, b uint8) (h, s, l float64) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	l = (maxC + minC) / 2

	if maxC == minC {
		return 0, 0, l // achromatic
	}

	d := maxC - minC
	if l > 0.5 {
		s = d / (2 - maxC - minC)
	} else {
		s = d / (maxC + minC)
	}

	switch maxC {
	case rf:
		h = (gf - bf) / d
		if gf < bf {
			h += 6
		}
	case gf:
		h = (bf-rf)/d + 2
	case bf:
		h = (rf-gf)/d + 4
	}
	h *= 60
	return h, s, l
}

// hslToRGB converts HSL (h: 0-360, s/l: 0-1) to RGB (0-255).
func hslToRGB(h, s, l float64) (r, g, b uint8) {
	if s == 0 {
		v := uint8(math.Round(l * 255))
		return v, v, v
	}

	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q

	hNorm := h / 360
	rf := hueToRGB(p, q, hNorm+1.0/3)
	gf := hueToRGB(p, q, hNorm)
	bf := hueToRGB(p, q, hNorm-1.0/3)

	return uint8(math.Round(rf * 255)), uint8(math.Round(gf * 255)), uint8(math.Round(bf * 255))
}

// hueToRGB is a helper for HSL→RGB conversion.
func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	default:
		return p
	}
}
