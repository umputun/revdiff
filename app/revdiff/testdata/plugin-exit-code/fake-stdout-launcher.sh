#!/usr/bin/env bash
set -euo pipefail
printf "%s" "${FAKE_OUTPUT:-}"
exit "${FAKE_RC:-0}"
