#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_root="$(mktemp -d)"
tmp_manifest="$tmp_root/manifest.toml"
tmp_proxy_err="$tmp_root/proxy.err"
tmp_host_bundle="$tmp_root/host.bundle"
tmp_host_root="$tmp_root/host.root"
tmp_host_localvault_bundle="$tmp_root/host-localvault.bundle"
tmp_host_localvault_root="$tmp_root/host-localvault.root"
tmp_host_localvault_bootstrap_bundle="$tmp_root/host-localvault-bootstrap.bundle"
tmp_host_localvault_bootstrap_root="$tmp_root/host-localvault-bootstrap.root"
tmp_runtime_bundle="$tmp_root/runtime.bundle"
tmp_runtime_root="$tmp_root/runtime.root"
tmp_lxc_bundle="$tmp_root/lxc.bundle"
tmp_lxc_root="$tmp_root/lxc.root"
tmp_stack_lxc_bundle="$tmp_root/stack-lxc.bundle"
tmp_stack_lxc_root="$tmp_root/stack-lxc.root"
tmp_stack_host_bundle="$tmp_root/stack-host.bundle"
tmp_stack_host_root="$tmp_root/stack-host.root"
mkdir -p \
  "$tmp_host_bundle" "$tmp_host_root" \
  "$tmp_host_localvault_bundle" "$tmp_host_localvault_root" \
  "$tmp_host_localvault_bootstrap_bundle" "$tmp_host_localvault_bootstrap_root" \
  "$tmp_runtime_bundle" "$tmp_runtime_root" \
  "$tmp_lxc_bundle" "$tmp_lxc_root" \
  "$tmp_stack_lxc_bundle" "$tmp_stack_lxc_root" \
  "$tmp_stack_host_bundle" "$tmp_stack_host_root"
trap 'rm -rf "$tmp_root"' EXIT

export VEILKEY_INSTALLER_GITLAB_API_BASE="${VEILKEY_INSTALLER_GITLAB_API_BASE:-https://gitlab.60.internal.kr/api/v4}"

# 외부 CI에서 내부 gitlab 접근 불가 시 skip
if [[ "${CI:-}" == "true" ]]; then
  if ! curl -sf --max-time 5 "${VEILKEY_INSTALLER_GITLAB_API_BASE}" >/dev/null 2>&1; then
    echo "skip: internal gitlab unreachable in CI"
    exit 0
  fi
fi

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-host-install.sh "$tmp_host_root" "$tmp_host_bundle" >/dev/null
test -x "$tmp_host_root/usr/local/bin/veilroot-shell"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host' "$tmp_host_root/opt/veilkey/installer/install.env" >/dev/null

VEILKEY_KEYCENTER_PASSWORD='stack-keycenter' \
VEILKEY_LOCALVAULT_PASSWORD='stack-localvault' \
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-allinone-stack-install.sh "$tmp_stack_lxc_root" "$tmp_stack_host_root" "$tmp_stack_lxc_bundle" "$tmp_stack_host_bundle" >/dev/null
test -x "$tmp_stack_lxc_root/usr/local/bin/veilkey-keycenter"
test -x "$tmp_stack_lxc_root/usr/local/bin/veilkey-localvault"
test -x "$tmp_stack_host_root/usr/local/bin/veilroot-shell"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-lxc-allinone' "$tmp_stack_lxc_root/opt/veilkey/installer/install.env" >/dev/null
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host-cli' "$tmp_stack_host_root/opt/veilkey/installer/install.env" >/dev/null

