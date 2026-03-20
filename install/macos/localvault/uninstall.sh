#!/bin/bash
set -euo pipefail

# LocalVault uninstaller for macOS
#
# Usage:
#   bash install/macos/localvault/uninstall.sh

if [ ! -f "docker-compose.yml" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

echo "=== LocalVault uninstaller ==="
echo ""
echo "Stopping LocalVault..."
docker compose stop localvault 2>/dev/null || true
docker compose rm -f localvault 2>/dev/null || true
echo "  Stopped."
echo ""
echo "데이터 삭제:"
echo "  rm -rf data/localvault"
echo "  (자동 삭제하지 않습니다 — vault 데이터가 포함될 수 있음)"
echo ""
