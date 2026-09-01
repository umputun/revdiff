---
worth: maybe
where: themes/gallery/catppuccin-latte
added: 2026-09-01
---
# catppuccin-latte and solarized-dark annotation colours are below WCAG AA

`color-annotation` against `color-diff-bg` is 2.31 in catppuccin-latte (`#df8e1d` on `#eff1f5`) and
3.26 in solarized-dark (`#cb4b16` on `#002b36`). AA is 4.5 and AA-large is 3.0, so Latte clears
neither. This already affects every *saved* annotation in those two themes and is not new; the #343
colour fix made the live input inherit the same ratios, which is why it surfaced.

Repair is not mechanical, which is why this is `maybe` rather than `yes`. Of Catppuccin Latte's 14
official accent colours only Red `#d20f39` (4.80) and Mauve `#8839ef` (4.79) clear AA on `#eff1f5`,
and the theme already assigns both, Red to `color-remove-fg` and Mauve to `color-accent` /
`color-selected-bg` / `color-status-bg`. Fixing Latte therefore means an off-palette shade in a
theme whose whole value is fidelity to upstream. Solarized Dark is easier: `cyan #2aa198` at 4.75 is
official, unused elsewhere in that theme and clears AA, at the cost of the annotation reading cool
rather than warm.

The trade was accepted knowingly in #343 and confirmed by eye on the built binary. Left here so the
underlying palette question is not lost.
