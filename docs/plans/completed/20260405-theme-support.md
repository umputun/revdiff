# Theme Support for revdiff

## Overview
- Add a theme system that loads complete color palettes by plain filename from `~/.config/revdiff/themes/`
- Each theme defines all 21 color properties plus `chroma-style` in INI format
- Ship 5 bundled themes (catppuccin-mocha, dracula, gruvbox, nord, solarized-dark) copied to disk on first run
- Users select themes via `--theme NAME` flag or `theme` config key
- Users create custom themes via `--dump-theme` (outputs currently resolved colors)
- Default colors remain hardcoded in code; no theme file for default

## Context
- Color configuration: `options.Colors` struct in `cmd/revdiff/main.go:49-71` with 21 fields
- Style creation: `ui.Colors` struct → `newStyles()` in `ui/styles.go`
- Config precedence: CLI flags > env vars > config file > built-in defaults
- Config dir: `~/.config/revdiff/` (already used for `config` and `keybindings` files)
- Existing dump pattern: `--dump-config` and `--dump-keys` for reference

## Development Approach
- **testing approach**: regular (code first, then tests)
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: every task MUST include new/updated tests**
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy
- **unit tests**: required for every task
- theme package: test Load, Dump, List, InitBundled, Parse
- main.go integration: test theme flag handling and precedence

## Solution Overview

### Theme file format
```ini
# name: dracula
# description: purple accent, vibrant colors on dark background

chroma-style = dracula
color-accent = #bd93f9
color-border = #6272a4
color-normal = #f8f8f2
...all 21 color keys...
```

Key names match existing `ini-name` tags. `chroma-style` is a real key (not a comment). Comment lines `# name:` and `# description:` provide metadata for `--list-themes`.

### Precedence (simplified)
`--theme` takes over completely — overwrites all 21 color fields + `chroma-style`, ignoring any `--color-*` flags or env vars. `--theme` + `--no-colors` prints warning and applies theme. Without `--theme`: built-in defaults → config file → env vars → CLI flags.

Theme is loaded in `handleThemes()` after `parseArgs()` completes. The `applyTheme()` function overwrites all opts.Colors fields from the loaded theme.

### Theme init behavior
- First run: if `~/.config/revdiff/themes/` doesn't exist, create it and write 5 bundled theme files
- `--init-themes`: re-create the 5 bundled theme files (always overwrites bundled names), don't touch user-added themes

### New `theme/` package
- `Theme` struct: Name, Description, ChromaStyle, Colors (map[string]string for the 21 keys)
- `Load(name, themesDir)` — reads `<themesDir>/<name>` file, parses INI keys
- `Apply(theme, parser)` — sets theme values on go-flags parser via `Option.Set()` calls
- `Dump(colors, chromaStyle, w)` — writes currently resolved colors as theme file to writer
- `List(themesDir)` — returns available theme names (filenames in themes dir)
- `InitBundled(themesDir)` — writes bundled theme files; always overwrites bundled names, skips non-bundled
- Bundled theme data as string constants in a separate file (`theme/bundled.go`)

