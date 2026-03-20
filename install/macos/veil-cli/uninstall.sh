#!/bin/bash
set -euo pipefail

# veil-cli uninstaller for macOS
#
# Usage:
#   bash install/macos/veil-cli/uninstall.sh

echo "=== veil-cli uninstaller ==="
echo ""

echo "[1/2] Removing npm package..."
npm uninstall -g veilkey-cli 2>/dev/null || true
echo "  Removed."

echo "[2/2] Cleaning .veilkey/env..."
if [ -f ".veilkey/env" ]; then
    rm -f .veilkey/env
    echo "  Removed .veilkey/env"
else
    echo "  .veilkey/env not found (skip)"
fi

echo ""
echo "=== veil-cli uninstall complete ==="
echo ""
