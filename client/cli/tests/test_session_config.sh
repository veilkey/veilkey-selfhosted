#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

TEST_KC_HOST="10.0.0.1"
TEST_KC_URL="http://${TEST_KC_HOST}:10180"

cfg="$(mktemp)"
trap 'rm -f "$cfg"' EXIT
cp deploy/host/session-tools.toml.example "$cfg"
sed -i "s|keycenter_url = .*|keycenter_url = \"${TEST_KC_URL}\"|" "$cfg"

export VEILKEY_SESSION_TOOLS_TOML="$cfg"
unset VEILKEY_PROXY_URL HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

out="$(deploy/shared/veilkey-session-config tool-bin codex)"
assert_contains "$out" "codex"

out="$(deploy/shared/veilkey-session-config tool-proxy-url codex)"
assert_eq "$out" "http://127.0.0.1:18080"

out="$(deploy/shared/veilkey-session-config proxy-plaintext-action codex)"
assert_eq "$out" "issue-temp-and-block"

out="$(deploy/shared/veilkey-session-config shell-exports)"
assert_contains "$out" "VEILKEY_PROXY_URL="
assert_contains "$out" "HTTP_PROXY="
assert_contains "$out" "VEILKEY_LOCALVAULT_URL='http://127.0.0.1:10180'"
assert_contains "$out" "VEILKEY_KEYCENTER_URL='${TEST_KC_URL}'"
out_tool="$(deploy/shared/veilkey-session-config tool-shell-exports codex)"
assert_contains "$out_tool" "NO_PROXY="
assert_contains "$out_tool" "$TEST_KC_HOST"
assert_contains "$out_tool" "127.0.0.1"

out_override="$(VEILKEY_PROXY_URL='http://10.9.8.7:28080' deploy/shared/veilkey-session-config tool-proxy-url codex)"
assert_eq "$out_override" "http://10.9.8.7:28080"

echo "ok: session-config"
