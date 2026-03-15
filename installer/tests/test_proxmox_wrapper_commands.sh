#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_manifest="$(mktemp)"
tmp_host_bundle="$(mktemp -d)"
tmp_host_root="$(mktemp -d)"
tmp_host_localvault_bundle="$(mktemp -d)"
tmp_host_localvault_root="$(mktemp -d)"
tmp_host_localvault_bootstrap_bundle="$(mktemp -d)"
tmp_host_localvault_bootstrap_root="$(mktemp -d)"
tmp_runtime_bundle="$(mktemp -d)"
tmp_runtime_root="$(mktemp -d)"
tmp_lxc_bundle="$(mktemp -d)"
tmp_lxc_root="$(mktemp -d)"
trap 'rm -f "$tmp_manifest"; rm -rf "$tmp_host_bundle" "$tmp_host_root" "$tmp_host_localvault_bundle" "$tmp_host_localvault_root" "$tmp_host_localvault_bootstrap_bundle" "$tmp_host_localvault_bootstrap_root" "$tmp_runtime_bundle" "$tmp_runtime_root" "$tmp_lxc_bundle" "$tmp_lxc_root"' EXIT

export VEILKEY_INSTALLER_GITLAB_API_BASE="${VEILKEY_INSTALLER_GITLAB_API_BASE:-https://gitlab.60.internal.kr/api/v4}"
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./scripts/proxmox-host-install.sh "$tmp_host_root" "$tmp_host_bundle" >/dev/null
test -x "$tmp_host_root/usr/local/bin/veilroot-shell"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host' "$tmp_host_root/opt/veilkey/installer/install.env" >/dev/null

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
