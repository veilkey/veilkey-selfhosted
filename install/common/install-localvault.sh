#!/bin/bash
set -euo pipefail

# VeilKey LocalVault installer
# All configuration via env vars — no hardcoded values.
#
# Required:
#   VEILKEY_CENTER_URL       VaultCenter 주소
#   VEILKEY_REG_TOKEN        Registration token
#
# Optional:
#   VEILKEY_PORT             포트 (default: 10180)
#   VEILKEY_LABEL            볼트 이름 (default: hostname)
#   VEILKEY_DATA_DIR         데이터 경로 (default: /data/localvault)
#   VEILKEY_BIN_DIR          바이너리 경로 (default: /usr/local/bin)
#   VEILKEY_TLS_INSECURE     TLS 검증 스킵 (default: 1)
#   VEILKEY_TRUSTED_IPS      허용 IP (default: private ranges)
#   VEILKEY_SYSTEMD          systemd 서비스 생성 (default: 1)
#   VEILKEY_BINARY_URL       바이너리 다운로드 URL

CENTER_URL="${VEILKEY_CENTER_URL:-}"
REG_TOKEN="${VEILKEY_REG_TOKEN:-}"

if [[ -z "$CENTER_URL" ]]; then
    echo "ERROR: VEILKEY_CENTER_URL is required"
    echo "  export VEILKEY_CENTER_URL=https://<host>:<port>"
    exit 1
fi
if [[ -z "$REG_TOKEN" ]]; then
    echo "ERROR: VEILKEY_REG_TOKEN is required"
    exit 1
fi

PORT="${VEILKEY_PORT:-10180}"
LABEL="${VEILKEY_LABEL:-$(hostname)}"
DATA_DIR="${VEILKEY_DATA_DIR:-/data/localvault}"
BIN_DIR="${VEILKEY_BIN_DIR:-/usr/local/bin}"
TLS_INSECURE="${VEILKEY_TLS_INSECURE:-1}"
TRUSTED_IPS="${VEILKEY_TRUSTED_IPS:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1}"
USE_SYSTEMD="${VEILKEY_SYSTEMD:-1}"
BINARY_URL="${VEILKEY_BINARY_URL:-}"
BIN="$BIN_DIR/veilkey-localvault"
ENV_FILE="$DATA_DIR/.env"
CERT_DIR="$DATA_DIR/certs"
TLS_CERT="$CERT_DIR/cert.pem"
TLS_KEY="$CERT_DIR/key.pem"
HEALTH_URL="https://127.0.0.1:$PORT/health"

echo "=== VeilKey LocalVault Installer ==="
echo "  센터: $CENTER_URL"
echo "  라벨: $LABEL | 포트: $PORT | 데이터: $DATA_DIR"

# [1] Binary
if [[ -n "$BINARY_URL" ]]; then
    echo "[1/5] 바이너리 다운로드..."
    mkdir -p "$BIN_DIR"
    curl -sL "$BINARY_URL" -o "$BIN" && chmod +x "$BIN"
elif command -v go &>/dev/null; then
    echo "[1/5] 소스 빌드..."
    REPO_DIR="${VEILKEY_REPO_DIR:-$(mktemp -d)}"
    [ -d "$REPO_DIR/.git" ] || git clone --quiet --depth 1 https://github.com/veilkey/veilkey-selfhosted.git "$REPO_DIR"
    cd "$REPO_DIR/services/localvault"
    GOTOOLCHAIN=auto CGO_ENABLED=1 go build -o "$BIN" ./cmd
elif [ -f "$BIN" ]; then
    echo "[1/5] 기존 바이너리 사용"
else
    echo "ERROR: Go 없음, 바이너리 없음. VEILKEY_BINARY_URL을 설정하세요."
    exit 1
fi

# [2] Data dir + TLS
echo "[2/5] 데이터 디렉토리 + TLS..."
mkdir -p "$CERT_DIR"
if [[ -f "$TLS_CERT" && -f "$TLS_KEY" ]]; then
    echo "  기존 TLS 인증서 유지"
