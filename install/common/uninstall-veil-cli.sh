#!/bin/bash
set -euo pipefail

# veil-cli uninstaller for Linux
#
# Usage:
#   bash install/common/uninstall-veil-cli.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

BIN_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.veilkey"

echo "=== veil-cli uninstaller (Linux) ==="
echo ""

# [1/2] Remove binaries
echo "[1/2] Removing binaries..."
for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$BIN_DIR/$bin" ]; then
        rm "$BIN_DIR/$bin"
        echo "  Removed $BIN_DIR/$bin"
    fi
done

# [2/2] Remove config
echo "[2/2] Removing config..."
if [ -d "$CONFIG_DIR" ]; then
    rm -rf "$CONFIG_DIR"
    echo "  Removed $CONFIG_DIR"
else
    echo "  $CONFIG_DIR not found (skip)"
fi

echo ""
echo "=== Uninstall complete ==="
echo ""
echo "If you added 'source ~/.veilkey/env' to your shell profile, remove that line manually."
echo ""
