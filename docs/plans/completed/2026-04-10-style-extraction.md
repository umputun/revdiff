# app/ui/style/ Sub-Package Extraction (Redesigned)

## Overview

Extract a new `app/ui/style/` sub-package from `app/ui/` that owns **all color and style resolution** for the TUI. This is a **restart** of an earlier failed attempt on branch `ui-subpackage-extraction` — the scope and API are deliberately different. Only `style` is extracted in this plan; `filetree`, `mdtoc`, and `worddiff` are explicitly out of scope and will be addressed in follow-up plans (which should reuse the design principles established here).

**Problem it solves:**

- The current `app/ui/` is a god-package: `Model` holds 20+ methods that resolve colors, produce ANSI sequences, build lipgloss styles, and parse SGR state. Color knowledge leaks into every renderer (every call site reads `m.styles.colors.AccentFg` and turns it into ANSI with `m.ansiFg(...)` on the spot).
- The earlier extraction attempt (`ui-subpackage-extraction`) did the mechanical move but kept the same shape — standalone functions, public `Colors` field, receiver-less helpers, missing tests for `ansi.go`. It was pragmatic ("what is easy to move") rather than principled.
- **The real goal**: make the `style` package the single source of truth for *anything color or style related*. UI code should express intent ("give me the fg for the accent role") and never touch hex strings or build ANSI sequences by hand.

**Why this matters beyond style/:**

This plan establishes **design principles and patterns** to reuse for the three follow-up sub-package extractions (`filetree`, `mdtoc`, `worddiff`). If this extraction gets the shape right — named types, parameterized accessors, consumer-side interfaces, milestone-based test gating — the other three become template work rather than original design work. Read the "Design Philosophy" and "Alternatives Considered" sections before planning any follow-up.

## Context (from discovery)

**Current state of affected code** (on master):

- `app/ui/styles.go` — 360 LOC. Defines `Colors`, `styles`, `newStyles`, `plainStyles`, `coloredText`, `coloredTextWithReset`, `hexColorToRGB`, `hexVal`, `normalizeColors`, `normalizeColor`, and several private lipgloss builders (`contextStyle`, `dirEntryStyle`, `fileEntryStyle`, `treeItemStyle`, `lineNumberStyle`, `contextHighlightStyle`). The `styles` struct has ~24 exported lipgloss.Style fields and one method (`fileStatusFg`).
- `app/ui/colorutil.go` — 126 LOC. HSL color math (`shiftLightness`, `parseHexRGB`, `rgbToHSL`, `hslToRGB`, `hueToRGB`).
- `app/ui/view.go` — defines `ansiColor`, `ansiFg`, `ansiBg` as methods on `Model` (receiver unused — they only need the `hex` arg). Also `effectiveStatusFg` which resolves StatusFg || Muted fallback.
- `app/ui/diffview.go` — defines `changeBgColor(ChangeType)`, `indicatorBg(lineBg)`, `annotationInline(text)`, `diffCursorCell()`, plus the 7 SGR-parsing methods (`reemitANSIState`, `scanANSIState`, `parseSGR`, `applySGR`, `buildSGRPrefix`, `isFgColor`, `isBgColor`).
- `app/ui/model.go:51` — `styles styles` field on `Model`. Construction at `model.go:195-197` via `newStyles(cfg.Colors)` / `plainStyles()`.
- `app/main.go:407` — `ui.Colors{...}` literal fed into `ModelConfig`.

**Files with call sites requiring migration** (9 files, ~35 production call sites + many tests):

| File | Sites touching color helpers or `m.styles.colors.X` |
|---|---:|
| `app/ui/diffview.go` | 25 |
| `app/ui/view.go` | 10 |
| `app/ui/themeselect.go` | 9 |
| `app/ui/annotlist.go` | 8 |
| `app/ui/collapsed.go` | 5 |
| `app/ui/handlers.go` | 4 |
| `app/ui/annotate.go` | 3 |
| `app/ui/filetree.go` | 3 |
| `app/ui/styles.go` | (being deleted) |

Plus `app/ui/*_test.go` files that exercise styling or build `styles{}` literals — need mechanical updates.

**Old branch reference:** `ui-subpackage-extraction` (5 commits from merge-base `8bd3973`). Readable via `git show ui-subpackage-extraction:app/ui/style/style.go` etc. **Useful only for content** (field list of `Colors`, HSL math, lipgloss builder bodies). **Do NOT copy its public API** — it had the problems this redesign fixes.

**Dependencies to add:**

- `github.com/go-pkgz/enum` — code generator, no runtime dep. Added to `go.mod` (and `go mod vendor` since revdiff uses vendoring).

## Development Approach

- **testing approach**: Regular — implementation first, then tests for each unit. Existing HSL tests and lipgloss-construction tests from the old branch are readable via git for reference content.
- **Complete each milestone fully before moving to the next**. Individual tasks within a milestone may leave the codebase in an intermediate broken state — that's allowed and expected. See the "Testing Policy" section below.
- **Small focused changes within each task**. No bundling unrelated changes.
- **CRITICAL: update this plan file when scope changes during implementation**.

## Testing Policy — CRITICAL: Milestone Gating, Not Per-Task

**This section deliberately overrides the default `/planning:make` policy of "all tests must pass before starting next task".**

This refactor is a single coordinated extraction that touches ~35 production call sites and a similar number of tests. It is **structurally impossible** for every intermediate task to leave `go test ./... -race` green, because:

- The new `style.Resolver`, `style.Renderer`, and `style.SGR` types must exist before any call site can migrate to them.
- `Model` methods (`ansiFg`, `changeBgColor`, etc.) must be deleted before the new API can fully replace them.
- Between those two points, some call sites call the new API and some still call the old — neither the old nor the new is a consistent state.

Forcing green at every task would either (a) require per-call-site migration commits that are too granular to be useful, or (b) make individual tasks absurdly large.

**The correct gating is per-milestone**. Three milestones, each with a specific green-gate:

| Milestone | What's built | Green-gate |
|---|---|---|
| **M1: Style package skeleton** | New `app/ui/style/` package complete and self-contained: types, enums, methods, tests. UI package is NOT yet migrated. | `go test ./app/ui/style/... -race` green + linter clean for `app/ui/style/`. **Full project build may be broken** — that's expected. |
| **M2: Call-site migration** | All ~35 production call sites + test files updated to new API. `Model` methods deleted. Old `app/ui/styles.go`, `colorutil.go` deleted. | Full `go test ./... -race` green. `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` clean. |
| **M3: Interfaces + cleanup** | Three consumer-side interfaces (`resolver`, `renderer`, `sgrProcessor`) in `app/ui/deps.go`, three moq mocks generated, documentation updated. | Same gate as M2 + mocks rebuild cleanly + manual smoke test. |

**Non-milestone tasks do NOT have "run tests — must pass" checkboxes.** They may note "may leave N tests failing until task M" or say nothing about tests at all. The milestone task is where convergence happens.

**Commit structure**: one git commit per milestone (3 total). Each milestone commit is a coherent unit that leaves the tree green (M2/M3) or at least leaves `style/` green (M1).

**If `/planning:exec` or human executor hits failing tests inside a non-milestone task — that is expected. Proceed to the next task. Only escalate if a MILESTONE task leaves failures.**

## Design Philosophy (Reusable for Future Extractions)

**These principles should guide follow-up extractions of `filetree`, `mdtoc`, and `worddiff`.** They emerged from brainstorming the style extraction and are not style-specific.

### 1. Sub-packages own their domain completely

The extracted sub-package should **own all knowledge** of its domain, not just hold data for the caller to manipulate. For `style/`, that means: all hex values, all ANSI sequence construction, all lipgloss style construction, all SGR parsing, all semantic color resolution logic — every concern that is *about* styling — lives in `style/`. Callers express intent ("give me the accent color") and never touch implementation details.

Rule of thumb: if a caller has to know the **representation** of a domain value (hex string, ANSI sequence, lipgloss field name), the sub-package is leaking.

### 2. Named types for domain values

Return types like `string` or `int` tell the reader nothing. A `Color` type (`type Color string`) communicates intent at every signature. Named types cost nothing at runtime and document the code in-place. Apply the same principle to keys, IDs, and any value that has semantic meaning beyond its underlying type.

### 3. Parameterized accessors over per-role method explosion

Instead of `s.AccentFg()`, `s.MutedFg()`, `s.AddLineFg()`, ..., use `s.Color(k ColorKey)`. Reasons:
- Interface surface stays small (one method per category instead of 20+).
- Adding a new role doesn't change the interface — just add an enum value and a case in the switch.
- Consumer-side mocking requires far less boilerplate.
- Enum-based exhaustiveness tests become possible (iterate all enum values, verify every one is handled).

Trade-off: call sites become slightly wordier (`s.Color(style.ColorKeyAccentFg)` vs `s.AccentFg()`). Acceptable cost for the other benefits.

### 4. Methods over standalone functions — applies to ALL helpers, not just the public API

If a function takes a type `T` as its primary (or only) operand, **it must be a method on `T`**, even if:
- the function is private (package-scoped)
- the receiver is unused in the body
- the function is a construction helper called only from a constructor

This rule applies uniformly to public API AND private helpers. The CLAUDE.md rule "Prefer methods over standalone utility functions" is absolute for this case. Standalone helper functions that operate on a single type are a design smell — they're methods wearing a disguise.

**Concrete example from this refactor's implementation** (documented as a lesson learned): the initial plan drafted `contextStyle(c Colors) lipgloss.Style`, `dirEntryStyle(c Colors) lipgloss.Style`, etc. as standalone private functions in `resolver.go`. These all take `Colors` as their primary operand. They should have been methods on `Colors` from the start: `(c Colors) contextStyle() lipgloss.Style`, etc. The standalone form was corrected mid-implementation (see commit `4e866a1`). **Private methods on a public data type do NOT bloat the public API** — they're invisible to external callers because they're lowercase. `Colors` remains a pure data struct externally while gaining discoverable, idiomatic methods internally.

**Rule for private helpers**: if your helper takes a type `T` as an argument (with or without extra params), make it a method on `T`. `f(t T, ...) X` becomes `(t T) f(...) X`. This applies to standalone functions, constructor helpers, builder helpers, and anything else.

**Exception (narrow)**: a function is standalone-justified ONLY when it serves as a pure adapter between two types, neither of which is the natural owner. Example: `style.Write(w io.Writer, c Color) (int, error)` bridges the `Color` domain to the `io.Writer` domain — `Color` and `io.Writer` are both peers, not a type-and-its-operation. `(c Color) WriteTo(w io.Writer)` would also be valid (stdlib `io.WriterTo` idiom), but the D10 decision picked the free function form.

**Applies to public API with unused receiver too**: if a function is conceptually part of a type's API (e.g., `SGR.Reemit(lines)` on a stateless `SGR` type), making it a method enables consumer-side interface wrapping. A struct with methods is useful even when the receiver is unused — it groups related functionality under a type, makes the API discoverable, and lets the consumer define an interface against it for mocking.

**Lesson**: "helper that operates on type T" and "method on T" are the same thing in Go. Treat them as such. Don't introduce a standalone function just because the logic "feels like a utility" — if it takes a type, it's a method on that type.

### 5. Consumer-side interfaces, not provider-side

Define the interface in the package that **consumes** the type, not the package that **provides** it. This is idiomatic Go and keeps the sub-package free of interface definitions that exist only for mocking purposes. For style, the `styler` interface lives in `app/ui/deps.go`, not in `app/ui/style/`.

### 6. Avoid splitting related data and behavior into separate files for the sake of "cleanness"

If `Colors`, `Service`, `New`, and lipgloss builders are all about "style construction", they belong in `style.go`. Don't split into `colors.go`, `service.go`, `new.go`, `builders.go` — that's fragmentation disguised as organization. Split by **concern** (ANSI sequences vs HSL math vs SGR parsing vs core types), not by "types should be in their own files".

### 7. Milestone-based test gating for coordinated refactors

For single-coordinated refactors that touch many call sites, per-task green gates force artificial commit gymnastics. Use milestone gates (as this plan does) and explicitly document it. Don't fight the reality that intermediate states are broken — embrace it and converge at boundaries.

### 8. Enum generator for type-safe keys

