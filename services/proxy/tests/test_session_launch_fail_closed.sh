#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

script="$(cat deploy/host/install-user-boundary.sh)"
assert_contains "$script" 'if [[ "${VEILKEY_VEILROOT:-}" == "1" ]]; then'
assert_contains "$script" "refusing direct exec without a verified Veil session boundary"
assert_not_contains "$script" 'exec /usr/local/bin/veilkey session "$real_bin" "$@"'

echo "ok: session launch fail closed"
