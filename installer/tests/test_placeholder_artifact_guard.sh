#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_manifest="$(mktemp)"
tmp_err="$(mktemp)"
trap 'rm -rf "$tmp_bundle" "$tmp_root"; rm -f "$tmp_manifest" "$tmp_err"' EXIT

env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null

if env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install-profile proxmox-host "$tmp_root" "$tmp_bundle" >/dev/null 2>"$tmp_err"; then
  echo "expected install-profile to fail without VEILKEY_INSTALLER_GITLAB_API_BASE when manifest still has placeholder URLs" >&2
  exit 1
fi

grep -F "placeholder artifact_url requires VEILKEY_INSTALLER_GITLAB_API_BASE or a rewritten manifest URL" "$tmp_err" >/dev/null

echo "ok: placeholder artifact_url guard"
