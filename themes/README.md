# Contributing Themes

Community-contributed color themes for revdiff. Each file in `gallery/` is a complete theme that ships with the binary and can be installed by users.

## Theme Format

Themes use INI-style key=value pairs with comment metadata:

```ini
# name: my-theme
# description: one-line description of the palette
# author: Your Name

chroma-style = dracula
color-accent = #bd93f9
color-border = #6272a4
color-normal = #f8f8f2
color-muted = #6272a4
color-selected-fg = #f8f8f2
color-selected-bg = #44475a
color-annotation = #f1fa8c
color-cursor-fg = #282a36
color-cursor-bg = #f8f8f2
color-add-fg = #50fa7b
color-add-bg = #2a4a2a
color-remove-fg = #ff5555
color-remove-bg = #4a2a2a
color-modify-fg = #ffb86c
color-modify-bg = #3a3a2a
color-tree-bg = #21222c
color-diff-bg = #282a36
color-status-fg = #f8f8f2
color-status-bg = #44475a
color-search-fg = #282a36
color-search-bg = #f1fa8c
```

### Metadata

- `# name:` — theme name, must match the filename exactly
- `# description:` — one-line description of the color palette
- `# author:` — your name (required for community themes)
- `# bundled: true` — used for bundled themes; contributors should not set this for community themes

### Color Keys

Gallery themes must include all required color keys. Optional keys are `color-cursor-bg`, `color-tree-bg`, and `color-diff-bg`. Colors must be 6-digit hex (`#RRGGBB`).

| Key | Controls |
|-----|----------|
| `color-accent` | Active pane borders and directory names |
| `color-border` | Inactive pane borders |
| `color-normal` | File entries and context lines |
| `color-muted` | Line numbers and dimmed text |
| `color-selected-fg` | Selected file text color |
| `color-selected-bg` | Selected file background |
| `color-annotation` | Annotation text and markers |
| `color-cursor-fg` | Diff cursor indicator color |
| `color-cursor-bg` | Diff cursor background |
| `color-add-fg` | Added line text |
| `color-add-bg` | Added line background |
| `color-remove-fg` | Removed line text |
| `color-remove-bg` | Removed line background |
| `color-modify-fg` | Modified line text (collapsed mode) |
| `color-modify-bg` | Modified line background (collapsed mode) |
| `color-tree-bg` | File tree pane background |
| `color-diff-bg` | Diff pane background |
| `color-status-fg` | Status bar foreground |
| `color-status-bg` | Status bar background |
| `color-search-fg` | Search match foreground |
| `color-search-bg` | Search match background |

### Chroma Style

`chroma-style` sets the syntax highlighting palette. Run `revdiff --dump-config` to see available styles, or browse [chroma styles](https://xyproto.github.io/splash/docs/). Pick one that complements your theme's background colors.

## Creating a Theme

The fastest way to start:

```bash
# dump current colors as a theme file
revdiff --dump-theme > themes/gallery/my-theme

# edit the file — update metadata, adjust colors
$EDITOR themes/gallery/my-theme

# preview your theme on a sample diff
revdiff --theme my-theme
```

### Tips

- **Backgrounds**: `tree-bg` and `diff-bg` should match or be close to each other for a cohesive look. The status bar (`status-bg`) often uses the accent color.
- **Contrast**: ensure `add-fg`/`add-bg`, `remove-fg`/`remove-bg`, and `modify-fg`/`modify-bg` pairs have sufficient contrast for readability.
- **Search**: `search-fg`/`search-bg` should stand out clearly against the diff background.
- **Cursor**: `cursor-fg` should be visible against both `cursor-bg` and the diff background.

## Submitting

1. Add your theme file to `themes/gallery/` (no file extension)
2. Validate locally: `make validate-themes`
3. Open a PR — CI validates automatically
4. Include a screenshot of your theme in the PR description (appreciated but not required)

### PR Checklist

- [ ] Filename matches `# name:` in the file
- [ ] `# author:` is set
- [ ] `# bundled: true` is NOT set
- [ ] All 18 required color keys present with valid `#RRGGBB` values (3 optional: `color-cursor-bg`, `color-tree-bg`, `color-diff-bg`)
- [ ] `chroma-style` is set to a valid chroma style name
- [ ] `make validate-themes` passes

## Installing Themes

Users can install themes from the gallery or from local files:

```bash
# list all available themes (gallery + installed)
revdiff --list-themes

# install a gallery theme by name
revdiff --install-theme tokyo-night

# install a local theme file (path with /)
revdiff --install-theme ./my-theme
revdiff --install-theme ~/themes/my-theme

# install all gallery themes at once
revdiff --init-all-themes

# use an installed theme
revdiff --theme tokyo-night
```

Local file install validates the theme (format, colors, required keys) before copying it to `~/.config/revdiff/themes/`.
