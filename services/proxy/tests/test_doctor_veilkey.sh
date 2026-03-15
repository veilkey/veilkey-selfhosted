#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
args="$*"
case "$args" in
  *127.0.0.1:10180/health*|*${TEST_HUB_IP}:10180/health*|*${TEST_HOSTVAULT_IP}:10180/health*)
    printf '%s' '{"status":"ok"}'
    ;;
  *)
    echo "unexpected curl: $*" >&2
    exit 1
    ;;
esac
EOF

cat >"$tmp/vibe_lxc_ops" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "VMID=$1 CMD=$2" >>"$TEST_STATE_DIR/vibe.log"
if [[ "$2" == *"/var/log/veilkey-proxy/default-rewrite.jsonl"* ]]; then
  printf '%s\n' "[]"
elif [[ "$2" == *"/api/agents/veilkey-hostvault/secrets"* ]]; then
  printf '%s' '{"scope":"TEMP","status":"temp","token":"VK:TEMP:testref"}'
else
  printf '%s\n' "mock"
fi
EOF

cat >"$tmp/verify-veilroot-session" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "veilroot-ok"
EOF

chmod +x "$tmp/curl" "$tmp/vibe_lxc_ops" "$tmp/verify-veilroot-session"
export PATH="$tmp:$PATH"
export TEST_STATE_DIR="$tmp"
TEST_HUB_IP="10.0.0.1"
TEST_HOSTVAULT_IP="10.0.0.2"
export VEILKEY_LOCALVAULT_HEALTH_URL="http://127.0.0.1:10180/health"
export VEILKEY_KEYCENTER_HEALTH_URL="http://${TEST_HUB_IP}:10180/health"
export VEILKEY_HOSTVAULT_HEALTH_URL="http://${TEST_HOSTVAULT_IP}:10180/health"
export VEILKEY_KEYCENTER_VMID="999001"
export VEILKEY_VEILROOT_VERIFY_BIN="$tmp/verify-veilroot-session"
export VEILKEY_VEILROOT_USER="root"
export VEILKEY_VEILROOT_CODEX_CONFIG="$tmp/codex-config.toml"
printf '%s\n' "x=1" >"$tmp/codex-config.toml"

sed 's#script_dir=.*#script_dir="'"$tmp"'"#' deploy/host/doctor-veilkey.sh >"$tmp/doctor-veilkey.sh"
chmod +x "$tmp/doctor-veilkey.sh"

out="$("$tmp/doctor-veilkey.sh" 100208 2)"
assert_contains "$out" "skipped"
assert_not_contains "$out" "temp issuance OK"
assert_contains "$out" "recent rewrite scope sanity"

out="$("$tmp/doctor-veilkey.sh" --check-temp-issuance 100208 1)"
assert_contains "$out" "temp issuance OK"
assert_file_contains "$tmp/vibe.log" "VMID=999001"

echo "ok: doctor-veilkey"
