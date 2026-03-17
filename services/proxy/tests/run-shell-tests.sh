#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bin_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_bin_dir"' EXIT

if [[ -z "${VEILKEY_SESSION_CONFIG_BIN_REAL:-}" ]]; then
  go build -o "${tmp_bin_dir}/veilkey-session-config" ./cmd/veilkey-session-config
  export VEILKEY_SESSION_CONFIG_BIN_REAL="${tmp_bin_dir}/veilkey-session-config"
fi

tests=(
  tests/test_session_config.sh
  tests/test_doctor_veilkey.sh
  tests/test_cleanup_proxy_logs.sh
  tests/test_check_proxy_audit.sh
  tests/test_install_user_boundary.sh
  tests/test_session_launch_fail_closed.sh
  tests/test_veilroot_session.sh
  tests/test_veilroot_egress_guard.sh
  tests/test_veilroot_shell_hook.sh
  tests/test_verify_proxy_lxc.sh
)

for t in "${tests[@]}"; do
  echo "== $t =="
  bash "$t"
done
