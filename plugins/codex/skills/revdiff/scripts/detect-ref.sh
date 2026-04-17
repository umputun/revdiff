#!/usr/bin/env bash
# detect-ref.sh - smart ref detection for revdiff skill.
# source: .claude-plugin/skills/revdiff/scripts/detect-ref.sh (keep in sync)
# outputs structured info about the current repo state so the skill can decide
# what ref to use or whether to ask the user.
#
# auto-detects the VCS (jj → git → hg precedence, matching app/diff/vcs.go),
# populates the same set of fields regardless of which VCS backs the repo, and
# applies a shared decision block. the git code path's runtime output is
# byte-identical to the pre-refactor script on any git repo state.
#
# output fields:
#   branch: current branch name
#   main_branch: detected main/master branch name
#   is_main: true/false (whether current branch is main/master)
#   has_uncommitted: true/false
#   has_staged_only: true/false (changes are staged but nothing unstaged; git-only)
#   suggested_ref: the ref to use (empty = uncommitted, HEAD~1, main branch name, or --all-files for no-commits git repos)
#   use_staged: true/false (pass --staged to revdiff; git-only)
#   needs_ask: true/false (whether the skill should ask the user)

set -euo pipefail

# field defaults — each detect_<vcs> may overwrite these
branch="unknown"
main_branch=""
is_main="false"
has_uncommitted="false"
has_unstaged="false"
has_staged_only="false"
has_commits="true"

detect_git() {
    branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

    # detect main branch name from remote HEAD, fallback to master/main check
    main_branch=""
    if remote_head=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null); then
        main_branch="${remote_head##refs/remotes/origin/}"
    elif git show-ref --verify --quiet refs/heads/master 2>/dev/null; then
        main_branch="master"
    elif git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
        main_branch="main"
    fi

    is_main="false"
    if [ "$branch" = "$main_branch" ]; then
        is_main="true"
    fi

    has_uncommitted="false"
    if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi

    # distinguish staged-only vs unstaged changes
    has_unstaged="false"
    if ! git diff --quiet 2>/dev/null; then
        has_unstaged="true"
    fi
    has_staged_only="false"
    if [ "$has_uncommitted" = "true" ] && [ "$has_unstaged" = "false" ]; then
        if ! git diff --cached --quiet 2>/dev/null; then
            has_staged_only="true"
        fi
    fi

    # detect no-commits state (fresh repo after git init)
    has_commits="true"
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        has_commits="false"
    fi
}

detect_hg() {
    branch=$(hg branch 2>/dev/null || echo "unknown")

    # hg has no remote HEAD equivalent; "default" is the conventional main branch.
    main_branch="default"
    is_main="false"
    if [ "$branch" = "$main_branch" ]; then
        is_main="true"
    fi

    has_uncommitted="false"
    if [ -n "$(hg status 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi

    # hg has no staging area — staged-only is never true.
    has_staged_only="false"

    # detect no-commits state (fresh repo after hg init). `hg log -r .` always
    # succeeds because `.` resolves to the null revision (all-zeros) on an empty
    # repo, so check for any actual revisions via `all()` — empty output means
    # no commits yet.
    has_commits="true"
    if [ -z "$(hg log -r 'all()' -l 1 -T '.' 2>/dev/null)" ]; then
        has_commits="false"
    fi
}

# targets jj 0.18+ — parsing uses `jj log -T` templates and `jj diff --summary`,
# both of which have been spec-stable since the 0.18 release. separator-guard
# below handles bookmark template separator variance (space vs comma) across
# 0.18–0.30+ without pinning a specific version.
detect_jj() {
    # bookmarks on @; jj's @ is usually an anonymous change (empty template
    # output). fall back to a literal "@" so the branch field is never blank.
    branch=$(jj log -r @ --no-graph -T 'bookmarks' 2>/dev/null)
    if [ -z "$branch" ]; then
        branch="@"
    fi

    # detect main bookmark: try main, then master, then trunk. `jj log -r <name>`
    # exits non-zero on unresolvable names — more stable than parsing
    # `jj bookmark list` output (prefix + status markers vary by version).
    main_branch=""
    for candidate in main master trunk; do
        if jj log -r "$candidate" -l 1 --no-graph -T '.' >/dev/null 2>&1; then
            main_branch="$candidate"
            break
        fi
    done

    is_main="false"
    if [ -n "$main_branch" ]; then
        # nearest ancestor bookmark — the actual "am I on main" semantic, since
        # @ is usually an anonymous change with no bookmarks of its own.
        nearest=$(jj log --no-graph \
            -r "latest(heads(::@ & bookmarks()))" \
            -T 'bookmarks' 2>/dev/null)
        # bookmarks template separator varies by jj version (space or comma);
        # guard both forms so we don't need to pin a specific separator.
        case " $nearest " in *" $main_branch "*) is_main="true" ;; esac
        case ",$nearest," in *",$main_branch,"*) is_main="true" ;; esac
    fi

    # "uncommitted" = @ has changes vs @-. `jj diff -r @ --summary` has been
    # stable since early jj releases — empty stdout means @ == @-.
    has_uncommitted="false"
    if [ -n "$(jj diff -r @ --summary 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi

    # jj has no staging area; @ always exists so has_commits is always true.
    has_staged_only="false"
    has_commits="true"
}

