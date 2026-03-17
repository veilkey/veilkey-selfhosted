#!/usr/bin/env bash
# VeilKey KeyCenter — Docker 통합 설치 스크립트
# Usage:
#   bash install-docker.sh                          # 대화형
#   VEILKEY_PASSWORD_FILE=/path/to/pw NONINTERACTIVE=1 bash install-docker.sh  # 자동
set -euo pipefail

INSTALL_DIR="${VEILKEY_INSTALL_DIR:-/opt/veilkey-keycenter}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE_FILE="docker-compose.hub.yml"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

# --- Prerequisites ---
command -v docker >/dev/null 2>&1 || error "Docker not found. Install Docker first."
docker compose version >/dev/null 2>&1 || error "Docker Compose not found."

info "VeilKey KeyCenter — Docker 설치"
echo ""

# --- Collect Config ---
if [[ "${NONINTERACTIVE:-}" == "1" ]]; then
    if [[ -n "${VEILKEY_PASSWORD_FILE:-}" && -f "${VEILKEY_PASSWORD_FILE}" ]]; then
        VEILKEY_PASSWORD="$(cat "${VEILKEY_PASSWORD_FILE}")"
    fi
    [[ -z "${VEILKEY_PASSWORD:-}" ]] && error "NONINTERACTIVE mode: VEILKEY_PASSWORD_FILE required"
    VEILKEY_MODE="${VEILKEY_MODE:-root}"
    VEILKEY_PORT="${VEILKEY_PORT:-10180}"
    VEILKEY_TRUSTED_IPS="${VEILKEY_TRUSTED_IPS:-127.0.0.1}"
    VEILKEY_PARENT_URL="${VEILKEY_PARENT_URL:-}"
    VEILKEY_LABEL="${VEILKEY_LABEL:-$(hostname)}"
