#!/usr/bin/env bash
set -euo pipefail
# launchers must enable exit-code-on-annotations via env, not a CLI flag
if [ "${REVDIFF_EXIT_CODE_ON_ANNOTATIONS:-}" != "true" ]; then
    echo "fake-revdiff: REVDIFF_EXIT_CODE_ON_ANNOTATIONS not set by launcher" >&2
    exit 3
fi
# records what the launcher forwarded, so a test can see editor env reaching revdiff
if [ -n "${FAKE_ENV_FILE:-}" ]; then
    printf '%s|%s' "${EDITOR:-missing}" "${VISUAL:-missing}" > "$FAKE_ENV_FILE"
fi
# records what reached revdiff's stdin, so a test can see a piped diff arriving
if [ -n "${FAKE_STDIN_FILE:-}" ]; then
    cat > "$FAKE_STDIN_FILE"
    # the launcher's spool is gone by the time the test looks, so its mode is captured here
    ls -l "${TMPDIR:-/tmp}"/revdiff-err-*.stdin > "$FAKE_STDIN_FILE.mode" 2>/dev/null || true
fi
if [ -n "${FAKE_STDERR:-}" ]; then
    printf "%s" "$FAKE_STDERR" >&2
fi
# lets a test hold a review open: the script has started, the sentinel is not yet written
if [ -n "${FAKE_REVDIFF_DELAY:-}" ]; then
    sleep "$FAKE_REVDIFF_DELAY"
fi
out=""
for arg in "$@"; do
    case "$arg" in
        --output=*) out="${arg#--output=}" ;;
    esac
done
if [ -n "$out" ] && [ "${FAKE_WRITE_OUTPUT:-1}" != "0" ]; then
    printf "%s" "${FAKE_OUTPUT:-}" > "$out"
fi
exit "${FAKE_RC:-0}"
