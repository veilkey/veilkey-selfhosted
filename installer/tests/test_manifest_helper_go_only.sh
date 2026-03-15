#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

grep -Fq 'GO_BIN                       Go binary used for manifest parsing helper' install.sh
if grep -Fq 'PYTHON_BIN' install.sh; then
  echo "unexpected python fallback in install.sh" >&2
  exit 1
fi
if grep -Fq 'installer_manifest.py' install.sh; then
  echo "unexpected python manifest helper path in install.sh" >&2
  exit 1
fi

tmp_manifest="$(mktemp)"
trap 'rm -f "$tmp_manifest"' EXIT
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null

echo "ok: manifest helper is go-only"
