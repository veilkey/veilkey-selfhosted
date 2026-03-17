#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_manifest="$(mktemp)"
tmp_stage="$(mktemp -d)"
tmp_downloads="$(mktemp -d)"
trap 'rm -f "$tmp_manifest"; rm -rf "$tmp_stage" "$tmp_downloads"' EXIT

export VEILKEY_INSTALLER_GITLAB_API_BASE="${VEILKEY_INSTALLER_GITLAB_API_BASE:-https://gitlab.60.internal.kr/api/v4}"
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null

plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan proxmox-host)"
stage_plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan-stage proxmox-host)"
download_plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan-download proxmox-host)"

printf '%s\n' "$plan" | grep -F "proxy" >/dev/null
printf '%s\n' "$stage_plan" | grep -F "project=veilkey/veilkey-proxy" >/dev/null
printf '%s\n' "$stage_plan" | grep -F "type=source_archive" >/dev/null
printf '%s\n' "$stage_plan" | grep -F "stage_assets=deploy/lxc/install-proxy-lxc.sh,deploy/lxc/verify-proxy-lxc.sh,deploy/lxc/veilkey-egress-proxy@.service,deploy/lxc/veilkey-proxy-launch,cmd/veilkey-session-config/main.go,deploy/host/session-tools.toml.example" >/dev/null
printf '%s\n' "$stage_plan" | grep -F "post_install_verify=/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh" >/dev/null
printf '%s\n' "$download_plan" | grep -F "veilkey-proxy-" >/dev/null

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh stage proxmox-host "$tmp_stage" >/dev/null
grep -F 'component=proxy;project=veilkey/veilkey-proxy' "$tmp_stage/state/install-plan.env" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh download proxmox-host "$tmp_downloads" >/dev/null
find "$tmp_downloads" -maxdepth 1 -type f -name 'veilkey-proxy-*.tar.gz' | grep -q .

single_plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan proxmox-lxc-allinone)"
printf '%s\n' "$single_plan" | grep -F "proxy" >/dev/null

echo "ok: proxy component manifest/stage"
