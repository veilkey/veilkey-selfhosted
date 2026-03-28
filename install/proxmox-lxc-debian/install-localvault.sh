#!/bin/bash
set -euo pipefail

# Standalone LocalVault installer for Proxmox host / LXC.
# Builds, initializes, and starts a LocalVault using VC-managed unlock.
#
# Usage:
#   VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
#     bash install/proxmox-lxc-debian/install-localvault.sh
#
# Options (env vars):
#   VEILKEY_CENTER_URL=                         VaultCenter URL (required)
#   VEILKEY_PORT=10180                          LocalVault listen port
#   VEILKEY_LABEL=$(hostname)                   Vault display name
#   VEILKEY_BULK_APPLY_ALLOWED_PATHS=           Comma-separated absolute paths for bulk-apply
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

if [ ! -d "services/localvault" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

CENTER_URL="${VEILKEY_CENTER_URL:-}"
PORT="${VEILKEY_PORT:-10180}"
VAULT_NAME="${VEILKEY_LABEL:-$(hostname)}"
BULK_PATHS="${VEILKEY_BULK_APPLY_ALLOWED_PATHS:-}"

if [[ -z "$CENTER_URL" ]]; then
    echo "ERROR: VEILKEY_CENTER_URL is required."
    echo ""
    echo "Usage:"
    echo "  VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> bash install/proxmox-lxc-debian/install-localvault.sh"
    exit 1
fi

REPO_ROOT="$(pwd)"
INSTALL_DIR="$REPO_ROOT/.localvault"
DATA_DIR="$INSTALL_DIR/data"
TLS_DIR="$DATA_DIR/tls"
PID_FILE="$INSTALL_DIR/localvault.pid"
LOG_FILE="$INSTALL_DIR/localvault.log"
BIN="$INSTALL_DIR/veilkey-localvault"
ENV_FILE="$INSTALL_DIR/.env"
HEALTH_URL="https://127.0.0.1:$PORT/health"
STATUS_URL="https://127.0.0.1:$PORT/api/status"

echo "=== LocalVault installer ==="
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Port:        $PORT"
echo "  Vault name:  $VAULT_NAME"
echo "  Center URL:  $CENTER_URL"
[[ -n "$BULK_PATHS" ]] && echo "  Bulk paths:  $BULK_PATHS"
echo ""

# --- [1/7] Check Go ---
if ! command -v go &>/dev/null; then
    echo "ERROR: Go not found."
    echo "  Install: apt install golang"
    exit 1
fi
echo "[1/7] Go $(go version | grep -oE 'go[0-9]+\.[0-9]+') OK"

# --- [2/7] Update source ---
echo "[2/7] Updating source..."
if git rev-parse --is-inside-work-tree &>/dev/null; then
    git pull --quiet 2>/dev/null || echo "  git pull skipped (detached or no remote)"
fi
echo "  Source up to date."

# --- [3/7] Build ---
echo "[3/7] Building LocalVault..."
mkdir -p "$INSTALL_DIR"
cd services/localvault
GOTOOLCHAIN=auto CGO_ENABLED=1 go build -o "$BIN" ./cmd 2>&1 | tail -5
cd "$REPO_ROOT"
echo "  Built: $BIN"

# --- [4/7] TLS certificate ---
echo "[4/7] TLS certificate..."
if [ -f "$TLS_DIR/cert.pem" ] && [ -f "$TLS_DIR/key.pem" ]; then
    echo "  Existing certificate preserved."
else
    mkdir -p "$TLS_DIR"
    # Detect IPs for SAN
    LOCAL_IPS=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v '^$' | head -5 || true)
    SAN="DNS:localhost,DNS:$(hostname),IP:127.0.0.1"
    for ip in $LOCAL_IPS; do
        SAN="$SAN,IP:$ip"
    done

    openssl req -x509 -newkey rsa:2048 \
        -keyout "$TLS_DIR/key.pem" -out "$TLS_DIR/cert.pem" \
        -days 3650 -nodes \
        -subj "/CN=$(hostname)" \
        -addext "subjectAltName=$SAN" \
        -addext "basicConstraints=critical,CA:FALSE" \
        -addext "keyUsage=digitalSignature,keyEncipherment" \
        -addext "extendedKeyUsage=serverAuth" 2>/dev/null
    echo "  Certificate generated (SAN: $SAN)"
