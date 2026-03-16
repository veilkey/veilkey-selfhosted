#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_manifest="$(mktemp)"
trap 'rm -rf "$tmp_bundle" "$tmp_root"; rm -f "$tmp_manifest"' EXIT

export VEILKEY_INSTALLER_GITLAB_API_BASE="${VEILKEY_INSTALLER_GITLAB_API_BASE:-https://gitlab.60.internal.kr/api/v4}"
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh bundle proxmox-host "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install proxmox-host "$tmp_root" "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh configure proxmox-host "$tmp_root" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh post-install-health "$tmp_root" >/dev/null

test -x "$tmp_root/usr/local/bin/veilkey-session-config"
test -x "$tmp_root/usr/local/bin/veilkey-proxy-launch"
test -x "$tmp_root/usr/local/bin/veilroot-shell"
test -x "$tmp_root/usr/local/bin/verify-veilroot-session"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-session"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-observe"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-egress-guard"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-curl"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-wget"
test -x "$tmp_root/usr/local/bin/veilkey-veilroot-http"
test -x "$tmp_root/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh"
test -x "$tmp_root/usr/local/lib/veilkey-proxy/install-veilroot-boundary.sh"
test -x "$tmp_root/usr/local/lib/veilkey-proxy/install-veilroot-codex.sh"
test -f "$tmp_root/etc/veilkey/proxy.env"
test -f "$tmp_root/etc/veilkey/proxy.env.example"
test -f "$tmp_root/etc/veilkey/session-tools.toml"
test -f "$tmp_root/etc/systemd/system/veilkey-egress-proxy@.service"
test -f "$tmp_root/etc/systemd/system/veilkey-veilroot-observe@.service"
test -f "$tmp_root/etc/systemd/system/veilkey-veilroot-egress-guard@.service"
test -f "$tmp_root/usr/local/share/veilkey/snippets/veilroot-veilkey-shell.sh"
grep -Fx 'veilkey-egress-proxy@default.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@codex.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@claude.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null
grep -Fx 'veilkey-egress-proxy@opencode.service' "$tmp_root/etc/veilkey/services.enabled" >/dev/null

echo "ok: proxy install layout"
