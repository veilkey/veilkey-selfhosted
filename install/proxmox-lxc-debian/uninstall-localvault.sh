#!/bin/bash
set -euo pipefail

# Standalone LocalVault uninstaller for Proxmox host
#
# Usage:
#   bash install/proxmox-lxc-debian/uninstall-localvault.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO_ROOT="$(pwd)"
INSTALL_DIR="$REPO_ROOT/.localvault"
PID_FILE="$INSTALL_DIR/localvault.pid"

echo "=== LocalVault uninstaller (Proxmox host) ==="
echo ""

# [1/2] Stop process
echo "[1/2] Stopping LocalVault..."
if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
        kill "$PID" 2>/dev/null || true
        echo "  Stopped (PID: $PID)"
    else
        echo "  Not running"
    fi
    rm -f "$PID_FILE"
else
    echo "  No PID file found"
fi

# [2/2] Remove data
echo "[2/2] Install directory: $INSTALL_DIR"
if [ -d "$INSTALL_DIR" ]; then
    read -p "  Delete $INSTALL_DIR? (y/N) " confirm
    if [[ "$confirm" =~ ^[yY]$ ]]; then
        rm -rf "$INSTALL_DIR"
        echo "  Removed."
    else
        echo "  Kept. Remove manually: rm -rf $INSTALL_DIR"
    fi
else
    echo "  Not found (skip)"
fi

echo ""
echo "=== Uninstall complete ==="
echo ""
