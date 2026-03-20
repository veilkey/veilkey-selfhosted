#!/bin/bash
set -euo pipefail

# VeilKey LocalVault one-liner installer
# Usage: curl -sL .../install-localvault.sh | bash
#
# 현재 디렉토리에 localvault를 설치합니다.
# 이미 설치되어 있으면 업데이트 후 재시작합니다.
#
# Options (env vars):
#   VEILKEY_CENTER_URL=http://10.87.40.2:10181   vaultcenter 주소
#   VEILKEY_PORT=10180                            localvault 포트
#   VEILKEY_NAME=my-vault                         볼트 이름 (기본: hostname)
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

INSTALL_DIR="$(pwd)/.localvault"
DATA_DIR="$INSTALL_DIR/data"
PID_FILE="$INSTALL_DIR/localvault.pid"
LOG_FILE="$INSTALL_DIR/localvault.log"
BIN="$INSTALL_DIR/veilkey-localvault"
ENV_FILE="$INSTALL_DIR/.env"

PORT="${VEILKEY_PORT:-10180}"
CENTER_URL="${VEILKEY_CENTER_URL:-}"
VAULT_NAME="${VEILKEY_NAME:-$(hostname)}"

REPO_URL="https://github.com/veilkey/veilkey-selfhosted.git"
REPO_DIR="$INSTALL_DIR/src"

echo "=== VeilKey LocalVault Installer ==="
echo "설치 경로: $INSTALL_DIR"
echo "포트:      $PORT"
echo "볼트 이름: $VAULT_NAME"
[[ -n "$CENTER_URL" ]] && echo "센터 URL:  $CENTER_URL"
echo ""

# 0. Check Go
if ! command -v go &>/dev/null; then
    echo "ERROR: Go가 설치되어 있지 않습니다."
    echo "  brew install go  또는  https://go.dev/dl/"
    exit 1
fi
GO_VER=$(go version | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
echo "[0/5] Go $GO_VER OK"

# 1. Clone or update source
mkdir -p "$INSTALL_DIR"
if [ -d "$REPO_DIR/.git" ]; then
    echo "[1/5] 소스 업데이트..."
    cd "$REPO_DIR"
    git pull --quiet
else
    echo "[1/5] 소스 다운로드..."
    git clone --quiet --depth 1 "$REPO_URL" "$REPO_DIR"
fi

# 2. Build
echo "[2/5] 빌드 중..."
cd "$REPO_DIR/services/localvault"
CGO_ENABLED=1 go build -o "$BIN" ./cmd
echo "  빌드 완료: $BIN"

# 3. Setup data directory and env
echo "[3/5] 데이터 디렉토리 설정..."
mkdir -p "$DATA_DIR"

if [ -f "$ENV_FILE" ]; then
    echo "  기존 설정 유지: $ENV_FILE"
else
cat > "$ENV_FILE" << ENVEOF
VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
VEILKEY_ADDR=:$PORT
VEILKEY_TRUSTED_IPS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1
VEILKEY_MODE=root
VEILKEY_VAULT_NAME=$VAULT_NAME
ENVEOF

if [[ -n "$CENTER_URL" ]]; then
    echo "VEILKEY_VAULTCENTER_URL=$CENTER_URL" >> "$ENV_FILE"
fi
echo "  환경 설정: $ENV_FILE"
echo "  (LOCKED 모드로 시작 — POST /api/unlock 으로 수동 unlock)"
fi

# 4. Stop existing process if running
echo "[4/5] 기존 프로세스 확인..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
        echo "  기존 프로세스 종료 (PID: $OLD_PID)"
        kill "$OLD_PID" 2>/dev/null || true
        sleep 2
    fi
    rm -f "$PID_FILE"
fi

# 5. Start
echo "[5/5] LocalVault 시작..."
cd "$INSTALL_DIR"
set -a; source "$ENV_FILE"; set +a
nohup "$BIN" server > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
NEW_PID=$(cat "$PID_FILE")

# Wait for startup
sleep 2
if kill -0 "$NEW_PID" 2>/dev/null; then
    # Health check
    if curl -s "http://localhost:$PORT/health" | grep -q '"ok"'; then
        echo ""
        echo "=== 설치 완료 ==="
        echo ""
        echo "  상태:    http://localhost:$PORT/health"
        echo "  PID:     $NEW_PID"
        echo "  로그:    $LOG_FILE"
        echo "  데이터:  $DATA_DIR"
        [[ -n "$CENTER_URL" ]] && echo "  센터:    $CENTER_URL (heartbeat 자동 연동)"
        echo ""
        echo "관리 명령:"
        echo "  tail -f $LOG_FILE          # 로그 보기"
        echo "  kill \$(cat $PID_FILE)      # 중지"
        echo "  curl -sL ... | bash         # 업데이트 (같은 명령 재실행)"
    else
        echo ""
        echo "⚠️  프로세스는 실행 중이지만 health check 실패"
        echo "  로그 확인: tail -20 $LOG_FILE"
    fi
else
    echo ""
    echo "❌ 시작 실패"
    echo "  로그 확인: tail -20 $LOG_FILE"
    tail -20 "$LOG_FILE"
    exit 1
fi
