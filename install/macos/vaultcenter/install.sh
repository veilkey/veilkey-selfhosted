#!/bin/bash
set -euo pipefail

# VaultCenter installer for macOS
# Builds and starts VaultCenter via Docker Compose.
#
# Usage:
#   bash install/macos/vaultcenter/install.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

if [ ! -f "docker-compose.yml" ] || [ ! -d "services" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

REPO_ROOT="$(pwd)"
VC_PORT="${VAULTCENTER_HOST_PORT:-11181}"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:${VC_PORT}}"
PORT="${VEILKEY_URL##*:}"
PORT="${PORT%%/*}"

echo "=== VaultCenter installer (macOS) ==="
echo ""

if ! command -v docker &>/dev/null; then
    echo "ERROR: docker not found."
    echo "  Install: https://docs.docker.com/desktop/install/mac-install/"
    exit 1
fi
echo "[1/2] Prerequisites OK"

echo "[2/2] Starting VaultCenter..."
[ ! -f "$REPO_ROOT/.env" ] && cp "$REPO_ROOT/.env.example" "$REPO_ROOT/.env" 2>/dev/null || true
docker compose up --build -d vaultcenter 2>&1 | tail -5

echo ""
echo "=== VaultCenter installation complete ==="
echo "  URL: https://localhost:${PORT}"
echo ""
