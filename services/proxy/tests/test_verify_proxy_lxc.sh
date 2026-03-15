#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

script="$(cat deploy/lxc/verify-proxy-lxc.sh)"
assert_not_contains "$script" "python3"
assert_contains "$script" "/dev/tcp/127.0.0.1/18081"

echo "ok: verify-proxy-lxc has no python dependency"
