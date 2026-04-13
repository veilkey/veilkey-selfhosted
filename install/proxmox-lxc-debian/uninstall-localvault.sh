#!/bin/bash
set -euo pipefail

# Standalone LocalVault uninstaller for Proxmox host / LXC
#
# Usage:
#   bash install/proxmox-lxc-debian/uninstall-localvault.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO_ROOT="$(pwd)"
INSTALL_DIR="$REPO_ROOT/.localvault"
SERVICE_NAME="veilkey-localvault"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
PID_FILE="$INSTALL_DIR/localvault.pid"

echo "=== LocalVault uninstaller ==="
echo ""

# [1/4] Stop systemd service
echo "[1/4] Stopping service..."
if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    echo "  Stopped systemd service."
elif [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
        kill "$PID" 2>/dev/null || true
        echo "  Stopped legacy process (PID: $PID)"
    fi
    rm -f "$PID_FILE"
else
    echo "  Not running."
fi

# [2/4] Remove systemd service
echo "[2/4] Removing systemd service..."
if [ -f "$SERVICE_FILE" ]; then
    systemctl disable "$SERVICE_NAME" --quiet 2>/dev/null || true
    rm -f "$SERVICE_FILE"
    systemctl daemon-reload
    echo "  Service removed."
else
    echo "  No service file found."
fi

# [3/4] Remove backup cron
echo "[3/4] Removing backup cron..."
if crontab -l 2>/dev/null | grep -q "veilkey-backup"; then
    crontab -l 2>/dev/null | grep -v "veilkey-backup" | crontab -
    echo "  Backup cron removed."
else
    echo "  No backup cron found."
fi
rm -f /usr/local/bin/veilkey-backup.sh

# [4/4] Remove data
echo "[4/4] Install directory: $INSTALL_DIR"
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

# Remove CLI binaries
echo ""
if [ -f /usr/local/bin/veilkey-cli ]; then
    read -p "  Remove /usr/local/bin/veilkey CLI? (y/N) " confirm_cli
    if [[ "$confirm_cli" =~ ^[yY]$ ]]; then
        rm -f /usr/local/bin/veilkey /usr/local/bin/veilkey-cli
        echo "  CLI removed."
    else
        echo "  CLI kept."
    fi
fi

echo ""
echo "=== Uninstall complete ==="
echo ""
