package style

import "strings"

// SGR handles re-emission of ANSI SGR (Select Graphic Rendition) state
// across line boundaries. it is stateless and zero-value usable — the type
// exists to provide a method namespace for the consumer-side sgrProcessor
// interface contract.
type SGR struct{}

// Reemit scans each line for active SGR attributes (foreground color,
// background color, bold, italic, reverse video) and prepends the
// accumulated state to the next line. this fixes the issue where
// ansi.Wrap splits a line mid-token, causing continuation lines to
// lose their styling.
func (SGR) Reemit(lines []string) []string {
	var st sgrState
	for i, line := range lines {
		if i > 0 {
			if pfx := st.prefix(); pfx != "" {
				lines[i] = pfx + line
			}
		}
		st = st.scan(lines[i])
	}
	return lines
}

// sgrState tracks active SGR attributes during a scan.
type sgrState struct {
	fg, bg       string // e.g. "\033[38;2;100;200;50m" or "\033[48;2;...]m"
	bold, italic bool
	reverse      bool
}

// scan walks the string looking for SGR sequences and returns the updated state.
// tracks foreground color (38;2;r;g;b or 3x), background color (48;2;r;g;b or 4x),
// bold (1), italic (3), reverse video (7) and their resets.
func (s sgrState) scan(line string) sgrState {
	for i := 0; i < len(line); i++ {
		if line[i] != '\033' || i+1 >= len(line) || line[i+1] != '[' {
			continue
		}
		seq, params, end := parseSGR(line, i)
		if end < 0 {
			break
		}
		i = end
		if seq == "" { // not an SGR sequence
			continue
		}
		s = s.applySGR(params, seq)
	}
	return s
}

// prefix assembles an ANSI prefix string from the accumulated SGR state.
func (s sgrState) prefix() string {
	var b strings.Builder
	if s.fg != "" {
		b.WriteString(s.fg)
	}
	if s.bg != "" {
		b.WriteString(s.bg)
	}
	if s.bold {
		b.WriteString("\033[1m")
	}
	if s.italic {
		b.WriteString("\033[3m")
	}
	if s.reverse {
		b.WriteString("\033[7m")
	}
	return b.String()
}

// applySGR updates the active SGR state based on a parameter string.
func (s sgrState) applySGR(params, seq string) sgrState {
	switch params {
	case "", "0": // full reset (\033[m and \033[0m)
		return sgrState{}
	case "1": // bold on
		s.bold = true
		return s
	case "3": // italic on
		s.italic = true
		return s
	case "7": // reverse video on
		s.reverse = true
		return s
	case "22": // bold off
		s.bold = false
		return s
	case "23": // italic off
		s.italic = false
		return s
	case "27": // reverse video off
		s.reverse = false
		return s
	case "39": // fg reset
		s.fg = ""
		return s
	case "49": // bg reset
		s.bg = ""
		return s
	}
	if isFgColor(params) {
		s.fg = seq
		return s
	}
	if isBgColor(params) {
		s.bg = seq
		return s
	}
	return s
}

// parseSGR extracts an SGR sequence starting at position i in s.
// returns the full sequence, the parameter string, and the end index.
// returns end=-1 if the sequence is unterminated.
// returns seq="" if the CSI sequence is not SGR (not terminated by 'm').
func parseSGR(s string, i int) (seq, params string, end int) {
	j := i + 2
	for j < len(s) && s[j] >= 0x20 && s[j] <= 0x3F {
		j++
	}
	if j >= len(s) {
		return "", "", -1
	}
	if s[j] != 'm' {
		return "", "", j
	}
	return s[i : j+1], s[i+2 : j], j
}

// isFgColor returns true if the SGR params represent a foreground color (24-bit or basic).
func isFgColor(params string) bool {
	return strings.HasPrefix(params, "38;2;") ||
		(len(params) == 2 && params[0] == '3' && params[1] >= '0' && params[1] <= '7')
}

// isBgColor returns true if the SGR params represent a background color (24-bit or basic).
func isBgColor(params string) bool {
	return strings.HasPrefix(params, "48;2;") ||
		(len(params) == 2 && params[0] == '4' && params[1] >= '0' && params[1] <= '7')
}
