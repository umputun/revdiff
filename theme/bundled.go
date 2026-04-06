package theme

// bundled theme definitions — each constant is a complete theme file string
// with all 21 color keys + chroma-style and comment metadata.

const bundledDracula = `# name: dracula
# description: purple accent, vibrant colors on dark background

chroma-style = dracula
color-accent = #bd93f9
color-border = #6272a4
color-normal = #f8f8f2
color-muted = #6272a4
color-selected-fg = #f8f8f2
color-selected-bg = #bd93f9
color-annotation = #f1fa8c
color-cursor-fg = #50fa7b
color-cursor-bg = #44475a
color-add-fg = #50fa7b
color-add-bg = #1a3a1a
color-remove-fg = #ff5555
color-remove-bg = #3a1a1a
color-modify-fg = #ffb86c
color-modify-bg = #3a2a1a
color-tree-bg = #282a36
color-diff-bg = #282a36
color-status-fg = #282a36
color-status-bg = #bd93f9
color-search-fg = #282a36
color-search-bg = #f1fa8c
`

const bundledNord = `# name: nord
# description: frost blue accent, arctic palette on polar night background

chroma-style = nord
color-accent = #88c0d0
color-border = #4c566a
color-normal = #d8dee9
color-muted = #4c566a
color-selected-fg = #eceff4
color-selected-bg = #88c0d0
color-annotation = #ebcb8b
color-cursor-fg = #a3be8c
color-cursor-bg = #3b4252
color-add-fg = #b4d09a
color-add-bg = #1a3a1a
color-remove-fg = #d06c75
color-remove-bg = #4a1a1a
color-modify-fg = #f0d399
color-modify-bg = #4a3a1a
color-tree-bg = #2e3440
color-diff-bg = #2e3440
color-status-fg = #2e3440
color-status-bg = #88c0d0
color-search-fg = #2e3440
color-search-bg = #ebcb8b
`

const bundledSolarizedDark = `# name: solarized-dark
# description: yellow accent, classic solarized palette on deep teal background

chroma-style = solarized-dark
color-accent = #b58900
color-border = #586e75
color-normal = #839496
color-muted = #586e75
color-selected-fg = #fdf6e3
color-selected-bg = #b58900
color-annotation = #cb4b16
color-cursor-fg = #859900
color-cursor-bg = #073642
color-add-fg = #719e07
color-add-bg = #0a2e0a
color-remove-fg = #dc322f
color-remove-bg = #2e0a0a
color-modify-fg = #b58900
color-modify-bg = #2e2a0a
color-tree-bg = #002b36
color-diff-bg = #002b36
color-status-fg = #002b36
color-status-bg = #b58900
color-search-fg = #002b36
color-search-bg = #cb4b16
`

const bundledCatppuccinMocha = `# name: catppuccin-mocha
# description: pastel colors on dark base, warm and cozy

chroma-style = catppuccin-mocha
color-accent = #89b4fa
color-border = #45475a
color-normal = #cdd6f4
color-muted = #7f849c
color-selected-fg = #1e1e2e
color-selected-bg = #89b4fa
color-annotation = #f9e2af
color-cursor-fg = #a6e3a1
color-cursor-bg = #313244
color-add-fg = #a6e3a1
color-add-bg = #1a3a1a
color-remove-fg = #f38ba8
color-remove-bg = #4a1a1a
color-modify-fg = #fab387
color-modify-bg = #4a3a1a
color-tree-bg = #1e1e2e
color-diff-bg = #1e1e2e
color-status-fg = #1e1e2e
color-status-bg = #89b4fa
color-search-fg = #1e1e2e
color-search-bg = #f9e2af
`

const bundledGruvbox = `# name: gruvbox
# description: warm retro colors on dark background

chroma-style = gruvbox
color-accent = #fabd2f
color-border = #504945
color-normal = #ebdbb2
color-muted = #928374
color-selected-fg = #282828
color-selected-bg = #fabd2f
color-annotation = #fe8019
color-cursor-fg = #b8bb26
color-cursor-bg = #3c3836
color-add-fg = #b8bb26
color-add-bg = #1d3a1a
color-remove-fg = #fb4934
color-remove-bg = #4a1a1a
color-modify-fg = #fabd2f
color-modify-bg = #4a3a1a
color-tree-bg = #282828
color-diff-bg = #282828
color-status-fg = #282828
color-status-bg = #fabd2f
color-search-fg = #282828
color-search-bg = #fe8019
`

// bundledThemes maps theme name to its file content.
var bundledThemes = map[string]string{
	"catppuccin-mocha": bundledCatppuccinMocha,
	"dracula":          bundledDracula,
	"gruvbox":          bundledGruvbox,
	"nord":             bundledNord,
	"solarized-dark":   bundledSolarizedDark,
}