### 3 Bundled themes (colors from chroma palettes)
- **dracula**: purple accent (#bd93f9), dark bg (#282a36), green adds (#50fa7b), red removes (#ff5555)
- **nord**: frost blue accent (#88c0d0), polar night bg (#2e3440), sage adds (#a3be8c), aurora red removes (#bf616a)
- **solarized-dark**: yellow accent (#b58900), deep teal bg (#002b36), green adds (#719e07), red removes (#dc322f)

### CLI flags
- `--theme NAME` — load theme from `~/.config/revdiff/themes/<NAME>` (ini-name: `theme`, env: `REVDIFF_THEME`)
- `--dump-theme` — print currently resolved colors as theme file to stdout, exit (no-ini)
- `--list-themes` — print available theme names to stdout, exit (no-ini)
- `--init-themes` — write bundled theme files to themes dir, exit (no-ini)

## Implementation Steps

### Task 1: Create theme package - core types and parsing

**Files:**
- Create: `theme/theme.go`
- Create: `theme/theme_test.go`

- [x] define `Theme` struct with Name, Description, ChromaStyle string fields and Colors map[string]string
- [x] implement `Parse(r io.Reader) (Theme, error)` — reads INI-style theme file, extracts comment metadata (name, description) and key=value pairs
- [x] implement `Dump(colors map[string]string, chromaStyle, name, description string, w io.Writer)` — writes theme file format to writer
- [x] write tests for Parse with valid theme, missing fields, malformed lines
- [x] write tests for Dump verifying output format, roundtrip with Parse
- [x] run tests — must pass before next task

### Task 2: Theme loading and listing

**Files:**
- Modify: `theme/theme.go`
- Modify: `theme/theme_test.go`

- [x] implement `Load(name, themesDir string) (Theme, error)` — constructs path, opens file, delegates to Parse
- [x] implement `List(themesDir string) ([]string, error)` — reads dir entries, returns sorted filenames
- [x] write tests for Load with valid file, missing file, invalid content (use testdata or temp dir)
- [x] write tests for List with empty dir, multiple themes, non-existent dir
- [x] run tests — must pass before next task

### Task 3: Bundled themes and InitBundled

**Files:**
- Create: `theme/bundled.go`
- Modify: `theme/theme.go`
- Modify: `theme/theme_test.go`

- [x] define 3 bundled theme constants with final palettes derived from chroma colors:
  - dracula: purple accent (#bd93f9), dark bg (#282a36), green adds (#50fa7b), red removes (#ff5555), chroma-style: dracula
  - nord: frost blue accent (#88c0d0), polar night bg (#2e3440), sage adds (#a3be8c), aurora red removes (#bf616a), chroma-style: nord
  - solarized-dark: yellow accent (#b58900), deep teal bg (#002b36), green adds (#719e07), red removes (#dc322f), chroma-style: solarized-dark
  - each constant must define all 21 color keys + chroma-style with internally consistent colors (bg/fg contrast)
- [x] implement `InitBundled(themesDir string) error` — creates dir if needed, writes each bundled theme file; always overwrites files matching bundled names, doesn't touch user-added files
- [x] implement `BundledNames() []string` — returns list of bundled theme names
- [x] write tests for InitBundled: creates dir, writes files, doesn't overwrite user themes, re-creates bundled themes on --init-themes
- [x] write tests for BundledNames
- [x] write tests verifying each bundled theme constant parses correctly via Parse
- [x] run tests — must pass before next task

### Task 4: Integrate theme into CLI and config

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] add `Theme` field to options struct (`--theme`, ini-name `theme`, env `REVDIFF_THEME`)
- [x] add `DumpTheme`, `ListThemes`, `InitThemes` bool fields (no-ini, exit-early flags)
- [x] implement `resolveThemeName(args)` helper (same pattern as `resolveConfigPath`) to extract theme name from CLI args before full parsing
- [x] in `parseArgs()`: after `loadConfigFile()` and before `p.ParseArgs(args)`, resolve theme name; if set, call `theme.Load()` then `theme.Apply()` to set values on the go-flags parser via `Option.Set()` — this ensures CLI args naturally override theme values
- [x] handle `--init-themes`: call `theme.InitBundled()`, print result, exit
- [x] handle `--list-themes`: call `theme.List()`, print names, exit
- [x] handle `--dump-theme`: collect resolved colors from opts, call `theme.Dump()`, exit
- [x] auto-init: in `parseArgs()` before theme loading, call `theme.InitBundled()` if themes dir doesn't exist (silent, no error on failure)
- [x] skip theme loading when `--no-colors` is set (check args for `--no-colors` before theme load)
- [x] write tests for parseArgs with --theme flag
- [x] write tests for --dump-theme output format
- [x] write test for precedence: set theme with color-accent=#aaa, pass --color-accent=#bbb on CLI, verify opts.Colors.Accent == #bbb
- [x] write tests for --list-themes output
- [x] run tests — must pass before next task

### Task 5: Vendor and verify

**Files:**
- No new dependencies expected (theme package uses only stdlib)

- [x] run `go mod vendor` if any dependencies changed
- [x] run `make test` — all tests pass
- [x] run `make lint` — no lint issues
- [x] run `make fmt` — no formatting issues
- [x] manual smoke test: `go run ./cmd/revdiff --theme dracula`, `--list-themes`, `--dump-theme`, `--init-themes`

### Task 6: Verify acceptance criteria
- [x] verify `--theme NAME` loads theme and applies colors
- [x] verify `--dump-theme` outputs currently resolved colors (not just defaults)
- [x] verify `--list-themes` shows bundled themes after first run
- [x] verify `--init-themes` re-creates bundled themes without touching user files
- [x] verify precedence: theme colors are overridden by explicit `--color-*` flags
- [x] verify `--no-colors` skips theme loading
- [x] verify theme + chroma-style are applied together
- [x] run full test suite: `make test`

### Task 7: [Final] Update documentation
- [x] update README.md with theme usage section (--theme, --dump-theme, --list-themes, --init-themes)
- [x] update CLAUDE.md if new patterns discovered
- [x] update `.claude-plugin/skills/revdiff/references/config.md` with theme options
- [x] move this plan to `docs/plans/completed/`

## Adjustment: Simplify theme precedence

Design change after initial implementation: `--theme` should take over completely instead of participating in a precedence chain with `--color-*` flags. This eliminates the complex `Option.Set()` / `Apply()` dance inside `parseArgs()`.

**New rules:**
- `--theme` overwrites all 21 color fields + `chroma-style`, ignoring any `--color-*` flags
- `--theme` suppresses `--no-colors` with warning: "warning: --no-colors ignored when --theme is set"
- No `--theme` = current behavior unchanged

### Task 8: Simplify theme loading — move from parseArgs to run

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] remove `Apply()` call from `parseArgs()` / `loadTheme()` — theme no longer participates in go-flags precedence
- [x] move theme loading to `run()`: after `parseArgs()`, if `opts.Theme` is set, call `theme.Load()` and overwrite all `opts.Colors` fields + `opts.ChromaStyle` unconditionally
- [x] if `opts.Theme` is set and `opts.NoColors` is true, print warning to stderr and set `opts.NoColors = false`
- [x] remove `resolveThemeName()` helper (no longer needed — go-flags resolves theme name normally)
- [x] remove `loadTheme()` function
- [x] simplify `parseArgs()` — remove theme-related logic between config file load and CLI parse
- [x] keep auto-init of themes dir (can stay in `run()` or `main()`)

### Task 9: Remove theme.Apply and update tests

**Files:**
- Modify: `theme/theme.go`
- Modify: `theme/theme_test.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] remove `Apply(t Theme, p *flags.Parser) error` function from theme package
- [x] remove `go-flags` import from theme package (should be stdlib-only now)
- [x] remove tests for `Apply`
- [x] update/add test: `--theme` overrides all `--color-*` flags completely
- [x] update/add test: `--theme` + `--no-colors` prints warning to stderr and uses theme colors
- [x] remove old precedence test (theme overridden by CLI color flags — that behavior is gone)
- [x] run tests — must pass before next task

### Task 10: Add helper to apply theme colors to opts

**Files:**
- Modify: `cmd/revdiff/main.go`
- Modify: `cmd/revdiff/main_test.go`

- [x] add `applyTheme(opts *options, th theme.Theme)` helper in main.go that overwrites all opts.Colors fields + opts.ChromaStyle from theme
- [x] write test for `applyTheme` verifying all 21 fields + chroma-style are set
- [x] run `make test`, `make lint`, `make fmt`

### Task 11: Vendor and final verify
- [x] run `go mod vendor` (go-flags import removed from theme package)
- [x] run `make test` — all tests pass
- [x] run `make lint` — no lint issues
- [x] manual smoke test: `--theme dracula`, `--theme dracula --color-accent=#fff` (accent should be dracula's), `--theme dracula --no-colors` (warning printed, colors applied)

## Post-Completion

**Manual verification:**
- visual inspection of each bundled theme in a real terminal
- test theme switching between dracula, nord, solarized-dark
- verify --dump-theme output can be saved as custom theme and loaded back
- verify colors look good with syntax highlighting enabled
- verify --theme ignores --color-* flags
- verify --theme + --no-colors prints warning and uses theme
