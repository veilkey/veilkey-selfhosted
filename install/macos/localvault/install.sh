#!/bin/bash
set -euo pipefail

# LocalVault installer for macOS
# Builds and starts LocalVault via Docker Compose.
# Requires VaultCenter to be running.
#
# Usage:
#   bash install/macos/localvault/install.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

if [ ! -f "docker-compose.yml" ] || [ ! -d "services" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

REPO_ROOT="$(pwd)"

echo "=== LocalVault installer (macOS) ==="
echo ""

if ! command -v docker &>/dev/null; then
    echo "ERROR: docker not found."
    echo "  Install: https://docs.docker.com/desktop/install/mac-install/"
    exit 1
fi

# Check VaultCenter is running
if ! docker compose ps vaultcenter 2>/dev/null | grep -q "running"; then
    echo "⚠️  VaultCenter가 실행 중이지 않습니다."
    echo "  먼저 실행: bash install/macos/vaultcenter/install.sh"
    exit 1
fi
echo "[1/2] Prerequisites OK (VaultCenter running)"

echo "[2/2] Starting LocalVault..."
docker compose up --build -d localvault 2>&1 | tail -5

echo ""
echo "=== LocalVault installation complete ==="
LV_PORT="${LOCALVAULT_HOST_PORT:-11180}"
echo "  URL: https://localhost:${LV_PORT}"
echo ""
echo "다음 단계:"
echo "  VaultCenter 키센터에서 등록 토큰 발급 → LocalVault init"
echo "  See docs/setup.md"
echo ""
