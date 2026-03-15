#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

out="$(deploy/host/install-user-boundary.sh root /etc/veilkey/session-tools.toml 2>&1 || true)"
assert_contains "$out" "internal session bootstrap helper"
assert_contains "$out" "install-veilroot-boundary.sh"

echo "ok: install-user-boundary"
