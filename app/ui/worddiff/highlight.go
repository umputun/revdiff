package worddiff

import "strings"

// InsertHighlightMarkers walks the string inserting hlOn/hlOff ANSI sequences at match positions,
// skipping over existing ANSI escape sequences to preserve them.
// tracks background ANSI state so that match-end restores to the correct bg (e.g. word-diff bg)
// rather than always using the static hlOff (line bg). when the input has no bg sequences
// (word-diff caller), restoreBg stays at hlOff and behavior is unchanged.
func (d *Differ) InsertHighlightMarkers(s string, matches []Range, hlOn, hlOff string) string {
	var b strings.Builder
	bytePos := 0  // byte position in visible (ANSI-stripped) text
	matchIdx := 0 // current match we're processing
	i := 0
	restoreBg := hlOff // tracks the active bg to restore after each match
	inMatch := false   // whether we're inside an active highlight span

	for i < len(s) {
		// skip ANSI escape sequences (copy them as-is, track bg state)
		if s[i] == '\033' {
			j := i + 1
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // include the 'm'
			}
			seq := s[i:j]
			b.WriteString(seq)
			prev := restoreBg
			restoreBg = d.updateRestoreBg(seq, restoreBg, hlOff)
			// if a bg-changing sequence appeared inside a match, re-emit hlOn
			// so the terminal doesn't switch away from the highlight
			if inMatch && restoreBg != prev {
				b.WriteString(hlOn)
			}
			i = j
			continue
		}

		// insert highlight start/end at match boundaries
		if matchIdx < len(matches) && bytePos == matches[matchIdx].Start {
			b.WriteString(hlOn)
			inMatch = true
		}
		if matchIdx < len(matches) && bytePos == matches[matchIdx].End {
			b.WriteString(restoreBg)
			inMatch = false
			matchIdx++
			if matchIdx < len(matches) && bytePos == matches[matchIdx].Start {
				b.WriteString(hlOn)
				inMatch = true
			}
		}

		b.WriteByte(s[i])
		bytePos++
		i++
	}

	// close any unclosed highlight
	if matchIdx < len(matches) && bytePos >= matches[matchIdx].Start && bytePos <= matches[matchIdx].End {
		b.WriteString(restoreBg)
	}

	return b.String()
}

// updateRestoreBg checks if an ANSI escape sequence changes the background or reverse-video state,
// returning the updated restore sequence. resets to hlOff on bg-reset, reverse-off, or full-reset.
func (d *Differ) updateRestoreBg(seq, current, hlOff string) string {
	if len(seq) < 3 || seq[0] != '\033' || seq[1] != '[' || seq[len(seq)-1] != 'm' {
		return current
	}
	params := seq[2 : len(seq)-1]
	switch {
	case strings.HasPrefix(params, "48;2;"): // 24-bit bg
		return seq
	case len(params) == 2 && params[0] == '4' && params[1] >= '0' && params[1] <= '7': // basic bg
		return seq
	case params == "7": // reverse video on
		return seq
	case params == "49" || params == "27" || params == "0" || params == "": // bg/reverse/full reset
		return hlOff
	}
	return current
}