else
    # Mode
    echo "  설치 모드를 선택하세요:"
    echo "    1) root  — 중앙 Hub 서버 (기본)"
    echo "    2) child — Parent에 연결되는 자식 노드"
    read -rp "  선택 [1]: " mode_choice
    case "${mode_choice:-1}" in
        2) VEILKEY_MODE="child" ;;
        *) VEILKEY_MODE="root" ;;
    esac

    # Password
    while true; do
        read -rsp "  마스터 패스워드 (8자 이상): " VEILKEY_PASSWORD
        echo ""
        [[ ${#VEILKEY_PASSWORD} -ge 8 ]] && break
        warn "패스워드는 8자 이상이어야 합니다."
    done
    read -rsp "  패스워드 확인: " pw_confirm
    echo ""
    [[ "$VEILKEY_PASSWORD" == "$pw_confirm" ]] || error "패스워드가 일치하지 않습니다."

    # Port
    read -rp "  포트 [10180]: " VEILKEY_PORT
    VEILKEY_PORT="${VEILKEY_PORT:-10180}"

    # Trusted IPs
    read -rp "  Trusted IPs [127.0.0.1]: " VEILKEY_TRUSTED_IPS
    VEILKEY_TRUSTED_IPS="${VEILKEY_TRUSTED_IPS:-127.0.0.1}"

    # Child mode extras
    VEILKEY_PARENT_URL=""
    VEILKEY_LABEL="$(hostname)"
    if [[ "$VEILKEY_MODE" == "child" ]]; then
        read -rp "  Parent URL (예: http://YOUR_HUB_IP:10180): " VEILKEY_PARENT_URL
        [[ -z "$VEILKEY_PARENT_URL" ]] && error "Child 모드에서는 Parent URL이 필수입니다."
        read -rp "  노드 라벨 [$(hostname)]: " label_input
        VEILKEY_LABEL="${label_input:-$(hostname)}"
    fi
fi

# --- Install ---
info "설치 디렉토리: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"

# Copy compose file
if [[ -f "$SCRIPT_DIR/$COMPOSE_FILE" ]]; then
    cp "$SCRIPT_DIR/$COMPOSE_FILE" "$INSTALL_DIR/docker-compose.yml"
else
    # Inline compose if script is run standalone
    cat > "$INSTALL_DIR/docker-compose.yml" << 'COMPOSEYML'
services:
  veilkey-keycenter:
    image: ${VEILKEY_REGISTRY:-ghcr.io}/veilkey/veilkey-keycenter:latest
    container_name: veilkey-keycenter
    ports:
      - "${VEILKEY_PORT:-10180}:10180"
    volumes:
      - veilkey-data:/data
      - ${VEILKEY_PASSWORD_DIR:-./secrets}/password:/run/secrets/veilkey_password:ro
    environment:
      VEILKEY_PASSWORD_FILE: /run/secrets/veilkey_password
      VEILKEY_MODE: ${VEILKEY_MODE:-root}
      VEILKEY_ADDR: ":10180"
      VEILKEY_DB_PATH: /data/veilkey.db
      VEILKEY_TRUSTED_IPS: ${VEILKEY_TRUSTED_IPS:-127.0.0.1}
      VEILKEY_PARENT_URL: ${VEILKEY_PARENT_URL:-}
      VEILKEY_LABEL: ${VEILKEY_LABEL:-}
    restart: unless-stopped

volumes:
  veilkey-data:
COMPOSEYML
fi

# Write password file (never store in .env)
mkdir -p "$INSTALL_DIR/secrets"
printf '%s' "$VEILKEY_PASSWORD" > "$INSTALL_DIR/secrets/password"
chmod 600 "$INSTALL_DIR/secrets/password"

# Write .env (no password here)
cat > "$INSTALL_DIR/.env" << ENVFILE
VEILKEY_PASSWORD_DIR=$INSTALL_DIR/secrets
VEILKEY_MODE=$VEILKEY_MODE
VEILKEY_PORT=$VEILKEY_PORT
VEILKEY_TRUSTED_IPS=$VEILKEY_TRUSTED_IPS
VEILKEY_PARENT_URL=$VEILKEY_PARENT_URL
VEILKEY_LABEL=$VEILKEY_LABEL
VEILKEY_REGISTRY=${VEILKEY_REGISTRY:-ghcr.io}
ENVFILE
chmod 600 "$INSTALL_DIR/.env"
info ".env 생성 완료 (패스워드는 secrets/password에 별도 저장)"

# Start
info "컨테이너 시작..."
cd "$INSTALL_DIR"
docker compose up -d 2>&1

# Health check
info "서버 준비 대기 (최대 30초)..."
for i in $(seq 1 30); do
    if curl -sfk "https://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1 || curl -sf "http://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

# Verify
echo ""
if curl -sfk "https://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1 || curl -sf "http://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1; then
    NODE_INFO=$(curl -sfk "https://127.0.0.1:${VEILKEY_PORT}/api/status" || curl -sf "http://127.0.0.1:${VEILKEY_PORT}/api/status")
    info "VeilKey KeyCenter 설치 완료!"
    echo ""
    echo "  ╔══════════════════════════════════════════════════════╗"
    echo "  ║           VeilKey KeyCenter — Docker Install              ║"
    echo "  ╚══════════════════════════════════════════════════════╝"
    echo ""
    echo "  Mode     : $VEILKEY_MODE"
    echo "  Port     : $VEILKEY_PORT"
    echo "  Dir      : $INSTALL_DIR"
    echo "  Status   : $(echo "$NODE_INFO" | grep -o '"locked":[a-z]*' | cut -d: -f2)"
    echo ""
    echo "  관리 명령어:"
    echo "    docker compose -f $INSTALL_DIR/docker-compose.yml logs -f"
    echo "    docker compose -f $INSTALL_DIR/docker-compose.yml restart"
    echo "    curl -k https://127.0.0.1:${VEILKEY_PORT}/api/status"
    echo ""
else
    error "서버가 응답하지 않습니다. 로그 확인: docker compose -f $INSTALL_DIR/docker-compose.yml logs"
fi
