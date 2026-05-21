#!/usr/bin/env bash
set -euo pipefail
# launchers must enable exit-code-on-annotations via env, not a CLI flag
if [ "${REVDIFF_EXIT_CODE_ON_ANNOTATIONS:-}" != "true" ]; then
    echo "fake-revdiff: REVDIFF_EXIT_CODE_ON_ANNOTATIONS not set by launcher" >&2
    exit 3
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
