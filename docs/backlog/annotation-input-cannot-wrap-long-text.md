---
worth: yes
where: app/ui/annotate.go:41
added: 2026-09-01
---
# annotation input hides the start of a long annotation

`newAnnotationInput` uses `bubbles/textinput`, which is a single-line horizontal scroll window:
`handleOverflow` renders only `value[offset:offsetRight]` within `Width`. With `annotCharLimit` at
8000 the beginning of a long annotation scrolls off and can only be reached by moving the cursor
back through it. No styling change can fix this, the control has to change.

`bubbles/textarea` is the right control and is not vendored yet, though `bubbles v1.0.0` is already
a direct require so it is a `go mod vendor` away rather than a new dependency. Integration notes
from working through it:

- `SetHeight` is explicit, textarea does not auto-grow. Pick a bounded visible height and let it
  scroll internally, or compute the wrapped height.
- `LineCount()` is `len(m.value)`, logical lines rather than wrapped rows. `LineInfo().Height` covers
  only the current logical line, which is the total only while the value stays one logical line.
- Today's `textinput` sanitizer collapses a pasted `\n` to a space, so the value is always one
  logical line. `textarea` does not. Keeping that collapse preserves current behavior and leaves
  ctrl+e / `$EDITOR` as the multi-line path.
- `View()` emits one row per wrapped line and calls `getPromptString(displayLine)` per row, so the
  prompt function is the route to the gold prefix on row 0 with matching indent after.
- `ShowLineNumbers` defaults to true and must be turned off.

Scroll is smaller than it looks. `hunkLineHeight` and `cursorVisualHeight` never count the live
input at all, since they add rows only for a *saved* annotation found in `annotationSet`. The one
place that knows the input's position is `ensureLineAnnotationInputVisible`
(`app/ui/annotate.go:129`), which hardcodes a single row and has one caller, `startAnnotation`. It
fires only on entry, so even today nothing re-scrolls when the input would grow. That function needs
an input-height term, a target of `inputY + height - 1`, and a call when the wrapped row count
changes. The file-level input needs nothing: `startFileAnnotation` ends with `GotoTop()`, which is
height-agnostic.

Enter saves at `annotate.go:354` and is intercepted before the message reaches the input, so
Enter-saves survives the swap unchanged.

Raised by Umputun while reviewing the #343 colour fix; deliberately kept out of that change since
the two share only the function they touch.
