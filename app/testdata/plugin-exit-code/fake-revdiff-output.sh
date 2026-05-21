#!/usr/bin/env bash
set -euo pipefail
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