For finite sets of keys (color roles, style roles), use `go-pkgz/enum` rather than plain iota constants. Benefits: compile-time type safety (can't accidentally pass a `StyleKey` to a method expecting `ColorKey`), `String()` method for debugging, `Values` slice for exhaustiveness tests, case-insensitive parsing if ever needed. Cost: slightly verbose generated constants (`ColorKeyAccentFg` vs `AccentFg`), one new dev dependency. For 10+ enum values across 2+ enums, the generator wins.

### 9. Comprehensive test coverage from day one — no "will add tests later"

Every file with executable code gets a matching `_test.go` file. Missing tests are a bug. The old branch shipped `ansi.go` without `ansi_test.go`; this plan fixes that and the same rule applies to future extractions.

## Design Decisions (Locked)

Each decision here has a "why" because future extractions should understand the reasoning, not just the conclusion.

### D1: Package name is `style` (singular); types are `Resolver`, `Renderer`, `SGR`

- **Decision**: `app/ui/style/` package, with three public types (`Resolver`, `Renderer`, `SGR`) per D15. There is NO single aggregate type and no wrapper container. Construction is done via three separate constructors: `NewResolver(c Colors) Resolver`, `PlainResolver() Resolver`, `NewRenderer(res Resolver) Renderer`. `SGR` is zero-value usable — callers just write `style.SGR{}`. Main.go wires the three types into `ModelConfig`.
- **History**: an earlier draft had a single `Service` type with 13 methods covering all concerns. It was split during plan review (see D15) because `ReemitANSIState` had zero coupling to colors/lipgloss and compound renderers are a different abstraction level from pure getters — packaging all four concerns on one type was arbitrary, not cohesive.
- **Why not `Styles`?**: `style.Styles` is visually redundant. `Service` was an intermediate choice (rejected during D15 as a god object).
- **Why not `Theme`?**: `app/theme/` already exists (it parses theme `.ini` files from disk). Name collision at the concept level.
- **Why not `Set`?** (the old branch's name): too vague.
- **For follow-up extractions**: if a sub-package has multiple distinct concerns (lookups + assembly + stream processing), split into multiple narrow types rather than one aggregate. One name per concern is clearer than a "does-it-all" god object.

### D2: All three types have all-private fields; `Colors` is construction input only

- **Decision**: `type Resolver struct { /* private */ }`, `type Renderer struct { /* private */ }`, `type SGR struct{}`. No public `Colors` accessor on any of them. The `Colors` struct stays public as the input to `New(c Colors)` — it's a configuration struct, not part of any type's runtime API.
- **Why**: the core goal is "UI never sees hex". If any type exposed `Colors` as a public field, call sites would still be reading `m.resolver.Colors.AccentFg` and the refactor would fail its purpose. Making `Colors` a construction-only input forces all color access to go through `resolver.Color(k)`.

### D3: `Color` is a named type (`type Color string`)

- **Decision**: `type Color string` with NO methods initially. `String()` and `IsEmpty()` were considered and CUT during plan review per the "no code without a caller" rule: `fmt.Printf("%s", c)` already prints the underlying string value because `Color` is a string type alias (Stringer is not needed), and `color != ""` works directly for emptiness checks (no `IsEmpty()` needed). If a specific call site later emerges that benefits from either method, add it at that time — not speculatively.
- **Why the named type at all**: `Color(k) string` tells the reader nothing about what the string represents. `Color(k) Color` makes the domain explicit at every signature. This is the entire value — not runtime behavior, but static documentation at the type level.
- **Trade-off**: `strings.Builder.WriteString(c)` requires `string(c)` cast because `Builder.WriteString` takes a plain string and there's no implicit conversion. Mitigated by the `style.Write(w, c)` helper (D10) which takes `io.Writer` + `Color` and handles the conversion.
- **Reset constants**: `ResetFg Color = "\033[39m"` and `ResetBg Color = "\033[49m"` as package-level typed constants. Theme-independent, don't need to go through `Resolver`.

### D4: Parameterized accessors, not per-role methods

- **Decision**: `Color(k ColorKey) Color` and `Style(k StyleKey) lipgloss.Style` — one accessor per category on `Resolver`, parameterized by a typed key. The D15 field rename (`styles` → `resolver`) eliminated an earlier collision concern that had briefly motivated a `Lipgloss(k)` name; see D7 for the history.
- **Why not per-role methods** (`s.AccentFg()`, `s.MutedFg()`, `s.LineAdd()`, ...): 40+ methods, interface surface too big, adding a new role changes the interface, exhaustiveness tests impossible.
- **Why not unified `Get(k any)`**: loses type safety, return type ambiguous.
- **Two separate enums (ColorKey / StyleKey)**: different return types (`Color` vs `lipgloss.Style`), so they must be separate typed keys.
- **Intentional key-name ↔ Colors-field-name mismatch**: `ColorKeyDiffPaneBg` vs `Colors.DiffBg`, `ColorKeyAddLineBg` vs `Colors.AddBg`. The enum describes *what the color is for* (diff pane background, add line background); the `Colors` struct field describes *what it contains* (the hex). Both sides are correct for their level of abstraction; don't rename either to match the other. The `Color(k)` switch handles the mapping internally. Add a comment above the `colorKey` iota declaration in `enums.go` documenting this so future maintainers don't try to "fix" it.
- **Non-getter semantic keys**: not every `ColorKey` has a 1:1 mapping to a `Colors` field. For example, `ColorKeyStatusFg` returns `Colors.StatusFg` when set, otherwise falls back to `Colors.Muted` — the key encodes a resolution rule, not just a field lookup. Future keys may encode similar rules (derived/computed colors). Tests must exercise both the populated path and the fallback path (see "Testing Strategy" for the two-fixture exhaustiveness pattern).

### D5: Runtime dispatchers alongside static accessors

- **Decision**: keep separate methods for `diff.ChangeType`-dispatched colors/styles: `LineBg(change)`, `LineStyle(change, highlighted)`, `WordDiffBg(change)`, `IndicatorBg(change)`.
- **Why not fold into `Color(k)`**: runtime dispatch requires a parameter, and `Color(k)` is single-arg by design. Making it variadic or overloaded fights the clean single-entry-point design.
- **Why put dispatch in style, not in ui**: otherwise UI code does `switch change { case ChangeAdd: s.Color(ColorKeyAddLineBg) }` at every renderer — that's the color-resolution-in-ui leak we're trying to eliminate.

### D6: Compound operations return pre-rendered strings

- **Decision**: high-level operations like `AnnotationInline(text)`, `DiffCursor(noColors)`, `FileReviewedMark()` return ready-to-print `string` (not `Color`), because they're fully assembled ANSI spans with multiple sequences (fg + bg + italic + text + resets).
- **Why**: calling them a `Color` would be a lie — they're multi-sequence renders. `string` is honest.

### D7: lipgloss styles become `Style(k StyleKey)` method on `Resolver` — NOT public fields

- **Decision**: the ~30 pre-built lipgloss.Style values become enum-keyed method access via `Style(k StyleKey) lipgloss.Style` on the `Resolver` type (see D15 for the decomposition from the earlier Service shape into `Resolver` + `Renderer` + `SGR`). Call sites read `m.resolver.Style(style.StyleKeyLineAdd)`. No public lipgloss fields.
- **Why parameterized access at all**: consistency with `Color(k)` on the same `Resolver`. A single lookup pattern works for both colors and styles, composes cleanly with the consumer-side `resolver` interface, and scales to ~30 keys without inflating the interface with 30 separate getter methods.
- **Method name `Style` (not `Lipgloss`)**: an intermediate draft renamed this to `Lipgloss(k)` because `m.styles.Style(style.StyleKey...)` had three "style" tokens (field + method + enum prefix) which was visually noisy. After D15 split the concerns into three types, the Model field for lookups is `resolver`, not `styles` — so `m.resolver.Style(style.StyleKeyLineAdd)` only has one "style" token left (in the enum prefix). The method name was reverted back to the obvious `Style(k)`. `LineLipgloss(...)` was similarly reverted to `LineStyle(...)`.
- **Trade-off**: call sites are still slightly wordier than raw field access would be (`resolver.Style(style.StyleKeyLineAdd).Render(...)` vs a hypothetical `resolver.LineAdd.Render(...)`). Accepted for consistency with `Color(k)` and the narrow 6-method `resolver` interface.
- **Rejected alternative**: expose lipgloss styles as public fields on `Resolver` (bubbles library convention). Would give slightly cleaner call sites but inflate `Resolver` to 30+ public fields and inflate the consumer `resolver` interface with 30 getter methods — worse both for the type shape and the interface contract. D15's field rename from `styles` to `resolver` made the parameterized method cleaner at the call site, eliminating the main motivation for public fields.

### D8: Use `go-pkgz/enum` for `ColorKey` / `StyleKey`

- **Decision**: generate both enums via `go-pkgz/enum`. New dev dep.
- **Why**: exhaustiveness tests (via `ColorKeyValues` slice) catch "new key added without case in Color() switch" bugs. String method for error messages. Project-ecosystem alignment.
- **Cost**: generated constants have type-name prefix (`ColorKeyAccentFg` instead of `AccentFg`) — wordier call sites.
- **Accepted** after weighing: exhaustiveness tests + String + Values iteration + future ecosystem consistency outweighs the ~9 char call-site cost.
- **Acceptable speculative exports from the generator**: `go-pkgz/enum` always generates `ParseXxx`, `MustParseXxx`, `MarshalText`, `UnmarshalText`, `String()`, `Index()`, and `XxxValues` methods on the exported type. Of these, only `XxxValues` (exhaustiveness test) and `String()` (debugging/error messages) have current callers in revdiff. `ParseColorKey`, `MustParseColorKey`, `MarshalText`, `UnmarshalText`, `Index` have no current callers. This technically violates the "no code without callers" rule (Rule 1 of the audit), but it's an **acceptable exception** because: (a) these are generator artifacts, not hand-written code; (b) we cannot opt out of them individually without forking the generator; (c) they're in the generated file (not hand-maintained), cost ~50 lines, zero runtime overhead, zero maintenance burden; (d) any future consumer that needs JSON/text marshaling of keys gets it for free. Document this exception here so future reviewers don't re-flag it.

### D9: SGR parsing moves into `style/sgr.go`

- **Decision**: the 7 SGR methods on `Model` in `diffview.go` move into `app/ui/style/sgr.go`, collapsed into a private `sgrState` struct with methods + `parseSGR`/`isFgColor`/`isBgColor` as private package-level helpers.
- **Why**: those methods touch no Model state. They're pure ANSI manipulation, which is style's domain.
- **Why not standalone funcs in style**: `sgrState` IS a natural type (5 fields tracking active SGR state). Methods on `sgrState` (e.g., `scan`, `prefix`) read better than functions taking 5 params. The three exception helpers (`parseSGR`, `isFgColor`, `isBgColor`) stay as private functions because they operate on input strings, not state — no natural type to attach.
- **Public entry point**: `(SGR) Reemit(lines []string) []string`. Receiver unused (`SGR` is stateless — it exists to provide a method namespace so the consumer-side `sgrProcessor` interface has something concrete to satisfy). Method name shortened from `ReemitANSIState` because the type name `SGR` already carries the "ANSI state" context.

### D10: `style.Write(w io.Writer, c Color)` is a standalone function

- **Decision**: `func Write(w io.Writer, c Color) (int, error)` — free function, not a method on `Resolver`, `Renderer`, or `Color`.
- **Why**: it's an adapter between two types (`Color` and `io.Writer`), neither of which it belongs to naturally. Methodizing it onto `Resolver` / `Renderer` / `Color` would fabricate an attachment.
- **Why `io.Writer`, not `*strings.Builder`**: accept any writer, not just Builder. `strings.Builder` implements `io.Writer`, so Builder still works.
- **This is the *justified exception* to "avoid standalone functions" rule**. The rule isn't absolute; the qualifier is "if attachment makes sense".

### D11: `app/theme/` stays untouched

- **Decision**: do NOT absorb `app/theme/` into `app/ui/style/`.
- **Why**: `app/theme/` handles file I/O (reading `.ini` theme files from `~/.config/revdiff/themes/`). That's a different concern from style materialization. Keeping them separate preserves clean separation: `theme.Load()` returns `Colors` → `main.go` calls `style.NewResolver(colors)` / `style.NewRenderer(res)` / `style.SGR{}` and wires them into `ModelConfig`. `main.go` is the wiring point.

### D12: Three narrow consumer-side interfaces in `app/ui/deps.go` — AND Model uses them as field types

- **Decision**: define **three** interfaces in `app/ui/deps.go` (new file), one per `style` type, matching the D15 three-type split. Each is narrow and independently mockable. **Model AND ModelConfig hold these interface types as field types**, not the concrete `style.Resolver` / `style.Renderer` / `style.SGR`.
  - `styleResolver` — 6 methods: `Color`, `Style`, `LineBg`, `LineStyle`, `WordDiffBg`, `IndicatorBg`. Implemented by `style.Resolver`.
  - `styleRenderer` — 6 methods: `AnnotationInline`, `DiffCursor`, `StatusBarSeparator`, `FileStatusMark`, `FileReviewedMark`, `FileAnnotationMark`. Implemented by `style.Renderer`.
  - `sgrProcessor` — 1 method: `Reemit`. Implemented by `style.SGR`.
- **Why three instead of one**: the three types serve different concerns (lookups, compound assembly, stream processing). A single fat interface would force test code that only cares about one concern to depend on all three. Three narrow interfaces let a test mock just the piece it needs. This matches D15's "single responsibility per type" principle and keeps the test boundary aligned with the production boundary.
- **Why Model holds interface types (NOT concrete types)**: idiomatic Go — "accept interfaces, return concrete types". Model is the consumer; it accepts the interfaces. The provider (`style` package) returns concrete types from constructors (`NewResolver`, `NewRenderer`) which are then assigned to interface-typed fields on Model via ModelConfig. Go handles the concrete-to-interface conversion at assignment with zero ceremony. The overhead of interface dispatch is negligible for a TUI (~1-2ns per call). The win is huge: tests can mock any of the three concerns independently without touching the others, Model is decoupled from the concrete style types, and the interfaces are actually used by production code (not just the compile-time assertion). **An interface that isn't used by any field or function signature is dead weight — it violates Rule 1 (no code without callers).**
- **Generated mocks**: three moq directives produce `app/ui/mocks/style_resolver.go`, `app/ui/mocks/style_renderer.go`, `app/ui/mocks/sgr_processor.go`. Each mock is small because each interface is small. Tests that only care about one concern can construct a Model with a mock for just that field and real `style.NewResolver(...)` / etc. for the others.
- **Compile-time contract enforcement**: `deps.go` contains three assertions that cost nothing at runtime but catch contract violations at compile time (they're redundant with Model using the interfaces as field types, but harmless — keep as defensive documentation):
  ```go
  var (
      _ styleResolver = (*style.Resolver)(nil)
      _ styleRenderer = (*style.Renderer)(nil)
      _ sgrProcessor  = (*style.SGR)(nil)
  )
  ```
- **Model field types (CORRECTED)**:
  ```go
  type Model struct {
      resolver styleResolver   // was: style.Resolver
      renderer styleRenderer   // was: style.Renderer
      sgr      sgrProcessor    // was: style.SGR
      // ...
  }

  type ModelConfig struct {
      Resolver styleResolver   // was: style.Resolver
      Renderer styleRenderer
      SGR      sgrProcessor
      // ...
  }
  ```
  `main.go` still calls `style.NewResolver(...)` / `style.NewRenderer(...)` / uses `style.SGR{}` and assigns those concrete values into the interface-typed `ModelConfig` fields. No cast, no wrapping — Go converts automatically.
- **Historical note**: an earlier draft of D12 specified concrete types on Model with the reasoning "interface dispatch overhead, simplify field init, interfaces are for mockability". This was incoherent — if interfaces exist only for a compile-time assertion with no runtime callers, they're dead code. The correction happened after the plan shipped, prompted by a review during implementation. Lesson: if you define an interface, USE IT at the field level on consumers. Don't create interfaces as documentation-only artifacts. See also Design Philosophy #5 and Rule 1 in the "Rule 1/Rule 2 audit" section.

### D13: Full test coverage — every file gets a `_test.go`

- **Decision**: `style_test.go`, `color_test.go`, `ansi_test.go`, `sgr_test.go`, plus the enum exhaustiveness test inside `style_test.go`.
- **Why**: the old branch shipped `ansi.go` with no test file. That's a gap. Every non-trivial file in `style/` gets tests — no exceptions.

### D14: All lipgloss.Style construction lives in `style/` — no `lipgloss.Color(hex)` in UI

- **Decision**: UI code MUST NOT call `lipgloss.NewStyle().BorderForeground(lipgloss.Color(...))` or any other inline lipgloss construction that touches hex. Every ad-hoc lipgloss style currently built inline in UI files gets a dedicated `StyleKey` and moves into `style.New()` / `Resolver.Style(k)`.
- **Why**: the core principle of this refactor (D2, Design Philosophy #1) is "UI never knows a color was hex". The initial review missed ~12 call sites in `annotate.go:36-49`, `annotlist.go:31`, `themeselect.go:151-156`, `handlers.go:172-176` that build lipgloss styles from raw hex — these would otherwise leak color knowledge into UI even after all `ansiFg`/`ansiBg` calls are migrated.
- **Why not** a `LipColor(k ColorKey) lipgloss.Color` escape hatch: it would preserve encapsulation of *which hex* but leak *the fact that there's color* — UI would still be in the business of constructing lipgloss styles from scratch. That's the wrong seam.
- **Rejected alternative** `LipColor(k) lipgloss.Color` accessor: less invasive but defeats the "sub-package owns its domain" principle. Escape hatches tend to become the default path.
- **Consequence for `StyleKey` enum**: grows from 25 to ~31 with these additions (exact names confirmed in Technical Details):
  - `styleKeyAnnotInputText` — `ti.TextStyle` / `ti.PromptStyle` for annotation input
  - `styleKeyAnnotInputPlaceholder` — `ti.PlaceholderStyle`
  - `styleKeyAnnotInputCursor` — `ti.Cursor.Style` / `ti.Cursor.TextStyle`
  - `styleKeyAnnotListBorder` — annotation list popup border (annotlist.go:31)
  - `styleKeyThemeSelectBox` — theme selector box (themeselect.go:151)
  - `styleKeyThemeSelectBoxFocused` — theme selector focused state (themeselect.go:154-156)
  - `styleKeyHelpBox` — help overlay box (handlers.go:172-176)
  - Grep `handlers.go:172-176` and `themeselect.go:151-156` during M1 Task 1.6 (resolver.go) to confirm the exact styles needed; adjust the list if additional ad-hoc styles are discovered.
- **Consequence for `annotate.go`**: `newAnnotationInput` no longer constructs `inputStyle` from raw colors; instead it assigns `ti.PromptStyle = m.resolver.Style(style.StyleKeyAnnotInputText)`, etc. — 5 field assignments, each fetching from the new enum. This is a signature reshape of textinput setup, not a simple 1:1 ANSI swap. Budget accordingly in Task 2.6.
- **Helper signatures**: `padContentBg(content, bgHex)` in `view.go` and `extendLineBg(styled, bgColor)` in `diffview.go` currently take `bgHex string`. They must migrate to take `bgColor style.Color` (already-resolved ANSI sequence); callers resolve the color via `m.resolver.Color(k)` or `m.resolver.LineBg(change)` / `m.resolver.IndicatorBg(change)` before calling. Internal `ansiBg(bgHex)` calls inside these helpers are replaced by direct use of the resolved Color.

### D15: Decompose `Service` into `Resolver` + `Renderer` + `SGR` (three types)

- **Decision**: there is no single `Service` type. The styling package exposes **three separate types**, each with a focused responsibility:
  - **`Resolver`** — pure lookup/dispatch. 6 methods: `Color(k ColorKey) Color`, `Style(k StyleKey) lipgloss.Style`, `LineBg(change)`, `LineStyle(change, highlighted)`, `WordDiffBg(change)`, `IndicatorBg(change)`. Constructed via `NewResolver(c Colors) Resolver` or `PlainResolver() Resolver`.
  - **`Renderer`** — compound ANSI assembly. 6 methods: `AnnotationInline(text)`, `DiffCursor(noColors)`, `StatusBarSeparator()`, `FileStatusMark(status)`, `FileReviewedMark()`, `FileAnnotationMark()`. Holds a `Resolver` internally to look up colors during assembly. Constructed via `NewRenderer(res Resolver) Renderer`.
  - **`SGR`** — ANSI SGR stream processing. 1 method: `Reemit(lines)`. Stateless — does not depend on `Colors`, `Resolver`, or any other styling state. Zero-value usable: just `style.SGR{}`. No constructor.
  - **No wrapper container type.** No `Bundle`, no `Set`, no aggregate return. `main.go` is the single wiring point and wires the three types directly into `ModelConfig`.

- **Why split (and what "god object" smell we avoided)**: the earlier draft had a single `Service` with 13 methods covering (a) static color/style lookup, (b) runtime dispatch on `diff.ChangeType`, (c) compound ANSI assembly using multiple lookups, and (d) pure ANSI stream parsing. Concerns (a) and (b) belong together (same return types, same underlying state). Concern (c) is a different abstraction level — it's a renderer, not a lookup table. Concern (d) is **completely decoupled** from the others — `ReemitANSIState` doesn't read `Colors` and doesn't use any styling state. Grouping all four on one object was arbitrary convenience, not cohesion. The result would have been a god object by smell even if not by raw method count.

- **Natural seams identified during brainstorm**:
  - SGR has zero coupling to colors → obviously its own type.
  - Compound renderers USE colors but are a different kind of thing from getters → natural split.
  - Color lookup and lipgloss-style lookup share the same construction input (`Colors`) and always co-occur → keep them merged on one type (`Resolver`).

- **Why not 4-way split (separate `Palette` for colors, `Stylesheet` for lipgloss)**: considered. Palette and Stylesheet are always constructed together from the same `Colors` input and always used together at call sites. Splitting them adds coordination cost for marginal gain. Merging into `Resolver` is more pragmatic.

- **Why not 2-way split (just pull SGR out, keep everything else on `Service`)**: considered as a "minimum viable split". The remaining Service would still mix lookups with compound assembly, which is the same smell. Splitting fully into 3 is the principled fix.

- **Model changes**: 3 fields instead of 1. `m.styles → m.resolver, m.renderer, m.sgr`. Accepted — they're conceptually grouped (all about visual presentation), Model already has 40+ fields across other concerns, and 3 clearly-named fields is more honest than 1 god field.

- **Naming win**: the split is what lets us keep `Style(k)` as the method name on Resolver. With `m.resolver` as the field name, `m.resolver.Style(style.StyleKeyLineAdd)` has no field/method collision (only the `StyleKey` enum prefix contains "style"). The `Lipgloss(k)` rename workaround from an earlier draft becomes unnecessary and was reverted.

- **Dependency direction**: one-way. `Renderer` depends on `Resolver` (uses it internally to look up colors). `SGR` depends on nothing. No cycles. Construction order at the caller: `Resolver` first (via `NewResolver` or `PlainResolver`), then `NewRenderer(resolver)`, then `SGR{}` directly. No single `New()` function that wires all three — three separate public constructors keep each concern's construction explicit.

- **Why three constructors instead of one `New(Colors)` returning all three**: considered a `Bundle` wrapper container during brainstorm. Rejected as speculative ceremony — Bundle would exist solely to package three types with zero added value. Three explicit constructors in `main.go` make the dependency flow visible at the call site: "Resolver first, Renderer from Resolver, SGR is free." Adding a fourth type later is a one-line addition in `main.go`, not a breaking change to a shared container return type.

- **Interface contract**: the consumer side gets **three interfaces** (`resolver`, `renderer`, `sgrProcessor`) in `app/ui/deps.go`, one per type. Each is narrow (1-6 methods), each can be mocked independently, and a test that only cares about rendering doesn't need to construct a color lookup mock.

## Alternatives Considered and Rejected

| Alternative | Why rejected |
|---|---|
| Keep the `Set` name from old branch | Too vague — "set of what?" |
| `Styles` as type name | `style.Styles` reads redundant |
| `Theme` as type name | Collides with `app/theme` package |
| `Sheet` as type name | Valid but "Service" was user's preference; low-value bikeshed |
| Plain `iota` enums instead of `go-pkgz/enum` | Loses `Values` slice for exhaustiveness tests |
| Single unified `Get(k any) any` accessor | Loses type safety, return type ambiguous |
| Per-role methods (`AccentFg()`, `MutedFg()`, ...) | 40+ method interface surface, no exhaustiveness tests |
| `Color` as opaque struct | Too much ceremony vs `type Color string` |
| `RawFg(hex)` / `RawBg(hex)` escape-hatch primitives | **Pattern C doesn't exist**: initial grep misread suggested SGR reemit code needed hex-in ANSI primitives; actual read showed it passes ANSI sequences as opaque strings. No hex-in needed anywhere in UI. |
| Absorb `app/theme` into `style` | Couples file I/O with style materialization; different concerns |
| Method on `Service` for `Write`: `(s Service) Write(b, c)` | Receiver meaningless; `Write` isn't a Service operation |
| Method on `Color` for `Write`: `(c Color) WriteTo(w)` | Valid (io.WriterTo idiom) but user picked standalone; don't re-litigate |
| lipgloss styles as public fields (old-branch shape) | Inconsistent with `Color(k)` parameterized access |
| `LipColor(k ColorKey) lipgloss.Color` escape hatch for inline lipgloss construction | Defeats D14 — UI would still construct lipgloss styles from scratch. All lipgloss construction belongs in `style/` via `StyleKey`. |
| Method name `Style(k StyleKey)` on single `Service` (initial draft) | `m.styles.Style(style.StyleKeyLineAdd)` had three "style" tokens. Intermediate fix was to rename method to `Lipgloss(k)`, but the real fix came with D15 (split Service → Resolver + Renderer + SGR), which changed the field name from `styles` to `resolver`. `m.resolver.Style(style.StyleKeyLineAdd)` has no collision, so the method name reverted to the obvious `Style(k)`. |
| Revert D7 and expose lipgloss styles as public fields on `Service` (bubbles-library convention) | Considered as an alternative fix for the `m.styles.Style(...)` collision. Would inflate to ~30 public fields AND inflate the interface with 30 getter methods AND break consistency with `Color(k)`. D15's field rename (`styles` → `resolver`) solved the same visual-collision problem without touching the API shape. |
| Single `Service` type with all 13 methods | Was the original design before D15. Grouped 4 distinct concerns (lookup, runtime dispatch, compound rendering, SGR parsing) on one type — god object smell even if LOC count was modest. Most egregious: `ReemitANSIState` has zero coupling to colors but was bundled anyway. Replaced by 3-type split (D15). |
| 4-way split — separate `Palette` (colors) + `Stylesheet` (lipgloss) | Considered during D15 brainstorm. `Palette` and `Stylesheet` share construction input (`Colors`) and are always co-located at call sites; splitting them adds coordination cost with no clear benefit. Merged into a single `Resolver` type. |
| 2-way split — only pull SGR out, keep everything else on `Service` | Considered as "minimum viable split". Would leave the lookup/render mix on one type, which is the same smell in smaller form. Chose to split fully into 3 (D15) rather than compromise. |
| `Bundle` container struct returned by `func New(c Colors) Bundle` | Considered during the D15 brainstorm and initially adopted. Rejected during a second plan review as pure ceremony — Bundle would exist solely to group three types with zero behavior. Three separate public constructors (`NewResolver`, `PlainResolver`, `NewRenderer`) and direct `style.SGR{}` instantiation gives `main.go` an explicit dependency-flow reading without a wrapper type. |
| Constructor returning three values `func New(c Colors) (Resolver, Renderer, SGR)` | Considered as an alternative to Bundle. Tuple returns are awkward for >2 values and positional ordering is fragile. Rejected in favor of three separate named constructors that each return one type. |
| Private style-builder functions (`contextStyle(c Colors)`, `dirEntryStyle(c Colors)`, etc.) as standalone functions in `resolver.go` | **INITIAL DESIGN MISTAKE, corrected mid-implementation** (commit `4e866a1`). The plan drafted ~6 private helpers that take `Colors` as their primary operand and build derived `lipgloss.Style` values. These were implemented as standalone functions in Task 1.6 and then refactored to methods on `Colors` after Ralphex had completed through Task 2.6. The standalone form violated the CLAUDE.md rule "Prefer methods over standalone utility functions" and the plan's own Design Philosophy #4. Lesson: the rule applies uniformly to private helpers — any function taking a type as its primary operand must be a method on that type, even if it's package-private and called only from one place. Design Philosophy #4 has been strengthened to state this explicitly so future extractions (filetree, mdtoc, worddiff) apply it from the start. |
| Extract multiple sub-packages in one plan (filetree, mdtoc, worddiff) | Old branch tried this and it was "what's easy" rather than principled. Start with `style` alone, get it right, use as template. |
| Per-task test gating | Structurally impossible for coordinated refactor; forces artificial commit gymnastics |

## Call Site Analysis (Why the Public API Shape Is What It Is)

A grep across production UI code before design found ~35 call sites using `ansiFg`, `ansiBg`, `coloredText`, `coloredTextWithReset`, and `hexColorToRGB`. They fall into two patterns:

**Pattern A — static semantic color** (~80% of calls):
```go
m.ansiFg(m.styles.colors.Accent)         → m.resolver.Color(style.ColorKeyAccentFg)
m.ansiBg(m.styles.colors.DiffBg)         → m.resolver.Color(style.ColorKeyDiffPaneBg)
m.ansiFg(m.styles.colors.AddFg)          → m.resolver.Color(style.ColorKeyAddFg)  (via LineFg dispatcher in dispatch case)
coloredTextWithReset(colors.AddFg, "✓", colors.Normal)
                                         → m.renderer.FileReviewedMark()
coloredText(colors.Annotation, " *")     → m.renderer.FileAnnotationMark()
```

**Pattern B — runtime resolution** (~15% of calls):
```go
m.ansiFg(m.effectiveStatusFg())          → m.resolver.Color(style.ColorKeyStatusFg)  (fallback internal to style)
m.ansiBg(m.changeBgColor(changeType))    → m.resolver.LineBg(changeType)
m.ansiBg(m.indicatorBg(lineBg))          → m.resolver.IndicatorBg(change)
```

**No Pattern C exists.** An earlier reading mistakenly claimed that SGR reemit code passed hex to `ansiFg` — actual code (`diffview.go:420-457`) passes pre-assembled escape sequences as strings. `AnsiColor(hex, code)` can therefore be fully private inside `style/` — no exported hex-in primitive is needed anywhere in the public API.

**Pattern D — ad-hoc lipgloss construction from raw hex** (~12 sites, discovered during plan review):

```go
// annotate.go:36-49 — textinput styling
inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))
ti.PromptStyle = inputStyle                                            → ti.PromptStyle = m.resolver.Style(style.StyleKeyAnnotInputText)
ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(c.Muted))
                                                                       → ti.PlaceholderStyle = m.resolver.Style(style.StyleKeyAnnotInputPlaceholder)

// annotlist.go:31, handlers.go:172-176, themeselect.go:151-156 — popup boxes
lipgloss.NewStyle().BorderForeground(lipgloss.Color(c.Accent))         → m.resolver.Style(style.StyleKeyAnnotListBorder)  (or equivalent)
```

These sites do NOT produce ANSI escape sequences — they produce `lipgloss.Style` values for use with `lipgloss.Render()`. The proposed `Color(k) Color` method (which returns an ANSI sequence) is the wrong shape for them. Per D14, every such site gets a dedicated `StyleKey` and all lipgloss construction lives in `style.New()`. See D14 for the list of new style keys and the rationale for not using a `LipColor` escape hatch.

## Technical Details

### Final file layout in `app/ui/style/`

```
app/ui/style/
├── enums.go              — colorKey, styleKey iota decls + go:generate directives (no runtime logic, no test file)
├── color_key_enum.go     — generated by go-pkgz/enum, tracked in repo
├── style_key_enum.go     — generated by go-pkgz/enum, tracked in repo
├── color.go              — Color type (no methods) + ResetFg/ResetBg consts + private HSL helpers (shiftLightness, parseHexRGB, rgbToHSL, hslToRGB, hueToRGB)
├── ansi.go               — Write standalone + private AnsiColor primitive + private hexVal helper
├── resolver.go           — Resolver type + 6 methods (Color, Style, LineBg, LineStyle, WordDiffBg, IndicatorBg) + private newResolver/plainResolver constructors + private lipgloss builders (contextStyle, dirEntryStyle, etc.)
├── renderer.go           — Renderer type + 6 methods (AnnotationInline, DiffCursor, StatusBarSeparator, FileStatusMark, FileReviewedMark, FileAnnotationMark) + NewRenderer(Resolver) constructor + private fileStatusFg helper
├── sgr.go                — SGR type + Reemit method + private sgrState struct with scan/prefix/applySGR methods + private parseSGR/isFgColor/isBgColor helpers
├── style.go              — Colors struct + private normalizeColors helper + package-level doc.go-style comment describing the three-type layout (no constructors here — each type owns its own constructor)
├── color_test.go         — reset consts, HSL math (no methods on Color per D3)
├── ansi_test.go          — Write, AnsiColor, hexVal
├── resolver_test.go      — all 6 Resolver methods + exhaustiveness tests (TestResolver_Color_coversAllKeys, TestResolver_Style_coversAllKeys) + TestResolver_Color_StatusFgFallback + test fixtures (fullColorsForTesting, sparseColorsForTesting)
├── renderer_test.go      — all 6 Renderer methods + private fileStatusFg
├── sgr_test.go           — SGR.Reemit + sgrState.scan/prefix/applySGR + parseSGR + isFgColor/isBgColor
└── style_test.go         — Colors struct + normalizeColors (constructor tests live in resolver_test.go for NewResolver/PlainResolver, renderer_test.go for NewRenderer)
```

Nine source files + 2 generated + 6 test files. Each source file has a matching `_test.go` (the one-test-file-per-source-file rule). `enums.go` has no test file because it contains only type declarations; the generator's output is implicitly tested via the exhaustiveness tests in `resolver_test.go`.

### Enum declarations (`enums.go`)

**Important — how go-pkgz/enum works**: `go-pkgz/enum` is a code generator, not a runtime library. The block below is the *source input* — you write **unexported** iota-based types (`colorKey`, `styleKey`) with unexported constants (`colorKeyUnknown`, `colorKeyAccentFg`, ...). The `//go:generate` directives tell `go generate` to invoke the tool, which produces **separate generated files** (`color_key_enum.go`, `style_key_enum.go`) that export the capitalized names: `ColorKey` type, `ColorKeyUnknown` / `ColorKeyAccentFg` / ... constants, `ColorKeyValues` slice, `String()`, `Index()`, `ParseColorKey`, `MustParseColorKey`, `MarshalText`/`UnmarshalText`/`Value`/`Scan`.

**Nothing outside `enums.go` references the lowercase source names** — call sites and the rest of `style/` use the exported names from the generated file (`style.ColorKeyAccentFg`, `style.ColorKeyValues`, etc.). The lowercase declarations exist solely as generator input. This is the same pattern as `stringer`, `moq`, and other Go code-gen tools. If you see "why does this look like plain iota if we said we're using go-pkgz/enum?" — the answer is: because `go-pkgz/enum` *reads iota blocks*. That IS the usage.

```go
package style

//go:generate go run github.com/go-pkgz/enum@latest -type colorKey
//go:generate go run github.com/go-pkgz/enum@latest -type styleKey

// colorKey is the generator input for ColorKey (see generated color_key_enum.go).
// Names intentionally diverge from Colors struct field names — see D4.
// Never reference colorKey or its constants outside this file; use the generated
// exported ColorKey type and its constants instead.
type colorKey int

const (
    colorKeyUnknown colorKey = iota // zero value sentinel
    colorKeyAccentFg
    colorKeyMutedFg
    colorKeyAnnotationFg
    colorKeyStatusFg       // internal fallback: StatusFg || MutedFg
    colorKeyDiffPaneBg
    colorKeyAddLineBg
    colorKeyRemoveLineBg
    colorKeyWordAddBg
    colorKeyWordRemoveBg
    colorKeySearchBg
)

type styleKey int

const (
    styleKeyUnknown styleKey = iota
    // diff line styles
    styleKeyLineAdd
    styleKeyLineRemove
    styleKeyLineContext
    styleKeyLineModify
    styleKeyLineAddHighlight
    styleKeyLineRemoveHighlight
    styleKeyLineContextHighlight
    styleKeyLineModifyHighlight
    styleKeyLineNumber
    // pane styles
    styleKeyTreePane
    styleKeyTreePaneActive
    styleKeyDiffPane
    styleKeyDiffPaneActive
    // file tree entry styles
    styleKeyDirEntry
    styleKeyFileEntry
    styleKeyFileSelected
    styleKeyAnnotationMark
    styleKeyReviewedMark
    // file status styles
    styleKeyStatusAdded
    styleKeyStatusDeleted
    styleKeyStatusUntracked
    styleKeyStatusDefault
    // chrome styles
    styleKeyStatusBar
    styleKeySearchMatch
    // overlay / popup / input styles — per D14, all inline lipgloss construction moves here
    styleKeyAnnotInputText        // ti.TextStyle / ti.PromptStyle in newAnnotationInput
    styleKeyAnnotInputPlaceholder // ti.PlaceholderStyle
    styleKeyAnnotInputCursor      // ti.Cursor.Style / ti.Cursor.TextStyle
    styleKeyAnnotListBorder       // annotlist.go:31 popup border
    styleKeyHelpBox               // handlers.go:172-176 help overlay box
    styleKeyThemeSelectBox        // themeselect.go:151 theme selector box
    styleKeyThemeSelectBoxFocused // themeselect.go:154-156 focused state
)
```

Generated exports: `ColorKey`, `ColorKeyAccentFg`, ..., `ColorKeyValues`, `String()`, `ParseColorKey`. Same pattern for `StyleKey`. Final `styleKey` count: **32 entries** (1 sentinel + 24 core + 7 ad-hoc popup/input styles from D14). The 7 D14 ad-hoc keys are the canonical list in D14 itself (`styleKeyAnnotInputText`, `styleKeyAnnotInputPlaceholder`, `styleKeyAnnotInputCursor`, `styleKeyAnnotListBorder`, `styleKeyHelpBox`, `styleKeyThemeSelectBox`, `styleKeyThemeSelectBoxFocused`) — do not duplicate the list elsewhere. Confirm exact list during M1 Task 1.6 (resolver.go) by grepping for every `lipgloss.NewStyle()` call in `app/ui/*.go` and adding a `styleKey` for each that builds a style from `m.styles.colors.X`. Adjust the enum if additional ad-hoc styles are discovered.

**Verified** `go-pkgz/enum` pattern against `~/.claude2/skills/knowledge-base/references/enum.md`: unexported source type with iota constants is correct; the generator capitalizes the first letter of the type and each constant to produce `ColorKey` + `ColorKeyAccentFg` etc. Output filename is `<snake_type>_enum.go` (`color_key_enum.go`). The generated file contains the exported type, `String()`, `Index()`, `ParseXxx`, `MustParseXxx`, `XxxValues`, and text/SQL marshaling methods.

**Note on enum naming vs `Colors` field names**: the `ColorKey` enum uses semantic role names (`DiffPaneBg`, `AddLineBg`) while the `Colors` struct uses shorter field names (`DiffBg`, `AddBg`). This is intentional — the enum describes *what the color is for*, the field describes *what it contains*. Do not rename either side to match the other. The `Color(k)` switch in `style.go` handles the mapping internally. Document this in the comment above the `colorKey` declaration so future maintainers don't try to "fix" the inconsistency.

### `Color` type (`color.go`)

```go
// Color is an ANSI escape sequence ready to write to a terminal stream.
// Zero value is an empty Color (no-op when written).
//
// Color has no methods. fmt verbs work on the underlying string value
// (since Color is a string type alias), and emptiness is checked with
// `c != ""`. The style.Write(w, c) helper handles io.Writer cases that
// need a string() cast. If a specific call site later requires methods
// on Color, add them then — not speculatively.
type Color string

// Theme-independent reset sequences.
const (
    ResetFg Color = "\033[39m"
    ResetBg Color = "\033[49m"
)

// shiftLightness, parseHexRGB, rgbToHSL, hslToRGB, hueToRGB — private HSL math
```

### Public API — three types with independent constructors (13 methods total: 6 on Resolver + 6 on Renderer + 1 on SGR)

Per D15, the package exposes three focused types rather than one aggregate `Service`. Each type has its own constructor; there is NO wrapper container. `main.go` wires them directly into `ModelConfig`.

```go
// style/style.go — Colors struct and normalizeColors only
// (no constructors here; each type has its own constructor in its own file)

type Colors struct { /* public, same 23 hex fields as today */ }

// normalizeColors is a private helper called by NewResolver. It adds
// # prefixes to hex values and auto-derives WordAddBg/WordRemoveBg from
// AddBg/RemoveBg via shiftLightness when those fields are empty.
```

**`Resolver` — pure lookup and runtime dispatch (6 methods), in `resolver.go`**

```go
// Resolver holds the pre-materialized color and lipgloss style tables
// and dispatches lookups by key or by diff.ChangeType.
type Resolver struct { /* all-private: colors, lipgloss styles */ }

// NewResolver builds a Resolver from a Colors palette. Normalizes the
// input, builds all ~32 lipgloss.Style values, stores them privately.
func NewResolver(c Colors) Resolver

// PlainResolver returns a Resolver with no-color styling — used for
// --no-colors mode. Borders are preserved; all color styling is stripped.
func PlainResolver() Resolver

// static lookups
func (r Resolver) Color(k ColorKey) Color
func (r Resolver) Style(k StyleKey) lipgloss.Style

// runtime dispatch on diff.ChangeType
func (r Resolver) LineBg(change diff.ChangeType) Color
func (r Resolver) LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style
func (r Resolver) WordDiffBg(change diff.ChangeType) Color
func (r Resolver) IndicatorBg(change diff.ChangeType) Color
```

**`Renderer` — compound ANSI assembly (6 methods), in `renderer.go`**

```go
// Renderer produces complete rendered strings for specific UI widgets
// (annotation marks, cursor cells, status separators, file status marks).
// It composes multiple Color lookups into final ANSI-tagged strings.
type Renderer struct {
    res Resolver // used internally; not exposed
}

// NewRenderer wires a Renderer to a Resolver. The caller is responsible
// for constructing the Resolver first (via NewResolver or PlainResolver).
// Main.go does this wiring at startup.
func NewRenderer(res Resolver) Renderer

func (r Renderer) AnnotationInline(text string) string
func (r Renderer) DiffCursor(noColors bool) string
func (r Renderer) StatusBarSeparator() string
func (r Renderer) FileStatusMark(status diff.FileStatus) string
func (r Renderer) FileReviewedMark() string
func (r Renderer) FileAnnotationMark() string
```

**`SGR` — ANSI SGR stream processing (1 method), in `sgr.go`**

```go
// SGR processes captured ANSI SGR state from an already-rendered stream.
// Stateless — does not depend on Colors, Resolver, or any styling state.
// Used by diffview wrap rendering to carry highlighting across
// ansi.Wrap-inserted line breaks.
type SGR struct{}

func (SGR) Reemit(lines []string) []string
```

Internal state (private `sgrState` struct, `parseSGR`, `isFgColor`, `isBgColor`) lives in `sgr.go` — the public `SGR` type is a thin wrapper over the internal parser.

**Method name `Reemit` (shortened from `ReemitANSIState`)**: the parent type `SGR` already carries the "ANSI state" context, so the method name doesn't need to repeat it. `m.sgr.Reemit(lines)` reads as "the SGR processor, reemit these lines".

**Note on `FileStatusFg`**: the old branch exposed `FileStatusFg(status) Color` as a public method. In this plan it does NOT exist as a public method — after migration, its only caller would be `FileStatusMark` internally. It becomes a **private** helper (`fileStatusFg`) inside `style.go` on `Renderer`, used only by `FileStatusMark`. Tests that need to verify the underlying color resolution can call `resolver.Color(ColorKeyAddFg)` / `resolver.Color(ColorKeyRemoveFg)` / `resolver.Color(ColorKeyMutedFg)` directly — those are the three possible resolutions. Exposing both `FileStatusFg` and `FileStatusMark` publicly would create "two ways to do the same thing".

### `Write` helper (`ansi.go`)

```go
// Write writes the ANSI sequence of c to w. Convenience for avoiding
// string(c) casts at io.Writer boundaries. Returns bytes written + any error.
func Write(w io.Writer, c Color) (int, error) {
    if c == "" {
        return 0, nil
    }
    return io.WriteString(w, string(c))
}
```

### `sgrState` (`sgr.go`)

```go
// sgrState tracks active SGR (graphics) state while scanning an ANSI stream.
// Zero value is the initial no-attributes state.
type sgrState struct {
    fg, bg                string  // raw ANSI sequences, e.g. "\033[38;2;100;200;50m"
    bold, italic, reverse bool
}

func (s sgrState) scan(line string) sgrState
func (s sgrState) prefix() string
func (s sgrState) applySGR(params, seq string) sgrState

// Private package-level helpers (no natural type attachment).
func parseSGR(s string, i int) (seq, params string, end int)
func isFgColor(params string) bool
func isBgColor(params string) bool
```

### Call site transformations (representative examples)

```go
// diffview.go / view.go / annotlist.go / etc.
m.ansiFg(m.styles.colors.Accent)          → m.resolver.Color(style.ColorKeyAccentFg)
m.ansiBg(m.styles.colors.DiffBg)          → m.resolver.Color(style.ColorKeyDiffPaneBg)
m.ansiBg(m.changeBgColor(changeType))     → m.resolver.LineBg(changeType)
m.ansiFg(m.effectiveStatusFg())           → m.resolver.Color(style.ColorKeyStatusFg)
"\033[39m"                                → style.ResetFg  (or m.resolver.Color(style.ColorKeyResetFg) if we add one)
m.styles.LineAdd.Render(content)          → m.resolver.Style(style.StyleKeyLineAdd).Render(content)
coloredTextWithReset(colors.AddFg, "✓", colors.Normal)
                                          → m.renderer.FileReviewedMark()
m.annotationInline(text)                  → m.renderer.AnnotationInline(text)
m.diffCursorCell()                        → m.renderer.DiffCursor(m.noColors)

// Builder boundary: pick one based on site readability
fmt.Fprintf(&b, "%s%s%s", fg, text, style.ResetFg)  // fmt uses Stringer
style.Write(&b, fg); b.WriteString(text); style.Write(&b, style.ResetFg)  // explicit
```

### `Model` deletions (all move into `style.Resolver`, `style.Renderer`, or `style.SGR`)

```
Model.ansiColor           DELETED (private to style, called internally)
Model.ansiFg              DELETED (internal to style)
Model.ansiBg              DELETED
Model.changeBgColor       DELETED → Resolver.LineBg
Model.indicatorBg         DELETED → Resolver.IndicatorBg
Model.effectiveStatusFg   DELETED → Resolver.Color(ColorKeyStatusFg) with internal fallback
Model.annotationInline    DELETED → Renderer.AnnotationInline
Model.diffCursorCell      DELETED → Renderer.DiffCursor
Model.reemitANSIState     DELETED → SGR.Reemit
Model.scanANSIState       DELETED → style.sgrState.scan (private)
Model.parseSGR            DELETED → style.parseSGR (private fn)
Model.applySGR            DELETED → style.sgrState.applySGR (private)
Model.buildSGRPrefix      DELETED → style.sgrState.prefix (private)
Model.isFgColor           DELETED → style.isFgColor (private fn)
Model.isBgColor           DELETED → style.isBgColor (private fn)

Model.styles (old styles struct)  DELETED → replaced by three fields:
                                    m.resolver style.Resolver
                                    m.renderer style.Renderer
                                    m.sgr      style.SGR

styles.fileStatusFg       DELETED → private style.fileStatusFg on Renderer, used only internally by FileStatusMark
styles.newStyles, plainStyles, coloredText, coloredTextWithReset,
hexColorToRGB, hexVal, normalizeColors, normalizeColor,
contextStyle, dirEntryStyle, fileEntryStyle, treeItemStyle,
lineNumberStyle, contextHighlightStyle — ALL DELETED
app/ui/styles.go — DELETED
app/ui/colorutil.go — DELETED
```

### Consumer-side interfaces (`app/ui/deps.go`, new file)

Three narrow interfaces — one per `style` type. Each is independently mockable.

```go
package ui

import (
    "github.com/charmbracelet/lipgloss"

    "github.com/umputun/revdiff/app/diff"
    "github.com/umputun/revdiff/app/ui/style"
)

//go:generate moq -out mocks/resolver.go -pkg mocks -skip-ensure -fmt goimports . resolver
//go:generate moq -out mocks/renderer.go -pkg mocks -skip-ensure -fmt goimports . renderer
//go:generate moq -out mocks/sgr_processor.go -pkg mocks -skip-ensure -fmt goimports . sgrProcessor

// resolver is what Model needs for static and runtime style/color lookups.
// Implemented by style.Resolver.
type resolver interface {
    Color(k style.ColorKey) style.Color
    Style(k style.StyleKey) lipgloss.Style
    LineBg(change diff.ChangeType) style.Color
    LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style
    WordDiffBg(change diff.ChangeType) style.Color
    IndicatorBg(change diff.ChangeType) style.Color
}

// renderer is what Model needs for compound ANSI rendering operations.
// Implemented by style.Renderer.
type renderer interface {
    AnnotationInline(text string) string
    DiffCursor(noColors bool) string
    StatusBarSeparator() string
    FileStatusMark(status diff.FileStatus) string
    FileReviewedMark() string
    FileAnnotationMark() string
}

// sgrProcessor is what Model needs for ANSI SGR stream processing.
// Implemented by style.SGR.
type sgrProcessor interface {
    Reemit(lines []string) []string
}

// compile-time assertions — enforce that the concrete style package types
// satisfy the consumer-side interfaces. These are the entire point of the
// interfaces: a contract boundary, caught at compile time.
var (
    _ resolver     = (*style.Resolver)(nil)
    _ renderer     = (*style.Renderer)(nil)
    _ sgrProcessor = (*style.SGR)(nil)
)
```

Interfaces may narrow during M3 if some methods turn out not to need mocking in any test. The compile-time assertions stay regardless — they cost nothing and document the contract.

### `app/main.go` change

Line 407 area + ModelConfig wiring. Three explicit constructor calls, no wrapper:
```go
// before
cfg := ui.ModelConfig{
    Colors: ui.Colors{ /* ... */ },
    // ...
}

// after — three explicit constructors, wired directly into ModelConfig
var res style.Resolver
if opts.NoColors {
    res = style.PlainResolver()
} else {
    res = style.NewResolver(style.Colors{ /* ... */ })
}

cfg := ui.ModelConfig{
    Resolver: res,
    Renderer: style.NewRenderer(res),
    SGR:      style.SGR{},
    // ... (Colors field removed; normalization happens inside NewResolver)
}
// add import: "github.com/umputun/revdiff/app/ui/style"
```

`ui.ModelConfig` gains three fields (`Resolver`, `Renderer`, `SGR`) and loses the old `Colors` field. `NewModel` stashes them into `m.resolver` / `m.renderer` / `m.sgr`. The `--no-colors` branch picks `PlainResolver()` instead of `NewResolver(colors)`; `NewRenderer(res)` doesn't care which was used. `SGR{}` is a zero-value literal — no constructor needed.

## Testing Strategy

### Unit tests per file (one test file per source file)

- `style_test.go` — `normalizeColors` (hex normalization, WordAddBg/WordRemoveBg auto-derivation), `Colors` zero-value behavior. Small file because most of style.go is just the `Colors` struct declaration.
- `color_test.go` — `ResetFg`/`ResetBg` constant values, `shiftLightness` (dark→lighter, light→darker, edge cases), `parseHexRGB` (valid/invalid), `rgbToHSL`/`hslToRGB` (roundtrip), `hueToRGB` (all 4 branches). No methods on `Color` itself to test (D3).
- `ansi_test.go` — **this is critical because the old branch was missing this file entirely**. Cover `Write` (non-empty color, empty color, error from writer), `AnsiColor` (fg/bg codes, invalid hex → empty), `hexVal` (all hex digits, invalid chars).
- `resolver_test.go` — `NewResolver` / `PlainResolver` construction, all 6 Resolver methods (Color, Style, LineBg, LineStyle, WordDiffBg, IndicatorBg), exhaustiveness tests (`TestResolver_Color_coversAllKeys` / `TestResolver_Style_coversAllKeys` with both fullColors and sparseColors fixtures), `TestResolver_Color_StatusFgFallback` for the internal fallback path. Test fixtures live here so they're in the same file as the primary consumer; renderer_test.go can import them via the package-private reference.
- `renderer_test.go` — `NewRenderer` construction, all 6 Renderer methods (AnnotationInline, DiffCursor, StatusBarSeparator, FileStatusMark, FileReviewedMark, FileAnnotationMark), private `fileStatusFg` helper. Uses `NewResolver(fullColorsForTesting)` or `PlainResolver()` to get a Resolver for Renderer construction.
- `sgr_test.go` — `SGR.Reemit` (empty, single line, multi-line with carryover, reset mid-stream), `sgrState.scan` (no SGR, single SGR, multiple SGRs, resets, fg/bg changes, bold/italic/reverse on/off), `sgrState.prefix` (empty, fg only, bg only, all attributes), `parseSGR` (valid seq, unterminated, non-SGR CSI), `isFgColor`/`isBgColor` (24-bit, basic 30-37/40-47, rejections).

### Exhaustiveness tests (go-pkgz/enum payoff)

**Two fixtures are required** — a fully-populated `Colors` and a sparse one — to exercise both the direct-lookup and fallback paths per D4. A single fully-populated fixture would miss fallback bugs like "`ColorKeyStatusFg` silently returns empty when `StatusFg` is unset instead of falling back to `MutedFg`".

```go
// style_test.go

// fullColors has every field populated — exercises the primary resolution path
var fullColorsForTesting = Colors{
    Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Annotation: "#ffd700",
    SelectedFg: "#ffffaf", SelectedBg: "#303030",
    AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
    ModifyFg: "#f5c542", ModifyBg: "#3D2E00",
    CursorFg: "#000000", CursorBg: "#3a3a3a",
    WordAddBg: "#045e04", WordRemoveBg: "#5e0404",
    TreeBg: "#111111", DiffBg: "#222222",
    StatusFg: "#cccccc", StatusBg: "#333333",
    SearchFg: "#1a1a1a", SearchBg: "#d7d700",
    Border: "#585858",
}

// sparseColors omits optional fields — exercises fallback resolution paths
var sparseColorsForTesting = Colors{
    Accent: "#5f87ff", Muted: "#6c6c6c", Normal: "#d0d0d0", Annotation: "#ffd700",
    SelectedFg: "#ffffaf", SelectedBg: "#303030",
    AddFg: "#87d787", AddBg: "#022800", RemoveFg: "#ff8787", RemoveBg: "#3D0100",
    Border: "#585858",
    // StatusFg, DiffBg, TreeBg, ModifyFg, ModifyBg, WordAddBg, WordRemoveBg,
    // CursorFg, CursorBg, StatusBg, SearchFg, SearchBg all unset — exercise fallbacks
}

func TestResolver_Color_coversAllKeys(t *testing.T) {
    for _, fixture := range []struct{ name string; c Colors }{
        {"full", fullColorsForTesting},
        {"sparse", sparseColorsForTesting},
    } {
        t.Run(fixture.name, func(t *testing.T) {
            r := newResolver(fixture.c)  // private constructor, test lives in same package
            for _, k := range ColorKeyValues {
                if k == ColorKeyUnknown { continue }
                // no panic = switch handles every key
                _ = r.Color(k)
            }
        })
    }
}

func TestResolver_Color_StatusFgFallback(t *testing.T) {
    // specific assertion for the fallback path documented in D4
    r := newResolver(sparseColorsForTesting)
    got := r.Color(ColorKeyStatusFg)
    assert.NotEmpty(t, string(got), "StatusFg with empty StatusFg field should fall back to Muted")
    // verify it matches the Muted resolution
    assert.Equal(t, r.Color(ColorKeyMutedFg), got, "StatusFg fallback should equal MutedFg")
}

func TestResolver_Style_coversAllKeys(t *testing.T) {
    for _, fixture := range []struct{ name string; c Colors }{
        {"full", fullColorsForTesting},
        {"sparse", sparseColorsForTesting},
    } {
        t.Run(fixture.name, func(t *testing.T) {
            r := newResolver(fixture.c)
            for _, k := range StyleKeyValues {
                if k == StyleKeyUnknown { continue }
                _ = r.Style(k)
            }
        })
    }
}
```

These tests catch the "new enum value added without a matching case" class of bug at CI time, AND catch fallback-path regressions (any new `ColorKey` that returns empty when its source field is unset will be caught by the sparse-fixture pass if it has a documented fallback).

### Integration validation (during M2)

After migrating call sites, re-run the full UI test suite. Every existing `app/ui/*_test.go` file that exercised rendering, annotations, filetree, etc. should still pass without assertion changes (the output is the same — only the code path changed).

### Manual smoke test (end of M3)

Not checkboxed — for human execution at the end:
- Multi-file diff review (`revdiff HEAD~3`): tree navigation, diff rendering, search, annotations, collapsed mode, theme selector, help overlay
- Single-file diff review (`revdiff --only foo.go`)
- Markdown full-context single file
- Each bundled theme via `T` key — verify colors update
- `--no-colors` mode — verify all decorations fall back cleanly

## Progress Tracking

- Mark completed items with `[x]` immediately when done.
- Add newly discovered tasks with ➕ prefix.
- Document issues/blockers with ⚠️ prefix.
- Update this plan file when scope changes during implementation.

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): tasks organized under milestones. Intermediate tasks may leave build/tests broken; milestone tasks are the convergence points.
- **Post-Completion** (no checkboxes): manual smoke-test checklist.

## Implementation Steps

### Milestone 1: Style package skeleton (green-gate: `go test ./app/ui/style/...`)

The goal of M1 is a **fully self-contained `app/ui/style/` package** with all types, methods, and tests. The full project does not build yet — `app/ui/styles.go` and the old `styles` struct are still in place, and `Model` still has all its soon-to-be-deleted methods. That's expected.

#### Task 1.1: Add `go-pkgz/enum` dependency and vendor

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `vendor/modules.txt` (regenerated)
- Modify: `vendor/github.com/go-pkgz/enum/...` (vendored)

- [x] run `go get github.com/go-pkgz/enum@latest`
- [x] run `go mod vendor`
- [x] verify `vendor/github.com/go-pkgz/enum/` exists
- [x] no tests — this is just dep setup

#### Task 1.2: Create `enums.go` with colorKey and styleKey declarations

**Files:**
- Create: `app/ui/style/enums.go`

- [x] create `app/ui/style/enums.go` with `package style` header
- [x] add `//go:generate` directives for both enums: `go run github.com/go-pkgz/enum@latest -type colorKey` and same for `styleKey`
- [x] declare `type colorKey int` (unexported) with all 11 iota constants: `colorKeyUnknown` (zero sentinel), `colorKeyAccentFg`, `colorKeyMutedFg`, `colorKeyAnnotationFg`, `colorKeyStatusFg`, `colorKeyDiffPaneBg`, `colorKeyAddLineBg`, `colorKeyRemoveLineBg`, `colorKeyWordAddBg`, `colorKeyWordRemoveBg`, `colorKeySearchBg`
- [x] declare `type styleKey int` (unexported) with all 32 iota constants (1 sentinel + 24 core + 7 D14 ad-hoc styles: `styleKeyAnnotInputText`, `styleKeyAnnotInputPlaceholder`, `styleKeyAnnotInputCursor`, `styleKeyAnnotListBorder`, `styleKeyHelpBox`, `styleKeyThemeSelectBox`, `styleKeyThemeSelectBoxFocused`). Exact list finalized during Task 1.6 (resolver.go) after grepping for inline `lipgloss.NewStyle()` calls.
- [x] add comments above both type declarations noting they are generator input and the exported generated names are what to use at call sites (reference: D4 enum/field naming mismatch note, D14 ad-hoc style expansion note)
- [x] run `go generate ./app/ui/style/...` to produce `color_key_enum.go` and `style_key_enum.go`
- [x] verify both generated files exist and compile (`go build ./app/ui/style/...`)
- [x] verify generated exports: `ColorKey` type, `ColorKeyUnknown`/`ColorKeyAccentFg`/... constants, `ColorKeyValues` slice, `String()` method. Same for `StyleKey`. Pattern verified against `~/.claude2/skills/knowledge-base/references/enum.md` (unexported source → exported generated names).
- [x] no tests for the `enums.go` declarations themselves — the generator's output is covered by the exhaustiveness tests in Task 1.6 (resolver_test.go)

#### Task 1.3: Create `color.go` with Color type, reset constants, and HSL helpers

**Files:**
- Create: `app/ui/style/color.go`
- Create: `app/ui/style/color_test.go`

- [x] create `color.go` with `type Color string` — NO methods initially (per D3, add methods only if a caller emerges during M2)
- [x] add `ResetFg` and `ResetBg` typed `Color` constants
- [x] port HSL helpers from old branch: `shiftLightness`, `parseHexRGB`, `rgbToHSL`, `hslToRGB`, `hueToRGB` — all private. Reference: `git show ui-subpackage-extraction:app/ui/style/color.go`
- [x] write tests for `shiftLightness` (dark → lighter, light → darker, saturation 0, boundaries 0/1)
- [x] write tests for `parseHexRGB` (valid 7-char hex, missing #, wrong length, invalid chars)
- [x] write tests for `rgbToHSL` / `hslToRGB` roundtrip with known colors
- [x] write tests for `hueToRGB` all 4 branches
- [x] tests for `ResetFg` / `ResetBg` constant values

#### Task 1.4: Create `ansi.go` with Write helper and private AnsiColor

**Files:**
- Create: `app/ui/style/ansi.go`
- Create: `app/ui/style/ansi_test.go`

- [x] create `ansi.go` with `func Write(w io.Writer, c Color) (int, error)` (standalone, per D10)
- [x] add private `AnsiColor(hex string, code int) string` — builds `\033[<code>;2;r;g;b;m` sequence (used internally by `Resolver.Color`)
- [x] add private `hexVal(c byte) byte` helper
- [x] write tests for `Write` — empty color (0 bytes, nil err), non-empty color (writes exact bytes), writer error propagation (use a fake writer)
- [x] write tests for `AnsiColor` — valid 7-char hex with `#`, valid 6-char without `#`, invalid inputs return empty
- [x] write tests for `hexVal` — all hex digits 0-9, a-f, A-F, invalid chars return 0
- [x] **this file is the one the old branch skipped — full coverage is a hard requirement**

#### Task 1.5: Create `style.go` with Colors struct and normalizeColors

**Files:**
- Create: `app/ui/style/style.go`
- Create: `app/ui/style/style_test.go`

Small file — the package-level `Colors` config struct and a private normalization helper. No types beyond `Colors`, no constructors. Per D15, each of the three types (`Resolver`, `Renderer`, `SGR`) has its own file; `style.go` is just the package-level data bag and helper.

- [x] create `style.go` with `Colors` struct (same 23 hex fields as current `ui.Colors` — copy field list from `app/ui/styles.go:12`)
- [x] implement private method `(c Colors) normalize() Colors` (copy from `ui-subpackage-extraction:app/ui/style/style.go`, includes `WordAddBg`/`WordRemoveBg` auto-derivation via `shiftLightness` from `color.go`). **NOTE**: this was initially drafted as standalone `normalizeColors(c Colors) Colors` and corrected to a method on `Colors` during mid-implementation cleanup — see the "design mistake" row in Alternatives Considered and Design Philosophy #4. Apply the method form from the start in future extractions.
- [x] (optional) add a package-level godoc comment at the top of `style.go` describing the three-type layout (`Resolver`, `Renderer`, `SGR`) so future readers browsing the package understand the structure without hunting through multiple files. Alternative: put this doc comment in a dedicated `doc.go`.
- [x] write tests for `normalizeColors` — `#` prefix normalization, auto-derivation of `WordAddBg`/`WordRemoveBg` when empty, preservation of already-set values

#### Task 1.6: Create `resolver.go` with `Resolver` type, constructors, and 6 methods

**Files:**
- Create: `app/ui/style/resolver.go`
- Create: `app/ui/style/resolver_test.go`

`Resolver` is the lookup/dispatch layer. It holds the pre-materialized color and lipgloss style tables and exposes 6 methods. Its constructor takes `Colors` and normalizes + builds internally.

- [x] declare `type Resolver struct` with ALL-PRIVATE fields (private `Colors` field, private lipgloss style table keyed by `StyleKey` — or individual fields — implementation choice)
- [x] implement `NewResolver(c Colors) Resolver` — public constructor. Calls `normalizeColors` from style.go, builds all ~32 lipgloss.Style values (including the D14 ad-hoc popup/input styles), stores them privately. Used by `main.go`.
- [x] implement `PlainResolver() Resolver` — public constructor for `--no-colors` mode. Borders preserved, all color styling stripped. Used by `main.go`.
- [x] **before writing the `Style(k)` switch, grep `app/ui/*.go` for every `lipgloss.NewStyle()` call and confirm the `StyleKey` enum covers each ad-hoc inline construction** — add missing keys to `enums.go` if the initial set doesn't cover everything
- [x] implement `(r Resolver) Color(k ColorKey) Color` with a switch over all 11 keys. Each returns the ANSI fg or bg sequence via private `AnsiColor` from `ansi.go`. `ColorKeyStatusFg` case implements `StatusFg || MutedFg` fallback.
- [x] implement `(r Resolver) Style(k StyleKey) lipgloss.Style` with a switch over all 32 keys (1 sentinel + 24 core + 7 D14 popup/input styles)
- [x] implement `(r Resolver) LineBg(change diff.ChangeType) Color` (ChangeAdd → AddBg ANSI, ChangeRemove → RemoveBg ANSI, else empty)
- [x] implement `(r Resolver) LineStyle(change diff.ChangeType, highlighted bool) lipgloss.Style` (dispatches to LineAdd/LineAddHighlight/etc)
- [x] implement `(r Resolver) WordDiffBg(change diff.ChangeType) Color` (dispatches to WordAddBg/WordRemoveBg)
- [x] implement `(r Resolver) IndicatorBg(change diff.ChangeType) Color` (LineBg if non-empty, else DiffPaneBg)
- [x] private lipgloss builders called by `NewResolver` — **implement as methods on `Colors`**, NOT as standalone functions: `(c Colors) contextStyle()`, `(c Colors) dirEntryStyle()`, `(c Colors) fileEntryStyle()`, `(c Colors) treeItemStyle(fg string)`, `(c Colors) lineNumberStyle()`, `(c Colors) contextHighlightStyle()`. Copy bodies from old branch reference `git show ui-subpackage-extraction:app/ui/style/style.go` but rewrite each as a method with `Colors` as receiver. These methods are package-private (lowercase), so `Colors` remains a pure data struct for external callers. **Historical note**: the initial plan drafted these as standalone functions; they were corrected to methods during mid-implementation cleanup (commit `4e866a1`). Apply the method form from the start in future extractions — see Design Philosophy #4 and the "design mistake" entry in Alternatives Considered.
- [x] define test fixtures `fullColorsForTesting` and `sparseColorsForTesting` at the top of `resolver_test.go` — these are package-private, reusable by `renderer_test.go` in the same package
- [x] write tests for `NewResolver` and `PlainResolver` (construction doesn't panic, internal state is non-nil)
- [x] write tests for all 6 Resolver methods (table-driven where possible)
- [x] write `TestResolver_Color_coversAllKeys` — two-fixture exhaustiveness iterating `ColorKeyValues` with both `fullColorsForTesting` and `sparseColorsForTesting`
- [x] write `TestResolver_Style_coversAllKeys` — same pattern for `StyleKeyValues`
- [x] write `TestResolver_Color_StatusFgFallback` verifying `ColorKeyStatusFg` falls back to `ColorKeyMutedFg` when `Colors.StatusFg` is empty

#### Task 1.7: Create `renderer.go` with `Renderer` type, constructor, and 6 methods

**Files:**
- Create: `app/ui/style/renderer.go`
- Create: `app/ui/style/renderer_test.go`

`Renderer` produces complete rendered strings for specific UI widgets. It holds a `Resolver` internally to look up colors during assembly.

- [x] declare `type Renderer struct { res Resolver }` (holds a Resolver internally for color lookups)
- [x] implement `NewRenderer(res Resolver) Renderer` — public constructor. Called by `main.go` after the caller constructs a Resolver via `NewResolver` or `PlainResolver`.
- [x] implement `(r Renderer) AnnotationInline(text string) string` — port from `diffview.go:848`, use `r.res.Color(style.ColorKeyAnnotationFg)` and `r.res.Color(style.ColorKeyDiffPaneBg)` internally
- [x] implement `(r Renderer) DiffCursor(noColors bool) string` — port from `diffview.go:872`
- [x] implement `(r Renderer) StatusBarSeparator() string` — port from `view.go:153` — `" " + MutedFg + "|" + StatusFg + " "`
- [x] implement `(r Renderer) FileStatusMark(status diff.FileStatus) string` — calls private `fileStatusFg(status)` internally
- [x] implement `(r Renderer) FileReviewedMark() string` — the `coloredTextWithReset(AddFg, "✓", Normal)` pattern
- [x] implement `(r Renderer) FileAnnotationMark() string` — the `coloredText(Annotation, " *")` pattern
- [x] implement private helper `(r Renderer) fileStatusFg(status diff.FileStatus) Color` — used only by `FileStatusMark`, NOT exposed publicly (I6 feedback)
- [x] write tests for `NewRenderer` (wires the resolver correctly)
- [x] write tests for all 6 Renderer methods — construct a `Renderer` from `NewResolver(fullColorsForTesting)` or `PlainResolver()` (fixtures live in `resolver_test.go`, same package)

#### Task 1.8: Create `sgr.go` with `SGR` type, private `sgrState`, and parsing helpers

**Files:**
- Create: `app/ui/style/sgr.go`
- Create: `app/ui/style/sgr_test.go`

Per D15, `SGR` is one of the three public types exposed by the `style` package (alongside `Resolver` and `Renderer`). It wraps the internal SGR parsing state machine and exposes a single method `Reemit`. Zero coupling to `Colors`, `Resolver`, or any other styling state.

- [x] create `sgr.go` with exported type `type SGR struct{}` (stateless; zero-value is usable)
- [x] create private `sgrState` struct (fields: fg, bg string; bold, italic, reverse bool) — tracks active SGR state during a scan
- [x] implement `(s sgrState) scan(line string) sgrState` — ports `Model.scanANSIState` + `Model.parseSGR` + `Model.applySGR` logic
- [x] implement `(s sgrState) prefix() string` — ports `Model.buildSGRPrefix`
- [x] implement `(s sgrState) applySGR(params, seq string) sgrState` — ports `Model.applySGR`
- [x] private package-level helpers: `parseSGR(s string, i int) (seq, params string, end int)`, `isFgColor(params string) bool`, `isBgColor(params string) bool` — ports from `diffview.go:484-539`. These stay as functions (no natural state type) per D9.
- [x] implement **`(SGR) Reemit(lines []string) []string`** — the public entry point. Method name shortened from `ReemitANSIState` because the type name `SGR` already carries the "ANSI state" context. Body: iterate lines, accumulate `sgrState` via `.scan`, prepend `.prefix()` to continuation lines. Receiver is unused (stateless `SGR{}`), which is fine — the type exists to provide a method namespace for the interface contract.
- [x] write tests for `sgrState.scan` — no SGR, single SGR, multiple SGRs, full reset, fg reset, bg reset, bold/italic/reverse transitions
- [x] write tests for `sgrState.prefix` — empty state, fg only, bg only, all attributes combined
- [x] write tests for `parseSGR` — valid SGR, unterminated CSI, non-SGR CSI, string boundaries
- [x] write tests for `isFgColor` and `isBgColor` — 24-bit (38;2 / 48;2), basic (30-37 / 40-47), rejections (other SGR codes)
- [x] write tests for `SGR.Reemit` — empty input, single line, multi-line with carryover, reset mid-stream

#### Task 1.9: M1 convergence gate

**Files:**
- none (verification task)

- [x] run `go build ./app/ui/style/...` — must compile
- [x] run `go test ./app/ui/style/... -race` — all tests green
- [x] run `golangci-lint run ./app/ui/style/...` — clean (expect linter to check only style/)
- [x] **full project build/test is NOT required at this milestone — it is still broken**
- [x] M1 COMPLETE — commit as single git commit: `refactor(ui): create app/ui/style/ package skeleton`

### Milestone 2: Call-site migration (green-gate: full `go test ./... -race`)

The goal of M2 is migrating every production file and every test file to the new API, deleting the old helpers, and leaving the full project green. This is the bulky phase — tasks are organized by file groups, and **intermediate tasks may leave compilation or tests broken** (that's fine — M2's final task is the convergence gate).

#### Task 2.1: Wire three `style` types into `Model` and `main.go`

**Files:**
- Modify: `app/ui/model.go`
- Modify: `app/main.go`

- [x] **replace the single `styles styles` field** at `app/ui/model.go:51` with three fields:
  ```go
  resolver style.Resolver
  renderer style.Renderer
  sgr      style.SGR
  ```
- [x] **update `ModelConfig`**: remove the old `Colors Colors` field; add `Resolver style.Resolver`, `Renderer style.Renderer`, `SGR style.SGR` fields
- [x] **update `NewModel`** to stash `cfg.Resolver` / `cfg.Renderer` / `cfg.SGR` into the three Model fields. The old `newStyles(cfg.Colors)` / `plainStyles()` calls at `model.go:195-197` are DELETED — construction moved to `main.go` using three explicit constructors.
- [x] add `style` import in `model.go`
- [x] **update `app/main.go`** at the `ui.Colors{...}` site (line 407 area): use three explicit constructors per D15:
  ```go
  var res style.Resolver
  if opts.NoColors {
      res = style.PlainResolver()
  } else {
      res = style.NewResolver(style.Colors{ /* ... */ })
  }
  cfg := ui.ModelConfig{
      Resolver: res,
      Renderer: style.NewRenderer(res),
      SGR:      style.SGR{},
      // ...
  }
  ```
  Add `style` import.
- [x] at this point, the project does NOT compile because `app/ui/styles.go` still defines `styles`/`Colors`/`newStyles` and call sites still reference `m.styles.X`. **That's expected — continue.**

#### Task 2.2: Migrate `app/ui/themeselect.go`

**Files:**
- Modify: `app/ui/themeselect.go`
- Modify: `app/ui/themeselect_test.go`

- [x] update `origStyles styles` field: replace the single field with three — `origResolver style.Resolver`, `origRenderer style.Renderer`, `origSGR style.SGR`. These are used to save/restore state when the theme selector is opened and cancelled.
- [x] update `colorsFromTheme` to return `style.Colors`
- [x] replace all `m.ansiFg(m.styles.colors.X)` calls with `m.resolver.Color(style.ColorKeyX)`
- [x] replace `coloredTextWithReset(...)` with `m.renderer.FileStatusMark(...)` / `m.renderer.FileReviewedMark()` where applicable, or the closest compound equivalent
- [x] update theme swatch rendering (`themeselect.go:187-189`) to use new API
- [x] **migrate inline lipgloss constructions at `themeselect.go:151` and `:154-156`** — these build the theme selector box via `lipgloss.NewStyle().BorderForeground(lipgloss.Color(...)).Background(lipgloss.Color(...))` inline. Per D14, replace with `m.resolver.Style(style.StyleKeyThemeSelectBox)` (default) and `m.resolver.Style(style.StyleKeyThemeSelectBoxFocused)` (focused state). No more `lipgloss.Color(hex)` in this file.
- [x] update `plainStyles()` / `newStyles(colors)` calls at `themeselect.go:292-294` — replace each old single-style construction with three explicit calls (`style.NewResolver(colors)` / `style.NewRenderer(res)` / `style.SGR{}` for the color path; `style.PlainResolver()` / `style.NewRenderer(res)` / `style.SGR{}` for the plain path). Wire all three into the Model fields.
- [x] update `themeselect_test.go` references to `newStyles`/`plainStyles`/`styles{}` literals to new API
- [x] **tests may fail here — still broken, continue**

#### Task 2.3: Migrate `app/ui/view.go`

**Files:**
- Modify: `app/ui/view.go`
- Modify: `app/ui/view_test.go`

- [x] delete `ansiColor`, `ansiFg`, `ansiBg` methods on `Model` (view.go:325-340)
- [x] delete `effectiveStatusFg` method on `Model` (view.go:343)
- [x] replace all `m.ansiFg(m.styles.colors.X)` call sites (10 of them per grep) with appropriate `m.resolver.Color(style.ColorKeyX)` or compound method
- [x] at `view.go:153`, the status bar separator inline sequence becomes `m.renderer.StatusBarSeparator()`
- [x] at `view.go:302`, `bg := m.ansiBg(bgHex)` — identify which semantic color bgHex resolves to and migrate to `m.resolver.Color(...)` or dispatch method
- [x] **`padContentBg(content, bgHex string)` signature change**: this helper currently takes a raw hex string and calls `ansiBg(bgHex)` internally. Change signature to `padContentBg(content string, bg style.Color) string` — callers resolve the color (e.g. `m.resolver.Color(style.ColorKeyDiffPaneBg)` or `m.resolver.Color(style.ColorKeyTreeBg)`) and pass the resolved `Color` in. Replace the internal `ansiBg(bgHex)` call with direct use of `bg` (or `style.Write(&b, bg)`). Update ALL call sites of `padContentBg`: `view.go:35, 82, 83, 307` and `diffview.go:810`.
- [x] at `view.go:359`, `m.tree.filter` access stays (that's a tree thing, not style)
- [x] at `view.go:366`, `m.tree.reviewedCount()` stays
- [x] at `view.go:370-371`, separator sequences become `m.resolver.Color(style.ColorKeyMutedFg)` and `m.resolver.Color(style.ColorKeyStatusFg)`
- [x] any direct `m.styles.LineAdd`/`TreePane`/etc. field reads → `m.resolver.Style(style.StyleKeyLineAdd)` / etc.
- [x] update `view_test.go` — table-driven updates for new API references

#### Task 2.4: Half-way build sanity check

**Files:**
- none (verification)

- [x] run `go build ./...` — compilation may still be broken, but compare error count to what it was after Task 2.1. If errors are GROWING instead of shrinking, something has gone wrong — pause and investigate before Task 2.5. If errors are localized to files not yet migrated (diffview/annotate/annotlist/filetree/handlers/mdtoc), that's expected — continue.
- [x] tests are NOT expected to pass — do not run `go test`

#### Task 2.5: Migrate `app/ui/diffview.go` (biggest file, 25 sites + 7 SGR methods to delete)

**Files:**
- Modify: `app/ui/diffview.go`
- Modify: `app/ui/diffview_test.go`
- Modify: `app/ui/collapsed.go` (uses styles from same file family)
- Modify: `app/ui/collapsed_render_test.go`

- [x] delete the 7 SGR methods (`reemitANSIState`, `scanANSIState`, `parseSGR`, `applySGR`, `buildSGRPrefix`, `isFgColor`, `isBgColor`) at `diffview.go:420-539`
- [x] delete `changeBgColor` at `diffview.go:765`
- [x] delete `indicatorBg` at `diffview.go:779` — callers switch to `m.resolver.IndicatorBg(change)`
- [x] delete `annotationInline` at `diffview.go:848` — callers switch to `m.renderer.AnnotationInline(text)`
- [x] delete `diffCursorCell` at `diffview.go:872` — callers switch to `m.renderer.DiffCursor(m.noColors)`
- [x] migrate `scrollIndicatorANSI` / scroll overflow rendering at `diffview.go:232-245` — `m.ansiBg(spaceBg)` becomes `m.resolver.Color(...)` or `m.resolver.IndicatorBg(change)`
- [x] migrate word-diff highlighting at `diffview.go:576-580` — `hlOn = m.ansiBg(m.styles.colors.WordAddBg)` becomes `m.resolver.Color(style.ColorKeyWordAddBg)`, `hlOff` uses the line bg (via `LineBg(change)` or direct `ColorKeyAddLineBg`)
- [x] migrate search highlighting at `diffview.go:640` — `m.ansiBg(m.styles.colors.SearchBg)` becomes `m.resolver.Color(style.ColorKeySearchBg)`
- [x] migrate `applyIntraLineHighlight` / `changeBgColor` caller at `diffview.go:646` — becomes `m.resolver.LineBg(changeType)`
- [x] **`extendLineBg(styled, bgColor string)` signature change** at `diffview.go:789`: same pattern as `padContentBg` in Task 2.3. Change to `extendLineBg(styled string, bg style.Color) string`; callers resolve via `m.resolver.LineBg(change)` or `m.resolver.Color(...)` and pass the `Color` in. Replace internal `m.ansiBg(bgColor)` with direct use of `bg`. Update all call sites.
- [x] migrate `buildSGRPrefix`-style rendering at `diffview.go:853-886` (inside `annotationInline` and `diffCursorCell` which are being moved)
- [x] direct `m.styles.LineAdd` / `LineRemove` / `LineContext` / etc. field reads → `m.resolver.Style(style.StyleKeyLineAdd)` etc.
- [x] same migration for `collapsed.go` (5 sites per grep)
- [x] delete the `Model.reemitANSIState` caller in `renderWrappedDiffLine` — it now calls `m.sgr.Reemit(lines)`
- [x] update `diffview_test.go` and `collapsed_render_test.go` references

#### Task 2.6: Migrate remaining files (annotate, annotlist, filetree, handlers, mdtoc)

**Files:**
- Modify: `app/ui/annotate.go` (3 sites + textinput reshape — see below)
- Modify: `app/ui/annotate_test.go`
- Modify: `app/ui/annotlist.go` (8 sites + inline lipgloss.NewStyle)
- Modify: `app/ui/annotlist_test.go`
- Modify: `app/ui/filetree.go` (3 sites)
- Modify: `app/ui/filetree_test.go`
- Modify: `app/ui/handlers.go` (4 sites + inline lipgloss.NewStyle for help box)
- Modify: `app/ui/handlers_test.go`
- Modify: `app/ui/mdtoc.go` — `(toc *mdTOC) render(width, height int, focusedPane pane, s styles)` signature change: `s styles` → `res style.Resolver`. Inside, `s.FileSelected` field read → `res.Style(style.StyleKeyFileSelected)`. Caller at `view.go:44` is updated in Task 2.3 (passes `m.resolver` instead of `m.styles`), but the body of `mdtoc.render` is updated here.
- Modify: `app/ui/mdtoc_test.go`

- [x] `annotlist.go:125-129` (styled prefix with fg colors): `m.ansiFg(colors.X)` → `m.resolver.Color(style.ColorKeyX)`
- [x] `annotlist.go:168-171` (bg + border): migrate to `m.resolver.Color(...)` calls
- [x] `annotlist.go:31` (inline `lipgloss.NewStyle().BorderForeground(lipgloss.Color(...))`): per D14, replace with `m.resolver.Style(style.StyleKeyAnnotListBorder)`. No more `lipgloss.Color(hex)` in this file.
- [x] `annotlist.go` any lipgloss field reads → `m.resolver.Style(style.StyleKey...)`
- [x] `filetree.go:313` (reviewed mark): `coloredTextWithReset(...)` → `m.renderer.FileReviewedMark()`
- [x] `filetree.go:326` (status mark): `coloredTextWithReset(...)` → `m.renderer.FileStatusMark(status)`
- [x] `filetree.go:332` (annotation mark): `coloredText(...)` → `m.renderer.FileAnnotationMark()`
- [x] `handlers.go:58` (styled sequences for resets/accent/annotation): replace with `style.ResetFg`, `m.resolver.Color(style.ColorKeyAccentFg)`, `m.resolver.Color(style.ColorKeyAnnotationFg)`
- [x] `handlers.go:172-176` (help box inline `lipgloss.NewStyle().BorderForeground(lipgloss.Color(...)).Background(lipgloss.Color(...))`): per D14, replace with `m.resolver.Style(style.StyleKeyHelpBox)`. No more `lipgloss.Color(hex)` in this file.
- [x] **`annotate.go:36-49` textinput styling reshape** (NOT a simple 1:1 ANSI swap): currently `newAnnotationInput` constructs an `inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(c.Normal))` and assigns it to `ti.PromptStyle`, `ti.TextStyle`, `ti.Cursor.TextStyle`, `ti.Cursor.Style`, `ti.PlaceholderStyle`. Per D14, delete the inline construction entirely. Replace with direct `Style(k)` lookups:
  - `ti.PromptStyle = m.resolver.Style(style.StyleKeyAnnotInputText)`
  - `ti.TextStyle = m.resolver.Style(style.StyleKeyAnnotInputText)`
  - `ti.PlaceholderStyle = m.resolver.Style(style.StyleKeyAnnotInputPlaceholder)`
  - `ti.Cursor.Style = m.resolver.Style(style.StyleKeyAnnotInputCursor)`
  - `ti.Cursor.TextStyle = m.resolver.Style(style.StyleKeyAnnotInputCursor)` (or a separate key if the styles diverge)
  - If the current inline style differs from the canonical style built in `style.New()`, use whichever makes sense for the textinput — may need to confirm via grep-and-compare during implementation. No `lipgloss.Color(hex)` in the final `annotate.go`.
- [x] remaining `annotate.go` sites (3 per earlier grep) — migrate
- [x] `themeselect.go:151, 154-156` inline lipgloss construction: per D14, the theme selector box and focused variant are now `m.resolver.Style(style.StyleKeyThemeSelectBox)` and `m.resolver.Style(style.StyleKeyThemeSelectBoxFocused)`. Task 2.2 already covers other `themeselect.go` migration, but these inline lipgloss sites were added to the D14 scope during plan review — **make sure Task 2.2 covers them too** (cross-reference: if Task 2.2 missed them, fix there or here — don't double-migrate).
- [x] update `mdtoc.go` — change `render` signature from `s styles` to `res style.Resolver`, rewrite `s.FileSelected` field access to `res.Style(style.StyleKeyFileSelected)`. Same for any other `s.XxxYyy` field reads inside the body. Caller (`view.go:44`) now passes `m.resolver` instead of `m.styles`.
- [x] update `mdtoc_test.go` if it constructs a `styles{}` literal — replace with `style.NewResolver(fullColorsForTesting)` or `style.PlainResolver()` to get the `Resolver` that `mdtoc.render` now accepts
- [x] all corresponding `_test.go` updates: replace `newStyles`/`plainStyles`/`styles{}` literals and any direct `m.styles.colors.X` test access

#### Task 2.7: Delete old `styles.go` and `colorutil.go`

**Files:**
- Delete: `app/ui/styles.go`
- Delete: `app/ui/colorutil.go`
- Delete: `app/ui/styles_test.go` (if still present — content was consolidated into tests elsewhere or absorbed)
- Delete: `app/ui/colorutil_test.go` (if still present)

- [x] delete `app/ui/styles.go`
- [x] delete `app/ui/colorutil.go`
- [x] delete their test files
- [x] verify no references remain: `grep -n 'newStyles\|plainStyles\|coloredText\|hexColorToRGB\|ui\.Colors\|m\.ansiFg\|m\.ansiBg' app/ui/*.go` — must be empty

#### Task 2.8: M2 convergence gate

**Files:**
- none (verification)

- [x] run `go build ./...` — must compile project-wide
- [x] run `go test ./... -race` — all packages pass
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` — zero issues
- [x] run formatter: `~/.claude/format.sh` (or gofmt + goimports)
- [x] M2 COMPLETE — commit as single git commit: `refactor(ui): migrate call sites to new style package API`

### Milestone 3: Consumer interface, mocks, documentation, smoke test

#### Task 3.1: Add three consumer-side interfaces in `app/ui/deps.go` and generate mocks

**Files:**
- Create: `app/ui/deps.go`
- Create: `app/ui/mocks/resolver.go` (generated)
- Create: `app/ui/mocks/renderer.go` (generated)
- Create: `app/ui/mocks/sgr_processor.go` (generated)

Per D15, there are three types in the `style` package and three consumer-side interfaces in `ui/deps.go`, one per type. Each interface is narrow (1–6 methods) and independently mockable.

- [x] create `app/ui/deps.go` with three interfaces — see the "Consumer-side interfaces" code block in Technical Details for the full definition
  - `styleResolver` — 6 methods (Color, Style, LineBg, LineStyle, WordDiffBg, IndicatorBg)
  - `styleRenderer` — 6 methods (AnnotationInline, DiffCursor, StatusBarSeparator, FileStatusMark, FileReviewedMark, FileAnnotationMark)
  - `sgrProcessor` — 1 method (Reemit)
- [x] add three `//go:generate moq` directives in `deps.go` — one per interface
- [x] add three compile-time assertions in `deps.go`:
  ```go
  var (
      _ styleResolver = (*style.Resolver)(nil)
      _ styleRenderer = (*style.Renderer)(nil)
      _ sgrProcessor  = (*style.SGR)(nil)
  )
  ```
- [x] run `go generate ./app/ui/...` — produces `mocks/style_resolver.go`, `mocks/style_renderer.go`, `mocks/sgr_processor.go`
- [x] **decide**: Model holds concrete types (`style.Resolver`, `style.Renderer`, `style.SGR`) — the compile-time assertions already enforce the contract, and keeping concrete types is simpler (no runtime dispatch, simpler zero values, direct struct passing via `ModelConfig`). Interfaces can be adopted at field level later if a test case genuinely needs to mock one of the three.

#### Task 3.2: Update documentation

**Files:**
- Modify: `app/ui/doc.go`
- Modify: `CLAUDE.md`

- [x] update `app/ui/doc.go` to mention the new `app/ui/style/` sub-package and what it owns
- [x] update `CLAUDE.md` project-structure section — add `app/ui/style/` entry, remove references to the extracted files (`styles.go`, `colorutil.go`), note the milestone-based refactor policy as a data point for future subpackage work

#### Task 3.3: M3 convergence gate

**Files:**
- none (verification + smoke test)

- [x] run `go generate ./app/ui/...` — mocks regenerate cleanly
- [x] run `go build ./...`
- [x] run `go test ./... -race`
- [x] run `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0`
- [x] run `~/.claude/format.sh`
- [x] verify test coverage: `go test -cover ./app/ui/... ./app/ui/style/...` — ui 92.5%, style 92.4%
- [x] perform manual smoke test (see Post-Completion section) — not a checkbox, but must pass before final commit
- [x] M3 COMPLETE — commit as single git commit: `refactor(ui): add consumer-side style interfaces and finalize extraction`

### Task 4: Move plan to completed

**Files:**
- Move: `docs/plans/2026-04-10-style-extraction.md` → `docs/plans/completed/2026-04-10-style-extraction.md`

- [x] move this plan file to `docs/plans/completed/`
- [x] commit as `docs: mark style extraction plan complete`

## Post-Completion

*Items requiring manual intervention — no checkboxes, informational only*

**Manual smoke test** (run before considering M3 done):

- **Multi-file diff review**: `.bin/revdiff HEAD~3` — tree navigation, file selection, diff rendering with chroma syntax highlight, line annotations, file-level annotations, annotation list popup (`@`), collapsed mode (`v`), search (`/`, `n`, `N`), hunk navigation (`[`, `]`), cross-file hunk nav, wrap mode (`w`), line numbers (`L`), blame gutter (`B`), tree toggle (`t`), filter (`F`), mark reviewed (`R`), theme selector (`T`), help overlay (`?`), discard-and-quit (`Q`), save-and-quit (`q`)
- **Single-file diff review**: `.bin/revdiff --only app/main.go` — verify tree pane hidden, diff uses full width, all features work
- **Single-file markdown full-context**: `.bin/revdiff --only README.md` on a clean checkout — verify mdtoc pane shows headers, TOC navigation works, active section highlights as diff cursor moves
- **Staged review**: `.bin/revdiff --staged`
- **Stdin review**: `cat foo.diff | .bin/revdiff --stdin`
- **Intra-line word-diff**: edit one line of a file slightly — verify the changed words highlighted with `WordAddBg`/`WordRemoveBg`
- **Theme switching**: cycle through every bundled theme via `T` — verify colors update live
- **`--no-colors` mode**: `.bin/revdiff --no-colors HEAD~1` — verify intra-line highlighting falls back to reverse video, all styles fall back cleanly

**Old branch cleanup** (after this plan is merged and shipped):

- `ui-subpackage-extraction` branch can be deleted — it was kept as a reference for content (HSL math, lipgloss builder bodies, `Colors` field list) and is no longer needed once this plan's merge commit is on master.

**Follow-up plan candidates** (explicitly out of scope for this plan):

- Extract `app/ui/filetree/` sub-package. Should follow the same design principles: consumer-side interface in `ui`, parameterized accessors where applicable, named types for domain values, milestone-based gating.
- Extract `app/ui/mdtoc/` sub-package. Same template.
- Extract `app/ui/worddiff/` sub-package (pure LCS algorithm — likely simpler, no interface needed).
- Evaluate whether `configpatch.go` should move into its own package.
- Evaluate whether overlays (`annotlist.go`, `themeselect.go`) should become sub-packages with result-type event patterns.
- If the three consumer-side interfaces (`resolver`, `renderer`, `sgrProcessor`) turn out to be too broad during follow-up extraction work, consider sub-splitting — e.g., splitting `resolver` into a read-only `colorLookup` + `styleLookup` pair. Only if natural seams appear.
