#!/usr/bin/env bash
# detect-ref.sh - smart ref detection for revdiff skill.
# outputs structured info about the current git state so the skill can decide
# what ref to use or whether to ask the user.
#
# output fields:
#   branch: current branch name
#   main_branch: detected main/master branch name
#   is_main: true/false (whether current branch is main/master)
#   has_uncommitted: true/false
#   has_staged_only: true/false (changes are staged but nothing unstaged)
#   suggested_ref: the ref to use (empty = uncommitted, HEAD~1, main branch name, or --all-files for no-commits repos)
#   use_staged: true/false (pass --staged to revdiff)
#   needs_ask: true/false (whether the skill should ask the user)

set -euo pipefail

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

# decision logic
suggested_ref=""
needs_ask="false"

use_staged="false"
if [ "$has_commits" = "false" ]; then
    suggested_ref="--all-files" # no commits yet, browse staged files
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

echo "branch: $branch"
echo "main_branch: $main_branch"
echo "is_main: $is_main"
echo "has_uncommitted: $has_uncommitted"
echo "has_staged_only: $has_staged_only"
echo "suggested_ref: $suggested_ref"
echo "use_staged: $use_staged"
echo "needs_ask: $needs_ask"
