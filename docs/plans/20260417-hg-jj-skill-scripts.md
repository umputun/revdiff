# Add hg + jj support to skill helper scripts

## Overview

The revdiff binary supports three VCS (git + hg + jj since v0.16), but the Claude Code skill's helper scripts (`detect-ref.sh`, `read-latest-history.sh`) are git-only. In hg-only or jj-only repos where `git` commands fail, the skill can't detect repo state or resolve the history directory name.

This plan adds hg and jj code paths to both scripts, keeping the git path's **runtime output** byte-identical on git repos, and mirrors the same changes into the codex plugin's script copies. (The script file itself changes — header comments, dispatch code — but `detect-ref.sh` output for a given git repo state must match the pre-change baseline exactly.) Also updates user-visible wording in `SKILL.md` (both trees) and in `references/usage.md` to drop "git-only" phrasing.

No binary changes. The `--all-files` mode is git-only inside the binary (`app/diff/directory.go` uses `git ls-files`) — that remains a separate issue, flagged in the PR description, out of scope here.

## Context (from discovery)

- **Scripts to change (primary)**: `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`, `.claude-plugin/skills/revdiff/scripts/read-latest-history.sh`
- **Scripts to change (duplicate)**: `plugins/codex/skills/revdiff/scripts/detect-ref.sh`, `plugins/codex/skills/revdiff/scripts/read-latest-history.sh` — copies, not symlinks (CLAUDE.md convention), each has a source comment at the top
- **SKILL.md files**: `.claude-plugin/skills/revdiff/SKILL.md`, `plugins/codex/skills/revdiff/SKILL.md`
- **VCS abstraction reference**: `app/diff/vcs.go:23-48` — `DetectVCS()` precedence is jj → git → hg (jj wins when colocated with .git)
- **jj ref translation reference**: `app/diff/jj.go:149-217` — empty ref = `@-..@`, `HEAD~N` = `@` + N+1 dashes, bookmarks pass through
- **Not affected**: `launch-revdiff.sh` (VCS-agnostic, passes args straight to the binary), `plugins/opencode/` (TypeScript tools, no shell scripts), `plugins/pi/skills/revdiff/` (binary-level commands only)

## Development Approach

- **testing approach**: Manual test matrix (shell scripts have no unit test framework in this repo). Validate by scaffolding throwaway `/tmp` repos in each VCS and running the script. Static checks via `shellcheck` and `shfmt`.
- complete each task fully before moving to the next
- make small, focused changes
- keep the git path's runtime output byte-identical — pre/post `detect-ref.sh` output on any git repo state must match exactly
- verify jj template/subcommand output against the locally installed jj version before writing `detect_jj()` — jj command output format varies across releases
- **CRITICAL: update this plan file when scope changes during implementation**

## Testing Strategy

