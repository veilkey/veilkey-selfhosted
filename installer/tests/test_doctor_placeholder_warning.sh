#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_manifest="$(mktemp)"
tmp_out="$(mktemp)"
trap 'rm -f "$tmp_manifest" "$tmp_out"' EXIT

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh doctor >"$tmp_out" 2>&1
grep -F "WARNING: manifest contains placeholder artifact URLs; set VEILKEY_INSTALLER_GITLAB_API_BASE or rewrite the manifest before bundle/download/install-profile" "$tmp_out" >/dev/null

VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4" \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh doctor >"$tmp_out" 2>&1
if grep -F "WARNING: manifest contains placeholder artifact URLs" "$tmp_out" >/dev/null; then
  echo "doctor should not warn when VEILKEY_INSTALLER_GITLAB_API_BASE is set" >&2
  exit 1
fi

echo "ok: doctor placeholder warning"
