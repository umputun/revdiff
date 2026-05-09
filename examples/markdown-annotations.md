## CLAUDE.md (file-level)
**Architecture review** — the new `app/markdown/` package is well-isolated and the *fg-only* invariant is documented. One nit: the `Style` struct's hex-color fields could grow into theme keys later — leave a `// TODO(v1)` so future-us doesn't re-derive that conversation.

```go
// app/markdown/style.go
type Style struct {
    BodyFg, HeadingFg, EmphFg string  // TODO(v1): expose via theme keys
    InlineCodeFg, LinkFg      string
    // ...
}
```

> The cursor-math invariant — *one cached `[]string` shared between height and paint* — is the kind of single-source-of-truth pattern that keeps multi-row UI from desyncing. Worth pinning down in `docs/ARCHITECTURE.md`.

## README.md (file-level)
Quick checklist before merge:
- [x] `--plain-annotations` flag mentioned in the keybindings table
- [x] `Ctrl+J` / `Alt+Enter` newline binding documented
- [ ] Add a screenshot of the new rendering to the README hero section
- [ ] Mention `examples/markdown-annotations.md` in the "Try it" snippet

The ~~plain italic~~ glow-style rendering is a meaningful upgrade for `inline code` and fenced blocks during code review. Pair it with [`gum write`'s convention](https://github.com/charmbracelet/gum) and the muscle memory transfers cleanly.

## app/main.go (file-level)
### Composition root
The `buildMarkdownRenderer(opts)` + `markdownRebuilder(opts)` pair is the right shape — startup colors live on `opts`, runtime colors come through `applyTheme`'s `style.Colors`. Both close over the same opt-out flags so `--plain-annotations` and `--no-colors` propagate consistently.

| Concern              | Where it lives                  |
| -------------------- | ------------------------------- |
| Renderer construction | `app/markdown_setup.go`         |
| Theme rebuild path    | `ModelConfig.AnnotationMarkdownBuilder` |
| Cache invalidation    | `applyTheme` + `handleFileLoaded` |
| Cursor-math chokepoint| `annotationVisualRows`          |

One follow-up to consider: the `markdownRebuilder` closure captures `opts` by value, which means `--chroma-style` changes mid-session won't re-derive the markdown style. Probably fine for v0 since chroma style isn't user-toggleable at runtime, but worth a comment.
