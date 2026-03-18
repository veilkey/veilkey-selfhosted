#!/usr/bin/env bash
# VeilKey VaultCenter — Shell 올인원 설치 스크립트
# Usage:
#   bash install-shell.sh                          # 대화형
#   VEILKEY_PASSWORD_FILE=/path/to/pw NONINTERACTIVE=1 bash install-shell.sh  # 자동
set -euo pipefail

BINARY_NAME="veilkey-vaultcenter"
INSTALL_BIN="/usr/local/bin/$BINARY_NAME"
DATA_DIR="${VEILKEY_DATA_DIR:-/opt/veilkey-vaultcenter/data}"
SERVICE_NAME="veilkey-vaultcenter"
LISTEN_ADDR=":10180"
REGISTRY="${VEILKEY_REGISTRY:-ghcr.io}"
IMAGE="$REGISTRY/veilkey/veilkey-vaultcenter:latest"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

# --- Root check ---
[[ $EUID -eq 0 ]] || error "root 권한이 필요합니다. sudo bash install-shell.sh"

info "VeilKey VaultCenter — Shell 설치"
echo ""

# --- Get binary ---
get_binary() {
    # Method 1: Extract from Docker image (if docker available)
    if command -v docker >/dev/null 2>&1; then
        info "Docker 이미지에서 바이너리 추출..."
        if docker pull "$IMAGE" >/dev/null 2>&1 || docker image inspect "$IMAGE" >/dev/null 2>&1; then
            local cid
            cid=$(docker create "$IMAGE" 2>/dev/null) || true
            if [[ -n "$cid" ]]; then
                docker cp "$cid:/usr/local/bin/$BINARY_NAME" "$INSTALL_BIN" 2>/dev/null && {
                    docker rm "$cid" >/dev/null 2>&1
                    chmod +x "$INSTALL_BIN"
                    info "바이너리 추출 완료: $INSTALL_BIN"
                    return 0
                }
                docker rm "$cid" >/dev/null 2>&1
            fi
        fi
        warn "Docker 이미지에서 추출 실패, 소스 빌드로 전환..."
    fi

    # Method 2: Build from source
    local SOURCE_DIR="/opt/veilkey/storage"
    if [[ -d "$SOURCE_DIR" ]] && [[ -f "$SOURCE_DIR/go.mod" ]]; then
        info "소스에서 빌드..."
        command -v go >/dev/null 2>&1 || error "Go가 설치되어 있지 않습니다."
        # CGO required for sqlite3
        if ! command -v gcc >/dev/null 2>&1; then
            if command -v apt-get >/dev/null 2>&1; then
                apt-get update -qq && apt-get install -y -qq gcc musl-dev >/dev/null 2>&1
            elif command -v apk >/dev/null 2>&1; then
                apk add --no-cache gcc musl-dev >/dev/null 2>&1
            else
                error "gcc가 필요합니다. 먼저 설치하세요."
            fi
        fi
        cd "$SOURCE_DIR"
        CGO_ENABLED=1 go build -ldflags="-s -w" -o "$INSTALL_BIN" ./cmd/main.go
        chmod +x "$INSTALL_BIN"
        info "빌드 완료: $INSTALL_BIN"
        return 0
    fi

    # Method 3: Copy from local build
    if [[ -f "/opt/veilkey/storage/veilkey-vaultcenter" ]]; then
        cp /opt/veilkey/storage/veilkey-vaultcenter "$INSTALL_BIN"
        chmod +x "$INSTALL_BIN"
        info "로컬 바이너리 복사 완료"
        return 0
    fi
    error "바이너리를 획득할 수 없습니다. Docker 또는 Go + 소스코드가 필요합니다."
}

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
    echo "  설치 모드를 선택하세요:"
    echo "    1) root  — 중앙 Hub 서버 (기본)"
    echo "    2) child — Parent에 연결되는 자식 노드"
    read -rp "  선택 [1]: " mode_choice
    case "${mode_choice:-1}" in
        2) VEILKEY_MODE="child" ;;
        *) VEILKEY_MODE="root" ;;
    esac

    while true; do
        read -rsp "  마스터 패스워드 (8자 이상): " VEILKEY_PASSWORD
        echo ""
        [[ ${#VEILKEY_PASSWORD} -ge 8 ]] && break
        warn "패스워드는 8자 이상이어야 합니다."
    done
    read -rsp "  패스워드 확인: " pw_confirm
    echo ""
    [[ "$VEILKEY_PASSWORD" == "$pw_confirm" ]] || error "패스워드가 일치하지 않습니다."

    read -rp "  포트 [10180]: " VEILKEY_PORT
    VEILKEY_PORT="${VEILKEY_PORT:-10180}"

    read -rp "  Trusted IPs [127.0.0.1]: " VEILKEY_TRUSTED_IPS
    VEILKEY_TRUSTED_IPS="${VEILKEY_TRUSTED_IPS:-127.0.0.1}"

    VEILKEY_PARENT_URL=""
    VEILKEY_LABEL="$(hostname)"
    if [[ "$VEILKEY_MODE" == "child" ]]; then
        read -rp "  Parent URL (예: http://YOUR_HUB_IP:10180): " VEILKEY_PARENT_URL
        [[ -z "$VEILKEY_PARENT_URL" ]] && error "Child 모드에서는 Parent URL이 필수입니다."
        read -rp "  노드 라벨 [$(hostname)]: " label_input
        VEILKEY_LABEL="${label_input:-$(hostname)}"
    fi
fi

LISTEN_ADDR=":${VEILKEY_PORT}"

# --- Install binary ---
get_binary

# --- Data directory ---
mkdir -p "$DATA_DIR"
info "데이터 디렉토리: $DATA_DIR"

# --- Initialize ---
export VEILKEY_DB_PATH="$DATA_DIR/veilkey.db"

if [[ ! -f "$DATA_DIR/salt" ]]; then
    info "초기화 ($VEILKEY_MODE 모드)..."
    if [[ "$VEILKEY_MODE" == "child" ]]; then
        echo "$VEILKEY_PASSWORD" | "$INSTALL_BIN" init --child \
            --parent "$VEILKEY_PARENT_URL" \
            --label "$VEILKEY_LABEL" \
            --addr "$LISTEN_ADDR"
    else
        echo "$VEILKEY_PASSWORD" | "$INSTALL_BIN" init --root
    fi
else
    info "이미 초기화됨 (salt 파일 존재)"
fi

# --- Save password to restricted file for auto-unlock ---
printf '%s' "$VEILKEY_PASSWORD" > "$DATA_DIR/password"
chmod 600 "$DATA_DIR/password"
cat > "$DATA_DIR/veilkey-env" << ENVFILE
VEILKEY_PASSWORD_FILE=$DATA_DIR/password
ENVFILE
chmod 600 "$DATA_DIR/veilkey-env"

# --- Systemd service ---
info "systemd 서비스 등록..."
cat > "/etc/systemd/system/${SERVICE_NAME}.service" << UNITFILE
[Unit]
Description=VeilKey VaultCenter
After=network.target

[Service]
Type=simple
ExecStart=$INSTALL_BIN
Environment=VEILKEY_ADDR=$LISTEN_ADDR
Environment=VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
Environment=VEILKEY_TRUSTED_IPS=$VEILKEY_TRUSTED_IPS
EnvironmentFile=-$DATA_DIR/veilkey-env
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNITFILE

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
systemctl restart "$SERVICE_NAME"
info "서비스 시작됨: $SERVICE_NAME"

# --- Health check ---
info "서버 준비 대기 (최대 15초)..."
for i in $(seq 1 15); do
    if curl -sfk "https://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1 || curl -sf "http://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1; then
        break
    fi
    sleep 1
done

# --- Verify ---
echo ""
if curl -sfk "https://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1 || curl -sf "http://127.0.0.1:${VEILKEY_PORT}/health" >/dev/null 2>&1; then
    info "VeilKey VaultCenter 설치 완료!"
    echo ""
    echo "  ╔══════════════════════════════════════════════════════╗"
    echo "  ║           VeilKey VaultCenter — Shell Install               ║"
    echo "  ╚══════════════════════════════════════════════════════╝"
    echo ""
    echo "  Mode     : $VEILKEY_MODE"
    echo "  Binary   : $INSTALL_BIN"
    echo "  Data     : $DATA_DIR"
    echo "  Port     : $VEILKEY_PORT"
    echo "  Service  : $SERVICE_NAME"
    echo ""
    echo "  관리 명령어:"
    echo "    systemctl status $SERVICE_NAME"
    echo "    journalctl -u $SERVICE_NAME -f"
    echo "    curl -k https://127.0.0.1:${VEILKEY_PORT}/api/status"
    echo ""
else
    error "서버가 응답하지 않습니다. journalctl -u $SERVICE_NAME -n 20 으로 로그를 확인하세요."
fi