fi

# --- [5/7] Setup .env ---
echo "[5/7] Setting up config..."
mkdir -p "$DATA_DIR"

cat > "$ENV_FILE" << ENVEOF
VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
VEILKEY_ADDR=:$PORT
VEILKEY_TRUSTED_IPS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1
VEILKEY_VAULT_NAME=$VAULT_NAME
VEILKEY_LABEL=$VAULT_NAME
VEILKEY_VAULTCENTER_URL=$CENTER_URL
VEILKEY_TLS_INSECURE=1
VEILKEY_TLS_CERT=$TLS_DIR/cert.pem
VEILKEY_TLS_KEY=$TLS_DIR/key.pem
ENVEOF
if [[ -n "$BULK_PATHS" ]]; then
    echo "VEILKEY_BULK_APPLY_ALLOWED_PATHS=$BULK_PATHS" >> "$ENV_FILE"
fi
echo "  Config written: $ENV_FILE"

# --- [6/7] Init ---
echo "[6/7] Initializing..."
if [ -f "$DATA_DIR/salt" ]; then
    echo "  Already initialized (salt exists). Skipping init."
else
    cd "$INSTALL_DIR"
    set -a
    source "$ENV_FILE"
    set +a
    "$BIN" init --root --center "$CENTER_URL"
    cd "$REPO_ROOT"
fi

# --- [7/7] Start ---
echo "[7/7] Starting LocalVault..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
        echo "  Stopping (PID: $OLD_PID)"
        kill "$OLD_PID" 2>/dev/null || true
        sleep 2
    fi
    rm -f "$PID_FILE"
fi

cd "$INSTALL_DIR"
set -a
source "$ENV_FILE"
set +a
nohup "$BIN" server > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
NEW_PID=$(cat "$PID_FILE")
cd "$REPO_ROOT"

sleep 1
if ! kill -0 "$NEW_PID" 2>/dev/null; then
    echo ""
    echo "❌ Start failed"
    echo "  Log: tail -20 $LOG_FILE"
    tail -20 "$LOG_FILE"
    exit 1
fi

HEALTH=""
for _ in $(seq 1 20); do
    HEALTH=$(curl -sk "$HEALTH_URL" 2>/dev/null || true)
    if [[ "$HEALTH" == *'"status":"ok"'* ]]; then
        break
    fi
    sleep 1
done

if [[ "$HEALTH" != *'"status":"ok"'* ]]; then
    echo ""
    echo "❌ LocalVault health check failed"
    echo "  Health: ${HEALTH:-<empty>}"
    echo "  Log: tail -40 $LOG_FILE"
    tail -40 "$LOG_FILE"
    exit 1
fi

STATUS_JSON=$(curl -sk "$STATUS_URL" 2>/dev/null || true)
echo ""
echo "=== Installation complete ==="
echo ""
echo "  Health: $HEALTH"
echo "  Status: ${STATUS_JSON:-<empty>}"
echo "  PID:    $NEW_PID"
echo "  Port:   $PORT (HTTPS)"
echo "  Log:    $LOG_FILE"
echo "  Data:   $DATA_DIR"
echo "  TLS:    $TLS_DIR/cert.pem"
echo "  Center: $CENTER_URL"
[[ -n "$BULK_PATHS" ]] && echo "  Bulk:   $BULK_PATHS"
echo ""
echo "Management:"
echo "  tail -f $LOG_FILE"
echo "  kill \$(cat $PID_FILE)"
echo "  bash install/proxmox-lxc-debian/uninstall-localvault.sh"
echo ""
