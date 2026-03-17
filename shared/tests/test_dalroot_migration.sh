#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

# ═══════════════════════════════════════════════════════════════════
# Verify that all dalroot legacy code has been fully removed.
# ═══════════════════════════════════════════════════════════════════

script="$PWD/deploy/host/install-veilroot-boundary.sh"

if grep -q 'dalroot' "$script"; then
  fail "install-veilroot-boundary.sh still contains dalroot references"
fi

echo "ok: no dalroot references in install-veilroot-boundary.sh"

for f in deploy/host/*.sh deploy/shared/*; do
  [[ -f "$f" ]] || continue
  if grep -q 'dalroot' "$f"; then
    fail "$f still contains dalroot references"
  fi
done

echo "ok: no dalroot references in any deploy script"
echo ""
echo "all dalroot removal checks passed"