else
    LOCAL_IPS=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v '^$' | head -5 || true)
    SAN="DNS:localhost,DNS:$(hostname),IP:127.0.0.1"
    for ip in $LOCAL_IPS; do
        SAN="$SAN,IP:$ip"
    done
    openssl req -x509 -newkey rsa:2048 \
        -keyout "$TLS_KEY" -out "$TLS_CERT" \
        -days 3650 -nodes \
        -subj "/CN=$(hostname)" \
        -addext "subjectAltName=$SAN" \
        -addext "basicConstraints=critical,CA:FALSE" \
        -addext "keyUsage=digitalSignature,keyEncipherment" \
        -addext "extendedKeyUsage=serverAuth" >/dev/null 2>&1
fi

# [3] Init
if [ -f "$DATA_DIR/salt" ]; then
    echo "[3/5] 이미 초기화됨 — 스킵"
else
    echo "[3/5] 초기화..."
    VEILKEY_TLS_INSECURE="$TLS_INSECURE" \
    VEILKEY_DB_PATH="$DATA_DIR/veilkey.db" \
    VEILKEY_VAULT_NAME="$LABEL" \
    VEILKEY_LABEL="$LABEL" \
    "$BIN" init --root --center "$CENTER_URL" --token "$REG_TOKEN"
fi

# [4] Env file
echo "[4/5] 환경 설정..."
cat > "$ENV_FILE" << ENVEOF
VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
VEILKEY_VAULT_NAME=$LABEL
VEILKEY_LABEL=$LABEL
VEILKEY_VAULTCENTER_URL=$CENTER_URL
VEILKEY_ADDR=:$PORT
VEILKEY_TLS_INSECURE=$TLS_INSECURE
VEILKEY_TLS_CERT=$TLS_CERT
VEILKEY_TLS_KEY=$TLS_KEY
VEILKEY_TRUSTED_IPS=$TRUSTED_IPS
ENVEOF
for extra_key in VEILKEY_BULK_APPLY_ALLOWED_PATHS VEILKEY_BULK_APPLY_ALLOWED_HOOKS VEILKEY_MANAGED_PATHS VEILKEY_CONTEXT_DIR VEILKEY_CONTEXT_FILE; do
    if [[ -n "${!extra_key:-}" ]]; then
        printf '%s=%s\n' "$extra_key" "${!extra_key}" >> "$ENV_FILE"
    fi
done

# [5] Service
if [[ "$USE_SYSTEMD" == "1" ]] && command -v systemctl &>/dev/null; then
    echo "[5/5] systemd 서비스..."
    cat > /etc/systemd/system/veilkey-localvault.service << SVCEOF
[Unit]
Description=VeilKey LocalVault
After=network.target
[Service]
Type=simple
EnvironmentFile=$ENV_FILE
ExecStart=$BIN server
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
SVCEOF
    systemctl daemon-reload
    systemctl enable veilkey-localvault >/dev/null 2>&1 || true
    systemctl restart veilkey-localvault >/dev/null 2>&1 || systemctl start veilkey-localvault
else
    echo "[5/5] 프로세스 시작..."
    if [[ -f "$DATA_DIR/localvault.pid" ]]; then
        OLD_PID=$(cat "$DATA_DIR/localvault.pid" 2>/dev/null || true)
        if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
            kill "$OLD_PID" 2>/dev/null || true
            sleep 1
        fi
        rm -f "$DATA_DIR/localvault.pid"
    fi
    cd "$DATA_DIR" && set -a && source "$ENV_FILE" && set +a
    nohup "$BIN" server > "$DATA_DIR/localvault.log" 2>&1 &
    echo $! > "$DATA_DIR/localvault.pid"
fi

HEALTH=""
for _ in $(seq 1 20); do
    HEALTH=$(curl -sk "$HEALTH_URL" 2>/dev/null || true)
    if [[ "$HEALTH" == *'"status":"ok"'* ]]; then
        echo "=== 완료 ==="
        echo "  Health: $HEALTH"
        exit 0
    fi
    sleep 1
done

echo "ERROR: localvault health check failed"
if [[ "$USE_SYSTEMD" == "1" ]] && command -v systemctl &>/dev/null; then
    journalctl -u veilkey-localvault -n 20 --no-pager || true
else
    tail -20 "$DATA_DIR/localvault.log" || true
fi
exit 1
