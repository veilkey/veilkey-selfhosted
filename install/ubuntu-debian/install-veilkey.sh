#!/bin/bash
set -euo pipefail

# VeilKey installer for Ubuntu / Debian
# Installs Docker, clones the repo, and starts all services via docker compose.
#
# Usage:
#   bash install/ubuntu-debian/install-veilkey.sh
#
# Options (env vars):
#   INSTALL_DIR=/opt/veilkey        Installation directory (default: /opt/veilkey)
#   VAULTCENTER_HOST_PORT=11181     VaultCenter port (default: 11181)
#   LOCALVAULT_HOST_PORT=11180      LocalVault port  (default: 11180)
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

INSTALL_DIR="${INSTALL_DIR:-/opt/veilkey}"
VC_PORT="${VAULTCENTER_HOST_PORT:-11181}"
LV_PORT="${LOCALVAULT_HOST_PORT:-11180}"
REPO_URL="https://github.com/veilkey/veilkey-selfhosted.git"

# --- Validation ---
if [[ "$EUID" -ne 0 ]]; then
    echo "ERROR: root 권한으로 실행해야 합니다."
    echo "  sudo bash install/ubuntu-debian/install-veilkey.sh"
    exit 1
fi

. /etc/os-release 2>/dev/null || true
case "${ID:-}" in
    ubuntu|debian) ;;
    *)
        echo "WARNING: Ubuntu / Debian 이 아닌 환경에서 실행 중입니다 (ID=${ID:-unknown})."
        echo "  계속하려면 Enter, 중단하려면 Ctrl+C"
        read -r
        ;;
esac

echo "=== VeilKey Installer (Ubuntu / Debian) ==="
echo ""
echo "  설치 경로:       $INSTALL_DIR"
echo "  VaultCenter 포트: $VC_PORT"
echo "  LocalVault 포트:  $LV_PORT"
echo ""

# --- [1/5] Install dependencies ---
echo "[1/5] 패키지 설치 중..."
apt-get update -qq
apt-get install -y -qq git curl ca-certificates

# Docker (공식 저장소)
if ! command -v docker &>/dev/null; then
    echo "  Docker 설치 중..."
    install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/${ID}/gpg \
        -o /etc/apt/keyrings/docker.asc
    chmod a+r /etc/apt/keyrings/docker.asc
    echo \
        "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
https://download.docker.com/linux/${ID} \
$(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
        > /etc/apt/sources.list.d/docker.list
    apt-get update -qq
    apt-get install -y -qq docker-ce docker-ce-cli containerd.io \
        docker-buildx-plugin docker-compose-plugin
    systemctl enable --now docker
    echo "  Docker $(docker --version) 설치 완료."
else
    echo "  Docker 이미 설치됨: $(docker --version)"

    # docker compose plugin 확인
    if ! docker compose version &>/dev/null; then
        echo "  Docker Compose 플러그인 설치 중..."
        mkdir -p /usr/lib/docker/cli-plugins
        curl -sL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
            -o /usr/lib/docker/cli-plugins/docker-compose
        chmod +x /usr/lib/docker/cli-plugins/docker-compose
    fi
    echo "  Docker Compose $(docker compose version --short) OK."
fi

# --- [2/5] Clone repository ---
echo "[2/5] 저장소 클론 중..."
if [[ -d "$INSTALL_DIR/.git" ]]; then
    echo "  이미 설치됨 — 업데이트 중..."
    git -C "$INSTALL_DIR" pull --quiet
else
    git clone --quiet "$REPO_URL" "$INSTALL_DIR"
fi
echo "  경로: $INSTALL_DIR"

# --- [3/5] Configure environment ---
echo "[3/5] 환경 설정..."
cd "$INSTALL_DIR"
if [[ ! -f ".env" ]]; then
    cp .env.example .env
    # 포트 덮어쓰기 (env var 로 오버라이드한 경우)
    sed -i "s/^VAULTCENTER_HOST_PORT=.*/VAULTCENTER_HOST_PORT=$VC_PORT/" .env
    sed -i "s/^LOCALVAULT_HOST_PORT=.*/LOCALVAULT_HOST_PORT=$LV_PORT/" .env
    echo "  .env 생성 완료 (필요 시 $INSTALL_DIR/.env 를 수정하세요)."
else
    echo "  기존 .env 유지."
fi

# --- [4/5] Start services ---
echo "[4/5] 서비스 시작 중 (첫 빌드는 수 분이 걸릴 수 있습니다)..."
docker compose up -d 2>&1 | tail -5

# --- [5/5] Health check ---
echo "[5/5] VaultCenter health check 대기 중..."
HEALTH=""
for i in $(seq 1 30); do
    HEALTH=$(curl -sk "https://localhost:${VC_PORT}/health" 2>/dev/null || true)
    if echo "$HEALTH" | grep -q '"status"'; then
        break
    fi
    sleep 5
done

if echo "$HEALTH" | grep -q '"status"'; then
    echo ""
    echo "=== 설치 완료 ==="
    echo ""
    echo "  VaultCenter: https://$(hostname -I | awk '{print $1}'):${VC_PORT}"
    echo "  LocalVault:  https://$(hostname -I | awk '{print $1}'):${LV_PORT}"
    echo "  상태:        $HEALTH"
    echo ""
    echo "다음 단계:"
    echo "  1. 초기 설정 (최초 1회):"
    echo "     curl -sk -X POST https://localhost:${VC_PORT}/api/setup/init \\"
    echo "       -H 'Content-Type: application/json' \\"
    echo "       -d '{\"password\":\"<MASTER_PASSWORD>\",\"admin_password\":\"<ADMIN_PASSWORD>\"}'"
    echo ""
    echo "  2. 이미 초기화된 경우 unlock:"
    echo "     curl -sk -X POST https://localhost:${VC_PORT}/api/unlock \\"
    echo "       -H 'Content-Type: application/json' \\"
    echo "       -d '{\"password\":\"<MASTER_PASSWORD>\"}'"
    echo ""
    echo "  3. LocalVault 등록: docs/setup.md 참고"
    echo ""
    echo "관리 명령:"
    echo "  cd $INSTALL_DIR"
    echo "  docker compose ps          # 상태 확인"
    echo "  docker compose logs -f     # 로그 보기"
    echo "  docker compose down        # 중지"
    echo ""
else
    echo ""
    echo "⚠️  Health check 가 시간 내에 응답하지 않았습니다."
    echo "  서비스가 아직 빌드 중일 수 있습니다. 아래 명령으로 확인하세요:"
    echo "    cd $INSTALL_DIR && docker compose ps"
    echo "    cd $INSTALL_DIR && docker compose logs"
fi
