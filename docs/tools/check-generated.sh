#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
tmp_before="$(mktemp -d)"

cleanup() {
  rm -rf "${tmp_before}"
}

trap cleanup EXIT

cd "${repo_root}"

if [ -d docs/generated ]; then
  mkdir -p "${tmp_before}/generated"
  cp -a docs/generated/. "${tmp_before}/generated/" 2>/dev/null || true
fi

bash docs/tools/generate.sh >/dev/null

if ! diff -ru "${tmp_before}/generated" docs/generated >/dev/null 2>&1; then
  echo "Generated docs are stale. Run: bash docs/tools/generate.sh"
  exit 1
fi

if [ ! -f docs/generated/summary.md ]; then
  echo "Generated docs are missing. Run: bash docs/tools/generate.sh"
  exit 1
fi

echo "Generated docs are up to date."
