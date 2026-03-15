#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_manifest="$(mktemp)"
trap 'rm -rf "$tmp_bundle" "$tmp_root"; rm -f "$tmp_manifest"' EXIT

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh bundle proxmox-lxc-allinone "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install proxmox-lxc-allinone "$tmp_root" "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh configure proxmox-lxc-allinone "$tmp_root" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh post-install-health "$tmp_root" >/dev/null

test -x "$tmp_root/usr/local/bin/veilkey"
test -x "$tmp_root/usr/local/bin/veilkey-keycenter"
test -x "$tmp_root/usr/local/bin/veilkey-localvault"
test -x "$tmp_root/usr/local/bin/veilkey-cli"
test -x "$tmp_root/usr/local/bin/veilkey-session-config"
test -x "$tmp_root/usr/local/bin/veilkey-proxy-launch"
test -x "$tmp_root/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh"
test -f "$tmp_root/etc/veilkey/proxy.env"
test -f "$tmp_root/etc/veilkey/proxy.env.example"
test -f "$tmp_root/etc/veilkey/session-tools.toml"
grep -Fx 'veilkey-keycenter.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-localvault.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@default.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@codex.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@claude.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@opencode.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null

echo "ok: proxmox-lxc-allinone includes proxy boundary"
