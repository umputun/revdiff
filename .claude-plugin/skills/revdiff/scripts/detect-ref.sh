#!/usr/bin/env bash
# detect-ref.sh - smart ref detection for revdiff skill.
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
has_staged_only="false"
has_commits="true"

detect_git() {
    branch=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")

    # detect main branch name from remote HEAD, fallback to master/main check
    main_branch=""
    local remote_head
    if remote_head=$(git symbolic-ref refs/remotes/origin/HEAD 2>/dev/null); then
        main_branch="${remote_head##refs/remotes/origin/}"
    elif git show-ref --verify --quiet refs/heads/master 2>/dev/null; then
        main_branch="master"
    elif git show-ref --verify --quiet refs/heads/main 2>/dev/null; then
        main_branch="main"
    fi

    if [ "$branch" = "$main_branch" ]; then
        is_main="true"
    fi

    if [ -n "$(git status --porcelain 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi

    # distinguish staged-only vs unstaged changes
    local has_unstaged="false"
    if ! git diff --quiet 2>/dev/null; then
        has_unstaged="true"
    fi
    if [ "$has_uncommitted" = "true" ] && [ "$has_unstaged" = "false" ]; then
        if ! git diff --cached --quiet 2>/dev/null; then
            has_staged_only="true"
        fi
    fi

    # detect no-commits state (fresh repo after git init)
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        has_commits="false"
    fi
}

detect_hg() {
    branch=$(hg branch 2>/dev/null || echo "unknown")

    # hg has no remote HEAD equivalent; "default" is the conventional main branch.
    main_branch="default"
    if [ "$branch" = "$main_branch" ]; then
        is_main="true"
    fi

    if [ -n "$(hg status 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi

    # detect no-commits state (fresh repo after hg init). `hg log -r .` always
    # resolves to null revision on empty repos, so use `all()` — empty means
    # no commits yet.
    if [ -z "$(hg log -r 'all()' -l 1 -T '.' 2>/dev/null)" ]; then
        has_commits="false"
    fi
}

# targets jj 0.18+ (spec-stable `jj log -T 'bookmarks'` and `jj diff --summary`).
detect_jj() {
    # bookmarks on @; @ is usually anonymous (empty template). strip newlines
    # that appear when @ has multiple bookmarks (template emits one per line).
    branch=$(jj log -r @ --no-graph -T 'bookmarks' 2>/dev/null | tr '\n' ' ' | sed 's/ *$//')
    if [ -z "$branch" ]; then
        branch="@"
    fi

    # detect main bookmark: try main, then master. `jj log -r <name>` exits
    # non-zero on unresolvable names.
    main_branch=""
    for candidate in main master; do
        if jj log -r "$candidate" -l 1 --no-graph -T '.' >/dev/null 2>&1; then
            main_branch="$candidate"
            break
        fi
    done

    if [ -n "$main_branch" ]; then
        # "am I on main" = @- (parent of working copy) is the main bookmark
        # itself. anonymous feature changes descend from main, so the old
        # "nearest ancestor bookmark" check mis-fired for them. compare
        # change_ids directly — analogous to git's `[ "$branch" = "$main" ]`.
        local main_id parent_id
        main_id=$(jj log -r "$main_branch" -l 1 --no-graph -T 'change_id' 2>/dev/null)
        parent_id=$(jj log -r @- -l 1 --no-graph -T 'change_id' 2>/dev/null)
        if [ -n "$main_id" ] && [ "$main_id" = "$parent_id" ]; then
            is_main="true"
        fi
    else
        # set needs_ask here (not in apply_decision_logic) because the decision
        # block would otherwise suggest an empty main_branch ref silently;
        # without a main bookmark there's nothing sensible to diff against.
        needs_ask="true"
    fi

    # uncommitted = @ has changes vs @-; empty `jj diff --summary` = no changes.
    if [ -n "$(jj diff -r @ --summary 2>/dev/null)" ]; then
        has_uncommitted="true"
    fi
    # jj has no staging area; @ always exists so has_commits stays true.
}

apply_decision_logic() {
    # no-commits short-circuit fires first: on git, fall back to --all-files
    # (browses staged files); on hg, ask the user since --all-files is not
    # supported for hg. jj always has @ so this branch is unreachable for jj.
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
elif command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
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
*) needs_ask="true" ;; # no VCS detected — defaults already set
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
