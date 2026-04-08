package keymap

// layoutAlias maps non-Latin characters to their Latin QWERTY equivalents
// based on physical key position. When a user presses a key with a non-Latin
// layout active, the terminal sends the non-Latin character. This map translates
// it back to the Latin equivalent so that key bindings work regardless of layout.
//
// Only non-ASCII characters are mapped to avoid ambiguity — an ASCII character
// like ';' is produced by different physical keys across layouts, so it cannot
// be safely attributed to a single source position.
//
// Mappings are based on standard keyboard layouts per CLDR/OS defaults.
// To add a new layout, append entries for its non-ASCII characters.
var layoutAlias = map[rune]rune{

		// ── Russian ЙЦУКЕН ──────────────────────────────────────────────
		// Also covers: Ukrainian, Belarusian, Kazakh, Serbian Cyrillic.
		// Row 1
		'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y',
		'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p', 'х': '[', 'ъ': ']',
		// Row 2
		'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h',
		'о': 'j', 'л': 'k', 'д': 'l', 'ж': ';', 'э': '\'',
		// Row 3
		'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n',
		'ь': 'm', 'б': ',', 'ю': '.',
		// Uppercase (shift)
		'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y',
		'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P', 'Х': '{', 'Ъ': '}',
		'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H',
		'О': 'J', 'Л': 'K', 'Д': 'L', 'Ж': ':', 'Э': '"',
		'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N',
		'Ь': 'M', 'Б': '<', 'Ю': '>',

		// ── Ukrainian extras (on top of Russian) ────────────────────────
		'і': 's', 'ї': ']', 'є': '\'',
		'І': 'S', 'Ї': '}', 'Є': '"',

		// ── Greek ───────────────────────────────────────────────────────
		// Row 1 (skip ';' at q-position — ASCII, ambiguous)
		'ς': 'w', 'ε': 'e', 'ρ': 'r', 'τ': 't', 'υ': 'y',
		'θ': 'u', 'ι': 'i', 'ο': 'o', 'π': 'p',
		// Row 2
		'α': 'a', 'σ': 's', 'δ': 'd', 'φ': 'f', 'γ': 'g', 'η': 'h',
		'ξ': 'j', 'κ': 'k', 'λ': 'l', 'ή': ';', 'ί': '\'',
		// Row 3
		'ζ': 'z', 'χ': 'x', 'ψ': 'c', 'ω': 'v', 'β': 'b', 'ν': 'n', 'μ': 'm',
		// Uppercase
		'Ε': 'E', 'Ρ': 'R', 'Τ': 'T', 'Υ': 'Y',
		'Θ': 'U', 'Ι': 'I', 'Ο': 'O', 'Π': 'P',
		'Α': 'A', 'Σ': 'S', 'Δ': 'D', 'Φ': 'F', 'Γ': 'G', 'Η': 'H',
		'Ξ': 'J', 'Κ': 'K', 'Λ': 'L', 'Ή': ':', 'Ί': '"',
		'Ζ': 'Z', 'Χ': 'X', 'Ψ': 'C', 'Ω': 'V', 'Β': 'B', 'Ν': 'N', 'Μ': 'M',

		// ── Hebrew ──────────────────────────────────────────────────────
		// Row 1 (skip '/' and ''' at q/w positions — ASCII, ambiguous)
		'ק': 'e', 'ר': 'r', 'א': 't', 'ט': 'y', 'ו': 'u', 'ן': 'i', 'ם': 'o', 'פ': 'p',
		// Row 2
		'ש': 'a', 'ד': 's', 'ג': 'd', 'כ': 'f', 'ע': 'g', 'י': 'h',
		'ח': 'j', 'ל': 'k', 'ך': 'l', 'ף': ';',
		// Row 3 (skip ',' at apostrophe position — ASCII, ambiguous)
		'ז': 'z', 'ס': 'x', 'ב': 'c', 'ה': 'v', 'נ': 'b', 'מ': 'n', 'צ': 'm',
		'ת': ',', 'ץ': '.',
	}

// layoutResolve returns the Latin QWERTY equivalent of a non-Latin character,
// and true if a mapping exists. Returns the original rune and false otherwise.
func layoutResolve(r rune) (rune, bool) {
	alias, ok := layoutAlias[r]
	if ok {
		return alias, true
	}
	return r, false
}
