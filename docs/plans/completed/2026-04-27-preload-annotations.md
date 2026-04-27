# Preload annotations from FormatOutput markdown (--annotations flag)

## Overview
Add a `--annotations=PATH` flag that preloads the annotation store at startup by parsing the same markdown format `Store.FormatOutput()` emits. Enables a round-trip workflow: `revdiff -o notes.md ...` → edit → `revdiff --annotations=notes.md ...`. Primary use case: seeding LLM-generated code review comments into the annotation gutter. Scope is locked by issue #153 discussion (11 + 2 points, all agreed by maintainer). Caller-side LLM tooling is out of scope — the contract is the bidirectional FormatOutput format.

## Context
- Files involved:
  - `app/annotation/store.go` — existing `Annotation` struct + `FormatOutput()` (write-only today; parser is the inverse)
  - `app/annotation/store_test.go` — existing test patterns (direct string assertions, no fixtures)
  - `app/config.go` — `options` struct, add `Annotations` field next to `Stdin`
  - `app/main.go` — `run()` wires parser between config parsing and Bubble Tea start
  - `app/diff/` — diff result is what we resolve files/lines against for the drop-orphans check
  - `README.md` — one example under run-configuration
  - `.claude-plugin/skills/revdiff/SKILL.md` and `references/usage.md` — short reference for AI agents
- Related patterns:
  - `Stdin` flag in `options` (per-invocation, `no-ini:"true"`)
  - `Store.Add` last-write-wins semantics (store.go:30-38) — preload uses the same path
  - `escapeHeaderLines` (store.go:160-171) — parser inverts this
- Dependencies: none new

## Development Approach
- Testing approach: TDD — parser is mechanical inverse of an already-tested format, table-driven tests fit naturally
- Complete each task fully before moving to the next
- CRITICAL: every task MUST include new/updated tests
- CRITICAL: all tests must pass before starting next task

## Implementation Steps

### Task 1: Parser in app/annotation/parse.go

**Files:**
- Create: `app/annotation/parse.go`
- Create: `app/annotation/parse_test.go`

- [x] write tests first: each header shape (`(+)`, `(-)`, `( )`, `(file-level)`), single line, range `n-m`, multiline body, escaped ` ## ` body line (one leading space stripped), mixed records, duplicate (last-write-wins via `Store.Add`), empty input = no-op + nil error, malformed header = error
- [x] write a `Parse(FormatOutput(s))` round-trip property test against a synthesized store covering all four shapes and a body containing `## `
- [x] implement `func Parse(r io.Reader) ([]Annotation, error)` — single regex over header lines matching the four shapes; body = lines between header and next `## ` or EOF, trimmed of one trailing newline; invert `escapeHeaderLines` (strip exactly one leading space from body lines whose prefix is ` ## `); non-matching `## ` header = error (no silent skip)
- [x] regex must accept literal space inside parens: `(+)`, `(-)`, `( )`, `(file-level)` — per maintainer point 3
- [x] run `make test` — must pass before task 2

### Task 2: CLI flag and run() wiring

**Files:**
- Modify: `app/config.go`
- Modify: `app/main.go`

- [x] add `Annotations string` to `options` next to `Stdin`, with `no-ini:"true"` and a long flag `annotations` taking a path
- [x] in `run()` after config parse, if `Annotations != ""`: open file (file-not-found = hard error to stderr, non-zero exit, before Bubble Tea starts), call `annotation.Parse`, on parse error fail hard; on success feed each record through `Store.Add`
- [x] resolve diff first, then drop orphans: file-level record (`Line=0`) requires file-in-diff; line-scoped requires both file and line present; warn on stderr per dropped record, drop silently from store
- [x] write tests for config parsing of the new flag and for the load+drop path (table-driven against a fake diff result if feasible, otherwise integration-style in `app/`)
- [x] run `make test` and `make lint` — must pass before task 3

### Task 3: Documentation

**Files:**
- Modify: `README.md`
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`
- Modify: `.claude-plugin/skills/revdiff/references/usage.md`
- Modify: `site/docs.html` (per CLAUDE.md sync rule with README)
- Modify: `plugins/codex/skills/revdiff/SKILL.md` if it mirrors the same flag list

- [x] README: one example under run-configuration: `revdiff --annotations=review.md HEAD~1`, plus a one-line note that the format is the same `-o` output (round-trip)
- [x] SKILL.md + usage.md: short reference noting the flag exists and the format is the FormatOutput shape both directions
- [x] site/docs.html: add the flag to the options reference and a usage example, mirroring README
- [x] no test changes for docs — but verify: grep all occurrences of `--stdin` in docs and confirm `--annotations` is added in matching spots

### Task 4: Verify acceptance criteria

- [x] `make test` (race + coverage) passes — all packages green; toolchain version mismatch warning in env is pre-existing
- [x] `make lint` passes — 0 issues
- [x] coverage on `app/annotation/parse.go` ≥ 80% — 97.2% (Parse 97.2%, parseHeader 94.1%)

### Task 5: Update changelog and move plan

- [x] update CHANGELOG with `--annotations` entry under unreleased
- [x] move `docs/plans/2026-04-27-preload-annotations.md` to `docs/plans/completed/`
