#!/bin/bash
set -euo pipefail

# Standalone LocalVault installer for Proxmox host
# Builds, installs, initializes, and starts a LocalVault on the Proxmox host.
# Connects to an existing VaultCenter (e.g. in LXC).
#
# Usage:
#   VEILKEY_CENTER_URL=https://<CT_IP>:<VC_PORT> \
#     bash install/proxmox-lxc-debian/install-localvault.sh
#
# Options (env vars):
#   VEILKEY_CENTER_URL=     VaultCenter URL (required)
#   VEILKEY_PORT=10180      LocalVault listen port
#   VEILKEY_NAME=$(hostname) Vault display name
#   VEILKEY_PASSWORD=       Master password (prompted if not set)
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

if [ ! -d "services/localvault" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

CENTER_URL="${VEILKEY_CENTER_URL:-}"
PORT="${VEILKEY_PORT:-10180}"
VAULT_NAME="${VEILKEY_NAME:-$(hostname)}"

if [[ -z "$CENTER_URL" ]]; then
    echo "ERROR: VEILKEY_CENTER_URL is required."
    echo ""
    echo "Usage:"
    echo "  VEILKEY_CENTER_URL=https://<CT_IP>:<VC_PORT> bash install/proxmox-lxc-debian/install-localvault.sh"
    exit 1
fi

if [[ -z "${VEILKEY_PASSWORD:-}" ]]; then
    read -s -p "Master password: " VEILKEY_PASSWORD
    echo ""
    if [[ -z "$VEILKEY_PASSWORD" ]]; then
        echo "ERROR: Password cannot be empty."
        exit 1
    fi
fi

REPO_ROOT="$(pwd)"
INSTALL_DIR="$REPO_ROOT/.localvault"
DATA_DIR="$INSTALL_DIR/data"
PID_FILE="$INSTALL_DIR/localvault.pid"
LOG_FILE="$INSTALL_DIR/localvault.log"
BIN="$INSTALL_DIR/veilkey-localvault"
ENV_FILE="$INSTALL_DIR/.env"

echo "=== LocalVault installer (Proxmox host) ==="
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Port:        $PORT"
echo "  Vault name:  $VAULT_NAME"
echo "  Center URL:  $CENTER_URL"
echo ""

# [1/6] Check Go
if ! command -v go &>/dev/null; then
    echo "ERROR: Go not found."
    echo "  Install: apt install golang"
    exit 1
fi
echo "[1/6] Go $(go version | grep -oE 'go[0-9]+\.[0-9]+') OK"

# [2/6] Build
echo "[2/6] Building LocalVault..."
mkdir -p "$INSTALL_DIR"
cd services/localvault
CGO_ENABLED=1 go build -o "$BIN" ./cmd 2>&1 | tail -5
cd "$REPO_ROOT"
echo "  Built: $BIN"

# [3/6] Setup data + env
echo "[3/6] Setting up data directory..."
mkdir -p "$DATA_DIR"

if [ -f "$ENV_FILE" ]; then
    echo "  Existing config preserved: $ENV_FILE"
else
    cat > "$ENV_FILE" << ENVEOF
VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
VEILKEY_ADDR=:$PORT
VEILKEY_TRUSTED_IPS=10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1
VEILKEY_MODE=root
VEILKEY_VAULT_NAME=$VAULT_NAME
VEILKEY_VAULTCENTER_URL=$CENTER_URL
VEILKEY_TLS_INSECURE=1
ENVEOF
    echo "  Config created: $ENV_FILE"
fi

# [4/6] Stop existing process
echo "[4/6] Checking existing process..."
if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE" 2>/dev/null || echo "")
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
        echo "  Stopping (PID: $OLD_PID)"
        kill "$OLD_PID" 2>/dev/null || true
        sleep 2
    fi
    rm -f "$PID_FILE"
fi

# [5/6] Init (if not already initialized)
echo "[5/6] Initializing..."
if [ -f "$DATA_DIR/salt" ]; then
    echo "  Already initialized (salt exists). Skipping init."
else
    cd "$INSTALL_DIR"
    set -a; source "$ENV_FILE"; set +a
    echo "$VEILKEY_PASSWORD" | "$BIN" init --root --center "$CENTER_URL"
    cd "$REPO_ROOT"
fi

# [6/6] Start + unlock
echo "[6/6] Starting LocalVault..."
cd "$INSTALL_DIR"
set -a; source "$ENV_FILE"; set +a
# VEILKEY_PASSWORD must not be in process env (security check)
SAVED_PASSWORD="$VEILKEY_PASSWORD"
unset VEILKEY_PASSWORD
nohup "$BIN" server > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"
NEW_PID=$(cat "$PID_FILE")
cd "$REPO_ROOT"

sleep 3
if kill -0 "$NEW_PID" 2>/dev/null; then
    # Unlock
    UNLOCK=$(curl -s -X POST "http://127.0.0.1:$PORT/api/unlock" \
        -H 'Content-Type: application/json' \
        -d "{\"password\":\"$SAVED_PASSWORD\"}" 2>/dev/null || echo '{"error":"failed"}')

    if echo "$UNLOCK" | grep -q '"unlocked"'; then
        HEALTH=$(curl -s "http://127.0.0.1:$PORT/health" 2>/dev/null || echo "")
        echo ""
        echo "=== Installation complete ==="
        echo ""
        echo "  Status: $HEALTH"
        echo "  PID:    $NEW_PID"
        echo "  Port:   $PORT"
        echo "  Log:    $LOG_FILE"
        echo "  Data:   $DATA_DIR"
        echo "  Center: $CENTER_URL"
        echo ""
        echo "Management:"
        echo "  tail -f $LOG_FILE"
        echo "  kill \$(cat $PID_FILE)"
        echo "  bash install/proxmox-lxc-debian/uninstall-localvault.sh"
        echo ""
    else
        echo ""
        echo "⚠️  Started but unlock failed: $UNLOCK"
        echo "  Log: tail -20 $LOG_FILE"
    fi
else
    echo ""
    echo "❌ Start failed"
    echo "  Log: tail -20 $LOG_FILE"
    tail -20 "$LOG_FILE"
    exit 1
fi
