# revdiff examples

Manual-test fixtures used during development and demos. None of these are
referenced by the unit-test suite — they exist for eyeballing rendering.

## `markdown-annotations.md`

Showcase annotation file that exercises every feature of the glow-style
annotation rendering: bold, italic, strikethrough, inline code, fenced code
blocks, blockquotes, bulleted and enumerated lists, headings, links, and
tables. All annotations are **file-level** so they always attach regardless
of what's in the current diff.

All annotations are **file-level** so they always attach regardless of what's
in the current diff — but only if the annotated files appear in the diff at
all. The fixture annotates `CLAUDE.md`, `README.md`, and `app/main.go`, so
run it against a diff that touches those files. The easiest is the working
tree against HEAD while you have local changes:

```sh
# inside the revdiff repo, with uncommitted changes touching the annotated files
revdiff --annotations examples/markdown-annotations.md
```

If you have a clean tree, pick a ref range that touches all three files, e.g.
`HEAD~50 HEAD` or any tag-to-tag range that spans broad changes. If a file
isn't in the diff, you'll see a `[WARN] file %q not in diff, dropping
annotation` line on stderr at startup.

Toggle the legacy italic-prose path with `--plain-annotations` to compare
the two renderings side by side:

```sh
revdiff --plain-annotations --annotations examples/markdown-annotations.md
```
