#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

TEST_KC_HOST="10.0.0.1"
TEST_KC_URL="http://${TEST_KC_HOST}:10180"
TEST_LV_URL="http://10.0.0.2:10180"
TEST_PROXY_URL="http://10.0.0.3:18080"

cfg="$(mktemp)"
bin="$(mktemp)"
trap 'rm -f "$cfg" "$bin"' EXIT
go build -o "$bin" ./cmd/veilkey-session-config
cat >"$cfg" <<EOF
[proxy.default]
listen = "10.0.0.3:18080"
url = "${TEST_PROXY_URL}"
no_proxy = "127.0.0.1,localhost"
plaintext_action = "issue-temp-and-block"
allow_hosts_enabled = false
allow_hosts = []

[proxy.tools.codex]
url = "${TEST_PROXY_URL}"
plaintext_action = "issue-temp-and-block"

[tools.codex]
bin = "/usr/bin/codex"
proxy = "codex"

[veilroot]
default_profile = "codex"
unit_prefix = "veilroot"

[veilkey]
localvault_url = "${TEST_LV_URL}"
keycenter_url = "${TEST_KC_URL}"
EOF

export VEILKEY_SESSION_TOOLS_TOML="$cfg"
export VEILKEY_SESSION_CONFIG_BIN_REAL="$bin"
unset VEILKEY_PROXY_URL HTTP_PROXY HTTPS_PROXY ALL_PROXY http_proxy https_proxy all_proxy

out="$(deploy/shared/veilkey-session-config tool-bin codex)"
assert_contains "$out" "codex"

out="$(deploy/shared/veilkey-session-config tool-proxy-url codex)"
assert_eq "$out" "${TEST_PROXY_URL}"

out="$(deploy/shared/veilkey-session-config proxy-plaintext-action codex)"
assert_eq "$out" "issue-temp-and-block"

out="$(deploy/shared/veilkey-session-config shell-exports)"
assert_contains "$out" "VEILKEY_PROXY_URL="
assert_contains "$out" "HTTP_PROXY="
assert_contains "$out" "VEILKEY_LOCALVAULT_URL='${TEST_LV_URL}'"
assert_contains "$out" "VEILKEY_KEYCENTER_URL='${TEST_KC_URL}'"
out_tool="$(deploy/shared/veilkey-session-config tool-shell-exports codex)"
assert_contains "$out_tool" "NO_PROXY="
assert_contains "$out_tool" "$TEST_KC_HOST"
assert_contains "$out_tool" "127.0.0.1"

out_override="$(VEILKEY_PROXY_URL='http://10.9.8.7:28080' deploy/shared/veilkey-session-config tool-proxy-url codex)"
assert_eq "$out_override" "http://10.9.8.7:28080"

echo "ok: session-config"