VEILKEY_KEYCENTER_PASSWORD='test-keycenter' \
VEILKEY_LOCALVAULT_PASSWORD='test-localvault' \
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-lxc-allinone-install.sh "$tmp_lxc_root" "$tmp_lxc_bundle" >/dev/null
test -x "$tmp_lxc_root/usr/local/bin/veilkey-keycenter"
test -x "$tmp_lxc_root/usr/local/bin/veilkey-localvault"
test -x "$tmp_lxc_root/usr/local/bin/veilroot-shell"
test -x "./scripts/proxmox-lxc-allinone-health.sh"
test -x "./scripts/proxmox-lxc-allinone-export-bootstrap.sh"
test -x "./scripts/proxmox-lxc-runtime-health.sh"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-lxc-allinone' "$tmp_lxc_root/opt/veilkey/installer/install.env" >/dev/null
grep -F 'VEILKEY_ADDR=:10181' "$tmp_lxc_root/etc/veilkey/keycenter.env" >/dev/null
grep -F 'VEILKEY_ADDR=:10180' "$tmp_lxc_root/etc/veilkey/localvault.env" >/dev/null
grep -F 'VEILKEY_KEYCENTER_URL=http://127.0.0.1:10181' "$tmp_lxc_root/etc/veilkey/localvault.env" >/dev/null
./scripts/proxmox-lxc-allinone-health.sh "$tmp_lxc_root" >/dev/null

if VEILKEY_ENABLE_PROXY=1 \
  VEILKEY_KEYCENTER_PASSWORD='blocked-keycenter' \
  VEILKEY_LOCALVAULT_PASSWORD='blocked-localvault' \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-lxc-allinone-install.sh "$tmp_lxc_root" "$tmp_lxc_bundle" >"$tmp_proxy_err" 2>&1; then
  echo "expected proxmox-lxc-allinone proxy enable to be rejected" >&2
  exit 1
fi
grep -F 'proxmox-host-cli' "$tmp_proxy_err" >/dev/null

rm -f ./components.toml
VEILKEY_KEYCENTER_PASSWORD='e2e-keycenter' \
VEILKEY_LOCALVAULT_PASSWORD='e2e-localvault' \
./scripts/proxmox-lxc-allinone-install.sh "$tmp_lxc_root" "$tmp_lxc_bundle" >/dev/null
test -f ./components.toml
test -f "$tmp_lxc_root/etc/veilkey/bootstrap/ssh/veilkey-admin"
test -f "$tmp_lxc_root/etc/veilkey/bootstrap/ssh/veilkey-admin.pub"
test -f "$tmp_lxc_root/etc/veilkey/bootstrap/ssh/veilkey-admin.enc"
./scripts/proxmox-lxc-allinone-purge.sh "$tmp_lxc_root" >/dev/null
test ! -e "$tmp_lxc_root/etc/veilkey/bootstrap/ssh/veilkey-admin"
test ! -e "$tmp_lxc_root/usr/local/bin/veilkey-keycenter"
test ! -e "$tmp_lxc_root/usr/local/bin/veilkey-localvault"

VEILKEY_LOCALVAULT_PASSWORD='e2e-localvault' \
VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180' \
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-host-localvault/install.sh --activate "$tmp_host_localvault_root" "$tmp_host_localvault_bundle" >/dev/null
test -x "$tmp_host_localvault_root/usr/local/bin/veilkey-localvault"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host-localvault' "$tmp_host_localvault_root/opt/veilkey/installer/install.env" >/dev/null

rm -f ./components.toml
VEILKEY_LOCALVAULT_PASSWORD='e2e-localvault' \
VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180' \
./scripts/proxmox-host-localvault/install.sh "$tmp_host_localvault_bootstrap_root" "$tmp_host_localvault_bootstrap_bundle" >/dev/null
test -f ./components.toml
test -x "$tmp_host_localvault_bootstrap_root/usr/local/bin/veilkey-localvault"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host-localvault' "$tmp_host_localvault_bootstrap_root/opt/veilkey/installer/install.env" >/dev/null

VEILKEY_LOCALVAULT_PASSWORD='runtime-localvault' \
VEILKEY_KEYCENTER_URL='http://127.0.0.1:10181' \
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-lxc-runtime-install.sh "$tmp_runtime_root" "$tmp_runtime_bundle" >/dev/null
./scripts/proxmox-lxc-runtime-health.sh "$tmp_runtime_root" >/dev/null

echo "ok: proxmox wrapper commands"
