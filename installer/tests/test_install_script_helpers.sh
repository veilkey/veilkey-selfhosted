#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

if grep -Eq 'python3|python -' ./install.sh; then
  echo "expected installer/install.sh to avoid python helpers" >&2
  exit 1
fi

tmp_manifest="$(mktemp)"
trap 'rm -f "$tmp_manifest"' EXIT

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh doctor >/dev/null 2>&1 || {
  echo "expected installer doctor to run after python helper removal" >&2
  exit 1
}

echo "ok: installer script helpers"