- **static**: `shellcheck` and `shfmt -d` must pass on both script trees before each commit
- **manual matrix** (run `detect-ref.sh` in throwaway repos under `/tmp`, verify each row's expected output):

  | repo state | expected `suggested_ref` / `needs_ask` |
  |---|---|
  | git on `master`/`main`, clean | `HEAD~1`, false |
  | git on `master`/`main`, uncommitted | empty, false |
  | git on `master`/`main`, staged only | empty + `use_staged=true`, false |
  | git on feature, clean | `<main_branch>`, false |
  | git on feature, uncommitted | empty, **true** |
  | git init, no commits | `--all-files`, false |
  | hg on `default`, clean | `HEAD~1`, false |
  | hg on `default`, uncommitted | empty, false |
  | hg on named branch, clean | `default`, false |
  | hg on named branch, uncommitted | empty, **true** |
  | hg init, no commits (untracked files may exist) | empty, **true** (no-commits short-circuit fires before uncommitted check) |
  | jj `@` empty on main ancestor | `HEAD~1`, false |
  | jj `@` non-empty on main ancestor | empty, false |
  | jj `@` empty on feature | `<main_bookmark>`, false |
  | jj `@` non-empty on feature | empty, **true** |
  | jj colocated with `.git` | jj path wins (not git) — verify branch/is_main populated via jj commands |
  | no VCS at all | empty, **true** |

- **cross-tree**: diff the two copies of each script after editing both — they must be functionally identical (only `# Source:` comment header differs)

## Progress Tracking

- mark completed items with `[x]` immediately when done
- add newly discovered tasks with ➕ prefix
- document issues/blockers with ⚠️ prefix
- update plan if implementation deviates from original scope

## Solution Overview

### `detect-ref.sh`

Top-level VCS probe picks one of `git | hg | jj | unknown`, dispatches to a `detect_<vcs>` function that populates eight shell variables (same set the current script outputs). A shared `apply_decision_logic` block then derives `suggested_ref` / `use_staged` / `needs_ask` from those variables using the existing branching — unchanged, except for one patch: the no-commits path emits `--all-files` only for git (since the binary's `--all-files` is git-only); hg/jj with no commits fall through to `needs_ask=true`.

Output format stays byte-identical. The skill consumes the same eight fields; it doesn't need to know which VCS produced them.

### `read-latest-history.sh`

Single line change — repo basename resolution falls through jj → git → hg → `pwd`, matching `DetectVCS()` precedence. Header comment updated to reflect multi-VCS resolution.

### `SKILL.md` wording

Both trees: argument-hint drops "git", intro line gains "Works in git, hg, and jj repos (auto-detected)", history section says "VCS root basename (jj/git/hg)", file-review note says "outside a VCS repo".

## Technical Details

### VCS probe (top of `detect-ref.sh`)

Probe order matches `app/diff/vcs.go:30-40` precedence (jj first, because jj often colocates with `.git`):

```bash
vcs="unknown"
if command -v jj >/dev/null 2>&1 && jj root >/dev/null 2>&1; then
    vcs="jj"
elif git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    vcs="git"
elif command -v hg >/dev/null 2>&1 && hg root >/dev/null 2>&1; then
    vcs="hg"
fi
```

`command -v` guards short-circuit the probe when `jj` or `hg` isn't installed — avoids a subprocess spawn and any `command not found` noise on the common git-only path.

### Field population — hg

```bash
detect_hg() {
    branch=$(hg branch 2>/dev/null || echo unknown)
    main_branch="default"
    is_main=false
    [ "$branch" = "$main_branch" ] && is_main=true

    has_uncommitted=false
    [ -n "$(hg status 2>/dev/null)" ] && has_uncommitted=true

    has_staged_only=false   # no index in hg
    has_commits=true
    hg log -r . -l 1 -T '.' >/dev/null 2>&1 || has_commits=false
}
```

### Field population — jj

**Pre-implementation spike (Task 3)**: before writing this function, run each command below against the locally installed jj version and record actual stdout. jj's template and subcommand output format varies across 0.18–0.30+; adjust the parsing in the function to match what jj actually emits. Document the minimum jj version we target in a comment at the top of `detect_jj()`.

```bash
detect_jj() {
    # bookmarks on @; jj @ is usually anonymous (empty template output)
    branch=$(jj log -r @ --no-graph -T 'bookmarks' 2>/dev/null)
    [ -z "$branch" ] && branch="@"

    # detect main bookmark: try main, then master, then trunk
    # check via `jj log -r <name>` (more stable than `bookmark list` output parsing —
    # exit non-zero on unresolvable name, exit 0 and non-empty on resolution)
    main_branch=""
    for candidate in main master trunk; do
        if jj log -r "$candidate" -l 1 --no-graph -T '.' >/dev/null 2>&1; then
            main_branch="$candidate"
            break
        fi
    done

    is_main=false
    if [ -n "$main_branch" ]; then
        # nearest ancestor bookmark — the actual "am I on main" semantic,
        # since @ is usually an anonymous change with no bookmarks
        nearest=$(jj log --no-graph \
            -r "latest(heads(::@ & bookmarks()))" \
            -T 'bookmarks' 2>/dev/null)
        # bookmarks template separator varies by jj version (space or comma);
        # guard both forms so we don't need to pin a separator
        case " $nearest " in *" $main_branch "*) is_main=true ;; esac
        case ",$nearest," in *",$main_branch,"*) is_main=true ;; esac
    fi

    # "uncommitted" = @ has changes vs @-. Use `jj diff --summary` which is
    # spec-stable across versions — empty stdout means @ equals @-.
    has_uncommitted=false
    if [ -n "$(jj diff -r @ --summary 2>/dev/null)" ]; then
        has_uncommitted=true
    fi

    has_staged_only=false
    has_commits=true   # jj always has @
}
```

**Why `jj diff -r @ --summary` instead of `-T 'empty'`**: the `empty` keyword in templates only works on recent jj versions and its literal output varies. `jj diff --summary` has been stable since early jj releases — empty output = no changes.

**Why `jj log -r <name>` instead of `jj bookmark list -r <name>` for bookmark detection**: `bookmark list` output format (prefix, status markers like `(ahead by 1)`) varies across jj versions. `jj log -r <name>` returns non-zero exit for unresolvable names, which is all we need.

Bookmark-separator guard (space vs comma) in the nearest-ancestor check stays defensive — if neither matches, `is_main` stays false and falls through to the "feature branch" decision path, which is still safe.

### Patched decision block

Decision logic stays as-is **except** for the `has_commits=false` branch — it must short-circuit **before** `is_main`/`has_uncommitted` branching, because a fresh hg repo may show `?` untracked files (setting `has_uncommitted=true`) that would otherwise misroute it into the "main+uncommitted" arm:

```bash
if [ "$has_commits" = "false" ]; then
    if [ "$vcs" = "git" ]; then
        suggested_ref="--all-files"   # git's fallback browses staged files
    else
        needs_ask=true                 # hg/jj: --all-files is git-only in the binary
    fi
    # short-circuit — do not fall through to is_main/uncommitted branches
else
    # existing logic unchanged (is_main branching, use_staged, etc.)
    ...
fi
```

All other branches (`is_main` true/false, `has_uncommitted` true/false, staged handling) stay verbatim inside the `else`.

### `read-latest-history.sh` change

```bash
repo="$(basename "$(jj root 2>/dev/null \
                 || git rev-parse --show-toplevel 2>/dev/null \
                 || hg root 2>/dev/null \
                 || pwd)")"
```

## What Goes Where

- **Implementation Steps** (`[ ]` checkboxes): edits to both script trees, SKILL.md wording in both trees, static checks, manual matrix
- **Post-Completion** (no checkboxes): version bump decision (ask user), PR description callouts (`--all-files` gap)

## Implementation Steps

### Task 1: Refactor `detect-ref.sh` to dispatch by VCS, git path unchanged

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`

- [x] capture pre-change baseline on a real git repo: `detect-ref.sh > ~/revdiff-detect-ref-baseline.txt` (durable path, not `/tmp` — survives reboots between tasks)
- [x] move existing git detection logic into a `detect_git()` function — zero logic changes, just wrap the body
- [x] add top-level VCS probe (jj → git → hg → unknown) with `command -v` guards, setting `vcs` variable
- [x] add **stubs** for `detect_hg()` and `detect_jj()` that set `needs_ask=true` and all other fields to empty/false — fully implemented in Tasks 2 and 3. This keeps the script in a working state for hg/jj repos (falls through to the "needs ask" path) rather than dispatching to undefined functions.
- [x] extract decision logic into `apply_decision_logic()` and call after dispatch
- [x] add `unknown` arm identical to the hg/jj stubs (needs_ask=true, fields empty/false)
- [x] patch no-commits branch with early short-circuit: `--all-files` only when `vcs=git`; otherwise `needs_ask=true`. Short-circuit **before** `is_main`/`has_uncommitted` branching (see Technical Details > Patched decision block)
- [x] update header comment to describe multi-VCS auto-detection (jj → git → hg precedence)
- [x] run `shellcheck` and `shfmt -d` — fix any issues (ran `shfmt -d -i 4` to match existing 4-space convention; shellcheck clean)
- [x] manual test: run `detect-ref.sh` on the same git repo state as the baseline; `diff ~/revdiff-detect-ref-baseline.txt <(detect-ref.sh)` must be empty (verified byte-identical output across 5 throwaway git states: main clean, main uncommitted, main staged-only, feature clean, feature uncommitted, plus no-commits)
- [x] manual test: run in an hg repo and a jj repo (if available) — both should output `needs_ask: true` with empty fields (stub behaviour, real logic in Tasks 2-3) (hg stub verified; jj not installed locally but stub logic is identical)
- [x] must pass before next task

### Task 2: Add `detect_hg()` implementation

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`

- [ ] replace `detect_hg()` stub with the real implementation per Technical Details > Field population — hg
- [ ] wire `hg` arm in the dispatch case already done in Task 1 — just confirm it now calls the real function
- [ ] run `shellcheck` and `shfmt -d`
- [ ] manual test: scaffold `hg init` repo in `/tmp`, run through each hg matrix row (clean/uncommitted/named branch/no-commits). Verify the no-commits row outputs `needs_ask: true` (the early short-circuit in the decision block must fire before `has_uncommitted` can misroute)
- [ ] re-run the git baseline diff (`diff ~/revdiff-detect-ref-baseline.txt <(detect-ref.sh)` on the same git repo) — must still be empty
- [ ] must pass before next task

### Task 3: Add `detect_jj()` implementation

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/detect-ref.sh`

- [ ] **spike jj commands** against the locally installed jj version (`jj --version` to record). For each of these, run against a scaffolded `jj git init` repo and paste actual stdout into a scratchpad for reference:
      - `jj log -r @ --no-graph -T 'bookmarks'` (empty @, with and without bookmarks on @)
      - `jj log -r main -l 1 --no-graph -T '.'` (with and without `main` bookmark)
      - `jj log --no-graph -r "latest(heads(::@ & bookmarks()))" -T 'bookmarks'` (observe separator: space vs comma)
      - `jj diff -r @ --summary` (empty @ vs dirty @)
- [ ] replace `detect_jj()` stub with the real implementation per Technical Details > Field population — jj. Adjust parsing if spike showed unexpected format.
- [ ] add a comment at the top of `detect_jj()` noting the minimum jj version the spike was run against
- [ ] wire `jj` arm in the dispatch case already done in Task 1 — just confirm it now calls the real function
- [ ] run `shellcheck` and `shfmt -d`
- [ ] manual test: scaffold `jj git init` repo in `/tmp`, run through each jj matrix row (empty `@`, dirty `@`, with/without `main` bookmark, colocated with `.git`), verify output
- [ ] must pass before next task

### Task 4: Update `read-latest-history.sh` repo root resolution

**Files:**
- Modify: `.claude-plugin/skills/revdiff/scripts/read-latest-history.sh`

- [ ] change the `repo=` line to probe jj → git → hg → pwd per Technical Details
- [ ] update header comment block to describe multi-VCS resolution
- [ ] run `shellcheck` and `shfmt -d`
- [ ] manual test: place a dummy history file under `~/.config/revdiff/history/<reponame>/test.md`, run the script from inside a git repo, hg repo, and jj repo — confirm correct file is printed in each
- [ ] must pass before next task

### Task 5: Mirror changes into codex plugin script copies

**Files:**
- Modify: `plugins/codex/skills/revdiff/scripts/detect-ref.sh`
- Modify: `plugins/codex/skills/revdiff/scripts/read-latest-history.sh`

- [ ] first inspect the codex copies to see exactly which lines are codex-specific (expected: a `# Source:` comment pointing back to `.claude-plugin/...`, right after the shebang): `head -5 plugins/codex/skills/revdiff/scripts/detect-ref.sh`
- [ ] for each script, replace the body while preserving the codex-specific header. Concrete recipe (adjust line count if `# Source:` spans multiple lines):
      ```bash
      # say the codex header is lines 1-3 (shebang + blank + # Source:)
      SRC=.claude-plugin/skills/revdiff/scripts/detect-ref.sh
      DST=plugins/codex/skills/revdiff/scripts/detect-ref.sh
      head -3 "$DST" > "$DST.tmp"                       # keep codex-specific header
      tail -n +2 "$SRC" >> "$DST.tmp"                   # append new body (skip SRC shebang)
      mv "$DST.tmp" "$DST" && chmod +x "$DST"
      ```
      Verify header line count by inspecting `head -5` output before running. Adjust `head -N`/`tail -n +N` accordingly.
- [ ] repeat for `read-latest-history.sh`
- [ ] run `shellcheck` and `shfmt -d` on both codex scripts
- [ ] diff the two trees ignoring the first few lines: `diff <(tail -n +3 .claude-plugin/skills/revdiff/scripts/detect-ref.sh) <(tail -n +4 plugins/codex/skills/revdiff/scripts/detect-ref.sh)` — should be empty (adjust offsets based on actual header lengths)
- [ ] manual test: repeat one hg and one jj matrix row using the codex copy to confirm parity
- [ ] must pass before next task

### Task 6: Update `SKILL.md` wording in both trees

**Files:**
- Modify: `.claude-plugin/skills/revdiff/SKILL.md`
- Modify: `plugins/codex/skills/revdiff/SKILL.md`

- [ ] `argument-hint`: change `optional: git ref(s), "all files", or file path` → `optional: ref(s), "all files", or file path`
- [ ] intro line: change `Review git diffs with inline annotations…` → `Review diffs with inline annotations… Works in git, hg, and jj repos (auto-detected).`
- [ ] history section: change `via git rev-parse --show-toplevel basename` → `via VCS root basename (jj/git/hg)`
- [ ] file review note: change `Works both inside and outside a git repo` → `Works both inside and outside a VCS repo`
- [ ] apply same four edits to the codex copy
- [ ] grep both trees for any remaining `git repo` / `git rev-parse` phrasing in user-facing text (excluding code blocks that document the actual commands); flag anything found, don't edit blindly
- [ ] must pass before next task

### Task 7: Audit references for VCS-specific wording

**Files:**
- Modify: `.claude-plugin/skills/revdiff/references/usage.md` (confirmed hits around lines 59-62)
- Modify (maybe): `.claude-plugin/skills/revdiff/references/install.md`, `config.md`
- Modify (maybe): `plugins/codex/skills/revdiff/references/usage.md`, `install.md`, `config.md` if they mirror the .claude-plugin copies

- [ ] grep all six reference files for `git repo` / `git-only` / `inside a git` / `outside a git`: `grep -rn "git repo\|git-only\|inside a git\|outside a git" .claude-plugin/skills/revdiff/references/ plugins/codex/skills/revdiff/references/`
- [ ] update `references/usage.md` user-facing prose at lines 59-62 (and any other confirmed hits) to VCS-neutral wording — e.g. "inside a git repo" → "inside a repo (git/hg/jj)"
- [ ] leave command examples alone — `git log`, `git diff` in example blocks remain accurate for git repos
- [ ] verify codex reference copies match if they exist (same copy-not-symlink convention applies)
- [ ] re-grep to confirm no remaining user-facing "git-only" prose
- [ ] must pass before next task

### Task 8: Verify acceptance criteria

- [ ] re-run `shellcheck` and `shfmt -d` across all four scripts
- [ ] diff `.claude-plugin/skills/revdiff/scripts/` vs `plugins/codex/skills/revdiff/scripts/` (ignoring the codex-specific header lines) — confirm bodies are identical
- [ ] re-run the full manual test matrix from Testing Strategy, including the jj-colocated-with-`.git` row
- [ ] verify git repo output is byte-identical to the durable baseline: `diff ~/revdiff-detect-ref-baseline.txt <(detect-ref.sh)` on the same git repo state captured in Task 1
- [ ] clean up the baseline file after verification passes
- [ ] must pass before next task

### Task 9: [Final] Documentation + plan archival

- [ ] README.md — skim for "review git diffs" phrasing, update if user-facing
- [ ] CLAUDE.md — no change expected (project structure docs already mention hg+jj)
- [ ] move this plan to `docs/plans/completed/20260417-hg-jj-skill-scripts.md`

## Post-Completion

**Version bump decision** (per CLAUDE.md rule):
- `.claude-plugin/plugin.json` + `.claude-plugin/marketplace.json` — ask user whether to bump (this is a user-visible bugfix → likely patch bump)
- `plugins/pi/` not touched → no `package.json` bump
- codex plugin has no manifest — no bump needed there

**PR description callouts:**
- Mention that `--all-files` remains git-only in the binary (`app/diff/directory.go` uses `git ls-files`); if users in hg/jj repos request all-files, the launcher will fail. Separate tracking issue, out of scope for this PR.
- Note that jj bookmark list output separator varies by jj version; the `case` patterns handle both space- and comma-separated forms.

**Manual verification in the wild** (nice-to-have, not required):
- Try the skill in a real hg-only repo and confirm auto-detection gives sensible refs.
- Try the skill in a jj-colocated repo and confirm jj path wins over git path (per `DetectVCS` precedence).
