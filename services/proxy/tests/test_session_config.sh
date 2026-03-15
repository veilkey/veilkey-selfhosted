#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

TEST_HUB_IP="10.0.0.1"
TEST_HOSTVAULT_IP="10.0.0.2"

cfg="$(mktemp)"
trap 'rm -f "$cfg"' EXIT
sed -e "s/<HUB_IP>/${TEST_HUB_IP}/g" -e "s/<HOSTVAULT_IP>/${TEST_HOSTVAULT_IP}/g" deploy/host/session-tools.toml.example > "$cfg"

export VEILKEY_SESSION_TOOLS_TOML="$cfg"

out="$(deploy/shared/veilkey-session-config tool-bin codex)"
assert_contains "$out" "codex"

out="$(deploy/shared/veilkey-session-config tool-proxy-url codex)"
assert_eq "$out" "http://127.0.0.1:18081"

out="$(deploy/shared/veilkey-session-config proxy-plaintext-action codex)"
assert_eq "$out" "issue-temp-and-block"

out="$(deploy/shared/veilkey-session-config shell-exports)"
assert_contains "$out" "VEILKEY_PROXY_URL="
assert_contains "$out" "HTTP_PROXY="
assert_contains "$out" "VEILKEY_LOCALVAULT_URL='http://127.0.0.1:10180'"
assert_contains "$out" "VEILKEY_HUB_URL='http://${TEST_HUB_IP}:10180'"
assert_contains "$out" "VEILKEY_HOSTVAULT_URL='http://${TEST_HOSTVAULT_IP}:10181'"
assert_contains "$out" "VEILKEY_KEYCENTER_URL="
out_tool="$(deploy/shared/veilkey-session-config tool-shell-exports codex)"
assert_contains "$out_tool" "NO_PROXY="
assert_contains "$out_tool" "${TEST_HUB_IP}"
assert_contains "$out_tool" "${TEST_HOSTVAULT_IP}"
assert_contains "$out_tool" "127.0.0.1"

echo "ok: session-config"
