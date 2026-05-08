## CLAUDE.md (file-level)
**Markdown showcase** — this annotation exercises every feature of the new
glow-style annotation rendering so you can eyeball them in one place.

You should see *italic*, **bold**, ~~strikethrough~~, and `inline code`
rendered with their actual styling rather than literal markers.

```go
func main() {
    fmt.Println("hello, glamour")
}
```

> Block quotes get their own treatment, usually italic with a leading bar, and are useful for citing reviewer requirements verbatim. The bar should appear on every wrapped line of this paragraph, not just the first one.

A quick **bulleted list**:
- first item with `inline code`
- second item with [a link](https://revdiff.com)
- third item with *emphasis*

And an enumerated one:
1. step one
2. step two
3. step three

---

## README.md (file-level)
Short paragraph annotation: most reviews use a single line of prose, so
this one should look almost identical to the legacy italic-prose path —
no extra padding, no leading blank line, just the 💬 prefix and the body.

## app/main.go (file-level)
### Subheading test
A heading inside an annotation. Content beneath the heading should
flow on the next visual row without weird indentation. The cursor-
height accounting must agree with however many rows this expands into.

| Feature        | Status |
| -------------- | ------ |
| Bold           | ✅     |
| Italic         | ✅     |
| Inline `code`  | ✅     |
| Fenced blocks  | ✅     |

If a table renders here, glamour's table layout is engaged.
