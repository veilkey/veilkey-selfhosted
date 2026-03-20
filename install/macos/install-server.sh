#!/bin/bash
set -euo pipefail

# VeilKey server installer for macOS
# Starts VaultCenter + LocalVault via Docker Compose.
#
# Usage:
#   bash install/macos/install-server.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

# Must run from repo root
if [ ! -f "docker-compose.yml" ] || [ ! -d "services" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

REPO_ROOT="$(pwd)"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:11181}"

echo "=== VeilKey server installer (macOS) ==="
echo ""

# [1/2] Check prerequisites
if ! command -v docker &>/dev/null; then
    echo "ERROR: docker not found."
    echo "  Install: https://docs.docker.com/desktop/install/mac-install/"
    exit 1
fi
echo "[1/2] Prerequisites OK"

# [2/2] Start Docker Compose
echo "[2/2] Starting services..."
PORT="${VEILKEY_URL##*:}"
PORT="${PORT%%/*}"

OWN_DOCKER=false
if docker compose ps --quiet 2>/dev/null | grep -q .; then
    OWN_DOCKER=true
fi

if [ "$OWN_DOCKER" = false ] && lsof -i ":$PORT" -sTCP:LISTEN >/dev/null 2>&1; then
    echo ""
    echo "⚠️  포트 $PORT 가 이미 사용 중입니다."
    echo "   기존 인스턴스: cd <경로> && docker compose down"
    echo "   다른 포트:     VEILKEY_URL=https://localhost:$((PORT+1)) bash install/macos/install-server.sh"
    echo "   Docker 건너뜁니다."
else
    [ ! -f "$REPO_ROOT/.env" ] && cp "$REPO_ROOT/.env.example" "$REPO_ROOT/.env" 2>/dev/null || true
    docker compose up --build -d 2>&1 | tail -5
fi

echo ""
echo "=== Server installation complete ==="
echo ""
echo "초기 설정:"
echo "  https://localhost:${PORT} 접속 → 마스터/관리자 비밀번호 설정"
echo ""
echo "서버 재시작 후:"
echo "  마스터 비밀번호 입력 필요 (비밀번호는 메모리에만 존재)"
echo ""
