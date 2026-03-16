#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_manifest="$(mktemp)"
trap 'rm -rf "$tmp_bundle" "$tmp_root"; rm -f "$tmp_manifest"' EXIT

export VEILKEY_INSTALLER_GITLAB_API_BASE="${VEILKEY_INSTALLER_GITLAB_API_BASE:-https://gitlab.60.internal.kr/api/v4}"
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install-profile proxmox-host "$tmp_root" "$tmp_bundle" >/dev/null

test -x "$tmp_root/usr/local/bin/veilroot-shell"
test -x "$tmp_root/usr/local/bin/verify-veilroot-session"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-session"
test -f "$tmp_root/etc/veilkey/proxy.env"
test -f "$tmp_root/opt/veilkey/installer/install.env"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host' "$tmp_root/opt/veilkey/installer/install.env" >/dev/null

host_localvault_bundle="$(mktemp -d)"
host_localvault_root="$(mktemp -d)"
trap 'rm -rf "$tmp_bundle" "$tmp_root" "$host_localvault_bundle" "$host_localvault_root"; rm -f "$tmp_manifest"' EXIT

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install-profile proxmox-host-localvault "$host_localvault_root" "$host_localvault_bundle" >/dev/null
test -x "$host_localvault_root/usr/local/bin/veilkey-localvault"
test -f "$host_localvault_root/etc/veilkey/localvault.env.example"
grep -F 'VEILKEY_INSTALLER_PROFILE=proxmox-host-localvault' "$host_localvault_root/opt/veilkey/installer/install.env" >/dev/null

echo "ok: install-profile command"
