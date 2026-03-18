#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/veilkey-cli" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'cli-args=%s\n' "$*" >> "${VEILKEY_WRAPPER_LOG}"
SCRIPT
chmod +x "$tmp/veilkey-cli"

cat >"$tmp/vk" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'vk-args=%s\n' "$*" >> "${VEILKEY_WRAPPER_LOG}"
SCRIPT
chmod +x "$tmp/vk"

log="$tmp/wrapper.log"
wrapper="$PWD/deploy/host/veilkey"

out="$(VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" --help)"
assert_contains "$out" "veilkey session"
assert_contains "$out" "veilkey encrypt"

VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" status
assert_contains "$(cat "$log")" "cli-args=status"

: >"$log"
VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" paste-mode off
assert_contains "$(cat "$log")" "cli-args=paste-mode off"

: >"$log"
VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" session codex
assert_contains "$(cat "$log")" "cli-args=wrap-pty codex"

: >"$log"
VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" encrypt
assert_contains "$(cat "$log")" "vk-args="

: >"$log"
VEILKEY_CLI_BIN="$tmp/veilkey-cli" VEILKEY_VK_BIN="$tmp/vk" VEILKEY_WRAPPER_LOG="$log" bash "$wrapper" resolve VK:TEMP:abcd
assert_contains "$(cat "$log")" "cli-args=resolve VK:TEMP:abcd"

echo "ok: veilkey wrapper"