apply_decision_logic() {
    # no-commits short-circuit fires first: on git, fall back to --all-files
    # (browses staged files); on hg/jj, ask the user because --all-files is
    # git-only in the binary (app/diff/directory.go uses `git ls-files`).
    # short-circuit deliberately precedes is_main/has_uncommitted so a fresh
    # hg repo with `?` untracked files doesn't misroute into the main+uncommitted arm.
    if [ "$has_commits" = "false" ]; then
        if [ "$vcs" = "git" ]; then
            suggested_ref="--all-files"
        else
            needs_ask="true"
        fi
    elif [ "$is_main" = "true" ]; then
        if [ "$has_uncommitted" = "true" ]; then
            if [ "$has_staged_only" = "true" ]; then
                use_staged="true" # staged-only changes on main
            fi
            suggested_ref="" # uncommitted changes on main
        else
            suggested_ref="HEAD~1" # last commit on main
        fi
    else
        if [ "$has_uncommitted" = "true" ]; then
            needs_ask="true" # ambiguous: uncommitted on feature branch
            if [ "$has_staged_only" = "true" ]; then
                use_staged="true"
            fi
        else
            suggested_ref="$main_branch" # clean feature branch → diff against main
        fi
    fi
}

# top-level VCS probe — order matches app/diff/vcs.go (jj first so it wins
# when colocated with .git). command -v guards short-circuit away subprocess
# spawns and "command not found" noise on the common git-only path.
vcs="unknown"
if command -v jj >/dev/null 2>&1 && jj root >/dev/null 2>&1; then
    vcs="jj"
elif git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    vcs="git"
elif command -v hg >/dev/null 2>&1 && hg root >/dev/null 2>&1; then
    vcs="hg"
fi

suggested_ref=""
needs_ask="false"
use_staged="false"

case "$vcs" in
git) detect_git ;;
hg) detect_hg ;;
jj) detect_jj ;;
*)
    # no VCS detected — fall through with empty fields so the skill asks
    branch="unknown"
    main_branch=""
    is_main="false"
    has_uncommitted="false"
    has_staged_only="false"
    has_commits="true"
    needs_ask="true"
    ;;
esac

apply_decision_logic

echo "branch: $branch"
echo "main_branch: $main_branch"
echo "is_main: $is_main"
echo "has_uncommitted: $has_uncommitted"
echo "has_staged_only: $has_staged_only"
echo "suggested_ref: $suggested_ref"
echo "use_staged: $use_staged"
echo "needs_ask: $needs_ask"
