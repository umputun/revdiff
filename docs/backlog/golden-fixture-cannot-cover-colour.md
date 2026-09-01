---
worth: maybe
where: app/ui/diffview_test.go:1559
added: 2026-09-01
---
# the render golden cannot cover any lipgloss colour

`TestModel_RenderDiffGolden` runs without a tty, so lipgloss degrades to the Ascii profile and every
lipgloss-rendered element lands in `renderdiff.golden` with no colour at all. The colour bytes that
are in the fixture come from the style package's raw-ANSI helpers (`AnsiFg`, `AnnotationInline` and
friends), never from a `lipgloss.Style.Render`.

The gap is wider than one test. It means no golden anywhere pins a `StyleKey`'s resolved colour, so
a change to any lipgloss-built style is invisible to the golden and has to be caught by a direct
assertion on `Resolver.Style(key).GetForeground()`. `TestResolver_AnnotationInputStyles` (added in
#343) is the shape to copy, but only the annotation input keys have one; the rest of `buildStyles`
has only `TestResolver_Style`, which calls `Render("test")` and asserts nothing.

Two ways out, neither obviously right, which is why this is `maybe`. Force a colour profile in the
golden test so lipgloss emits colour under test, which changes an existing fixture and may be
fragile across lipgloss versions. Or add resolved-colour assertions per style key and accept that
the golden covers layout only, which is more code but no fixture churn.

Surfaced by the revmux round on #343, which reported it as pre-existing rather than as a finding
against that change.
