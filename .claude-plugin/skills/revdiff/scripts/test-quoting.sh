#!/usr/bin/env bash
# tests for the sq() shell-quoting function used by launcher scripts.
# verifies that arguments survive an sh -c round-trip intact.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

PASS=0
FAIL=0

# source the real sq() from the shared file
source "$SCRIPT_DIR/shell-quote.sh"

# verify both launcher scripts source shell-quote.sh instead of defining sq() inline
assert_no_inline_sq() {
    local file="$1"
    local label="$2"
    if grep -q '^sq()' "$file"; then
        FAIL=$((FAIL + 1))
        printf "FAIL: %s defines sq() inline instead of sourcing shell-quote.sh\n" "$label" >&2
    else
        PASS=$((PASS + 1))
    fi
}

assert_no_inline_sq "$SCRIPT_DIR/launch-revdiff.sh" "launch-revdiff.sh"
assert_no_inline_sq "$REPO_ROOT/plugins/revdiff-planning/scripts/launch-plan-review.sh" "launch-plan-review.sh"

assert_sq_roundtrip() {
    local input="$1"
    local label="${2:-$1}"
    local quoted
    quoted=$(sq "$input")
    local result
    result=$(sh -c "printf '%s' $quoted")
    if [ "$result" = "$input" ]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        printf "FAIL: %s\n  input:    %s\n  quoted:   %s\n  got:      %s\n" "$label" "$input" "$quoted" "$result" >&2
    fi
}

# verify that multiple sq()-quoted args passed through sh -c preserve argument boundaries
assert_args_roundtrip() {
    local label="$1"
    shift
    local cmd=""
    local i=1
    for arg in "$@"; do
        cmd="${cmd:+$cmd }$(sq "$arg")"
    done
    # use printf with %s\n to print each arg on its own line, then compare
    local result
    result=$(sh -c "printf '%s\n' $cmd")
    local expected
    expected=$(printf '%s\n' "$@")
    if [ "$result" = "$expected" ]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        printf "FAIL: %s\n  expected:\n%s\n  got:\n%s\n" "$label" "$expected" "$result" >&2
    fi
}

# --- sq() round-trip tests ---

assert_sq_roundtrip "/usr/local/bin/revdiff" "simple path"
assert_sq_roundtrip "/path/with spaces/revdiff" "path with spaces"
assert_sq_roundtrip "/path/it's here/file" "path with single quote"
assert_sq_roundtrip "it's a 'quoted' thing" "multiple single quotes"
assert_sq_roundtrip '--config=/home/user/my config/revdiff.ini' "flag with spaces in value"
assert_sq_roundtrip '--only=file with spaces.txt' "only flag with spaces"
assert_sq_roundtrip 'HEAD~1' "git ref"
assert_sq_roundtrip 'feature/my branch' "branch name with space"
assert_sq_roundtrip 'file$name' "dollar sign in path"
assert_sq_roundtrip 'file`cmd`name' "backticks in path"
assert_sq_roundtrip 'file\name' "backslash in path"
assert_sq_roundtrip 'file"name' "double quote in path"
assert_sq_roundtrip 'file name  double' "multiple spaces"
assert_sq_roundtrip '' "empty string"
assert_sq_roundtrip 'a	b' "tab character"
assert_sq_roundtrip '*?.txt' "glob characters"
assert_sq_roundtrip '$(echo pwned)' "command substitution attempt"
assert_sq_roundtrip '/tmp/revdiff-output-AbC123' "typical mktemp path"

# --- multi-argument boundary tests ---

assert_args_roundtrip "two simple args" "/usr/bin/revdiff" "--wrap"
assert_args_roundtrip "arg with spaces preserved" "/usr/bin/revdiff" "--only=/path/with spaces/file.go" "--output=/tmp/out"
assert_args_roundtrip "single quotes in args" "/usr/bin/revdiff" "--only=it's here.txt" "--output=/tmp/file"
assert_args_roundtrip "mixed special chars" "/opt/rev diff/bin" "--config=/home/user's/cfg" 'HEAD~1'
assert_args_roundtrip "simulated full command" \
    "/usr/local/bin/revdiff" \
    "--config=/home/user/my config/revdiff.ini" \
    "--output=/tmp/revdiff-output-abc123" \
    "feature/my branch" \
    "--staged"

# --- heredoc expansion test ---
# verify that sq() output embedded in an unquoted heredoc produces a valid sh script

assert_heredoc_roundtrip() {
    local label="$1"
    local arg="$2"
    local script
    script=$(cat <<HEREDOC
#!/bin/sh
printf '%s' $(sq "$arg")
HEREDOC
    )
    local result
    result=$(sh -c "$script")
    if [ "$result" = "$arg" ]; then
        PASS=$((PASS + 1))
    else
        FAIL=$((FAIL + 1))
        printf "FAIL: heredoc %s\n  input:  %s\n  script: %s\n  got:    %s\n" "$label" "$arg" "$script" "$result" >&2
    fi
}

assert_heredoc_roundtrip "space in path" "/path/with spaces/file"
assert_heredoc_roundtrip "single quote in path" "/path/it's/file"
assert_heredoc_roundtrip "dollar sign" '/tmp/$HOME/file'

# --- report ---

printf "\n%d passed, %d failed\n" "$PASS" "$FAIL"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
