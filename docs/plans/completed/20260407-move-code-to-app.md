# Move Go source packages into app/ directory

## Overview
- Reorganize project structure by moving all Go source packages into a top-level `app/` directory
- Matches the layout used in secrets and cronn projects: `app/main.go` as entry point, library packages under `app/`
- Eliminates `cmd/revdiff/` — main.go moves directly to `app/main.go`
- `go.mod`, `go.sum`, `vendor/`, and non-code files remain at repo root

## Context (from discovery)
- module path: `github.com/umputun/revdiff` (unchanged)
- packages to move: `annotation/`, `diff/`, `highlight/`, `keymap/`, `theme/`, `ui/` (with `ui/mocks/`)
- entry point to move: `cmd/revdiff/main.go` + `main_test.go` → `app/main.go` + `app/main_test.go`
- all import paths change from `github.com/umputun/revdiff/X` to `github.com/umputun/revdiff/app/X`
- `go install` path changes to `.../app@latest` (binary distributed via brew/goreleaser, not `go install`)
- build configs: `.goreleaser.yml`, `Makefile`, `.zed/tasks.json`
- docs: `README.md`, `CLAUDE.md`, `llms.txt`, `site/docs.html`, `site/llms.txt`, plugin refs

## Development Approach
- **testing approach**: Regular — pure file-move refactoring, no new tests needed
- complete each task fully before moving to the next
- make small, focused changes
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- run tests after each change

## Progress Tracking
- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix

## Solution Overview
- move all Go source packages under `app/` directory
- flatten `cmd/revdiff/main.go` to `app/main.go`
- sed-replace all internal import paths
- update build/config/doc files
- regenerate mocks, re-vendor

## Implementation Steps

### Task 1: Move source packages into app/

**Files:**
- Move: `annotation/` → `app/annotation/`
- Move: `diff/` → `app/diff/`
- Move: `highlight/` → `app/highlight/`
- Move: `keymap/` → `app/keymap/`
- Move: `theme/` → `app/theme/`
- Move: `ui/` → `app/ui/` (including `ui/mocks/`)

- [x] create `app/` directory
- [x] move `annotation/`, `diff/`, `highlight/`, `keymap/`, `theme/`, `ui/` into `app/`
- [x] move `cmd/revdiff/main.go` and `cmd/revdiff/main_test.go` to `app/main.go` and `app/main_test.go`
- [x] remove empty `cmd/` directory

### Task 2: Update all import paths

**Files:**
- Modify: `app/main.go` (6 internal imports)
- Modify: `app/main_test.go` (2 internal imports)
- Modify: `app/ui/*.go` — model.go, diffview.go, annotlist.go, annotate.go, collapsed.go, mdtoc.go, search.go, styles.go
- Modify: `app/ui/*_test.go` — model_test.go, annotlist_test.go, collapsed_test.go, diffview_test.go, mdtoc_test.go, styles_test.go
- Modify: `app/highlight/highlight.go`, `app/highlight/highlight_test.go`

- [x] replace `"github.com/umputun/revdiff/` with `"github.com/umputun/revdiff/app/` in all `.go` files under `app/`
- [x] verify no stale import paths remain: `grep -r '"github.com/umputun/revdiff/' app/ | grep -v '/app/'`
- [x] regenerate mocks: `go generate ./app/...`
- [x] run `go mod vendor`
- [x] run `go test ./app/...` — must pass

### Task 3: Update build and config files

**Files:**
- Modify: `.goreleaser.yml` — `main: ./cmd/revdiff` → `main: ./app`
- Modify: `Makefile` — build command path
- Modify: `.zed/tasks.json` — run commands
- Modify: `.gitignore` — if any path-specific entries

- [x] update `.goreleaser.yml`: `main: ./app`
- [x] update `Makefile`: build command to `cd app && go build ... -o ../.bin/revdiff.$(BRANCH)` or equivalent
- [x] update `.zed/tasks.json`: `go run ./app` instead of `go run ./cmd/revdiff`
- [x] check `.gitignore` for path-specific entries and update if needed
- [x] run `make build` — must succeed
- [x] run `make test` — must pass
- [x] run `make lint` — must pass

### Task 4: Update documentation

**Files:**
- Modify: `README.md` — install path, project structure
- Modify: `CLAUDE.md` — project structure section, file references
- Modify: `llms.txt` — install path
- Modify: `site/docs.html` — install path
- Modify: `site/llms.txt` — install path
- Modify: `.claude-plugin/skills/revdiff/SKILL.md` — install path
- Modify: `.claude-plugin/skills/revdiff/references/install.md` — install path
- Modify: `.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh` — go install error message
- Modify: `docs/plans/2026-04-06-stdin-scratch-buffer-review.md` — active plan with stale cmd/revdiff paths

- [x] update `README.md`: project structure, install command, note binary name from `go install`
- [x] update `CLAUDE.md`: project structure section, all `cmd/revdiff/` references → `app/`
- [x] update `llms.txt` and `site/llms.txt`: install path
- [x] update `site/docs.html`: install path
- [x] update `.claude-plugin/skills/revdiff/SKILL.md`: install path
- [x] update `.claude-plugin/skills/revdiff/references/install.md`: install path
- [x] update `.claude-plugin/skills/revdiff/scripts/launch-revdiff.sh`: go install path in error message
- [x] update `docs/plans/2026-04-06-stdin-scratch-buffer-review.md`: change `cmd/revdiff/` references to `app/`
- [x] do NOT update completed plan files in `docs/plans/completed/` (historical)

### Task 5: Verify acceptance criteria

- [x] `make build` succeeds
- [x] `make test` passes (race detector + coverage)
- [x] `make lint` passes
- [x] no references to old paths in Go source files
- [x] `go install` note updated in docs
- [x] top-level directory is clean: no Go packages except `app/`

### Task 6: [Final] Update plan and cleanup

- [x] update CLAUDE.md if new patterns discovered
- [x] move this plan to `docs/plans/completed/`

## Post-Completion

**Manual verification:**
- goreleaser dry-run to verify binary builds from new path
- verify plugin skill still works after path changes

**User-facing change:**
- `go install github.com/umputun/revdiff/cmd/revdiff@latest` no longer works
- new path: `go install github.com/umputun/revdiff/app@latest` (binary named `app`, not `revdiff`)
- primary distribution remains brew/goreleaser, so minimal impact
