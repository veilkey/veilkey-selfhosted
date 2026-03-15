#!/usr/bin/env bash
set -euo pipefail

launcher_bin="${VEILKEY_VEILROOT_LAUNCHER:-/usr/local/bin/veilkey-veilroot-session}"
session_config_bin="${VEILKEY_SESSION_CONFIG_BIN:-/usr/local/bin/veilkey-session-config}"

profile="${1:-}"
if [[ -n "$profile" ]]; then
  :
else
  profile="$("$session_config_bin" veilroot-default-profile)"
fi

unit_prefix="${VEILKEY_VEILROOT_UNIT_PREFIX:-$("$session_config_bin" veilroot-unit-prefix)}"
scope_name="${VEILKEY_VEILROOT_SCOPE:-${unit_prefix}-${profile}}"

out="$("$launcher_bin" "$profile")"
printf '%s\n' "$out"

expected_proxy="$("$session_config_bin" tool-proxy-url "$profile")"
printf '%s\n' "$out" | grep -q "^VEILKEY_PROXY_URL=${expected_proxy}$"
printf '%s\n' "$out" | grep -q '^VEILKEY_VEILROOT=1$'
printf '%s\n' "$out" | grep -q "^VEILKEY_VEILROOT_PROFILE=${profile}$"
printf '%s\n' "$out" | grep -q "${scope_name}"

echo "ok: veilroot session verify (${profile})"
