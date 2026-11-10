#!/bin/bash
set -euo pipefail

# Standalone LocalVault installer for Proxmox host / LXC
# Builds, installs, initializes, and starts a LocalVault.
# Connects to an existing VaultCenter.
# Re-running updates (git pull + rebuild + restart).
#
# Usage:
#   VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
#     bash install/proxmox-lxc-debian/install-localvault.sh
#
# Options (env vars):
#   VEILKEY_CENTER_URL=                         VaultCenter URL (required)
#   VEILKEY_PORT=10180                          LocalVault listen port
#   VEILKEY_NAME=$(hostname)                    Vault display name
#   VEILKEY_REG_TOKEN=                          Registration token (required for first init)
#   VEILKEY_TRUSTED_IPS=                        Defaults to the resolved VaultCenter address only
#   VEILKEY_BULK_APPLY_ALLOWED_PATHS=           Comma-separated absolute paths for bulk-apply
#
# Changes (v0.5.1):
#   - Trust only the VaultCenter address by default
#   - systemd 서비스 등록 (nohup/PID 방식 폐기)
#   - VaultCenter-managed unlock (no master password files)
#   - 일일 백업 cron 자동 등록
#   - veilkey CLI 빌드/설치
#   - 설치 후 health check

if [ ! -d "services/localvault" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

CENTER_URL="${VEILKEY_CENTER_URL:-}"
PORT="${VEILKEY_PORT:-10180}"
VAULT_NAME="${VEILKEY_NAME:-$(hostname)}"
BULK_PATHS="${VEILKEY_BULK_APPLY_ALLOWED_PATHS:-}"

if [[ -z "$CENTER_URL" ]]; then
    echo "ERROR: VEILKEY_CENTER_URL is required."
    echo ""
    echo "Usage:"
    echo "  VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> bash install/proxmox-lxc-debian/install-localvault.sh"
    exit 1
fi

source "$(dirname "${BASH_SOURCE[0]}")/../common/localvault-trusted-ip.sh"
TRUSTED_IPS="${VEILKEY_TRUSTED_IPS:-$(veilkey_center_ip "$CENTER_URL")}"

REPO_ROOT="$(pwd)"
INSTALL_DIR="$REPO_ROOT/.localvault"
DATA_DIR="$INSTALL_DIR/data"
TLS_DIR="$DATA_DIR/tls"
BACKUP_DIR="$DATA_DIR/backups"
LOG_FILE="$INSTALL_DIR/localvault.log"
BIN="$INSTALL_DIR/veilkey-localvault"
ENV_FILE="$INSTALL_DIR/.env"
SERVICE_NAME="veilkey-localvault"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

if [ ! -f "$DATA_DIR/salt" ] && [[ -z "${VEILKEY_REG_TOKEN:-}" ]]; then
    echo "ERROR: VEILKEY_REG_TOKEN is required for initial registration."
    exit 1
fi

echo "=== LocalVault installer ==="
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Port:        $PORT"
echo "  Vault name:  $VAULT_NAME"
echo "  Center URL:  $CENTER_URL"
[[ -n "$BULK_PATHS" ]] && echo "  Bulk paths:  $BULK_PATHS"
echo ""

# --- [1/10] Check Go ---
if ! command -v go &>/dev/null; then
    echo "ERROR: Go not found."
    echo "  Install: apt install golang"
    exit 1
fi
echo "[1/10] Go $(go version | grep -oE 'go[0-9]+\.[0-9]+') OK"

# --- [2/10] Update source ---
echo "[2/10] Updating source..."
if git rev-parse --is-inside-work-tree &>/dev/null; then
    git pull --quiet 2>/dev/null || echo "  git pull skipped (detached or no remote)"
fi
echo "  Source up to date."

# --- [3/10] Build LocalVault ---
echo "[3/10] Building LocalVault..."
mkdir -p "$INSTALL_DIR"
cd services/localvault
CGO_ENABLED=1 go build -o "$BIN" ./cmd 2>&1 | tail -5
cd "$REPO_ROOT"
echo "  Built: $BIN"

# --- [4/10] Build veilkey CLI ---
echo "[4/10] Building veilkey CLI..."
if [ -d "services/veil-cli" ] && command -v cargo &>/dev/null; then
    cargo build --release -p veil-cli-rs --bin veilkey-cli 2>&1 | tail -3
    cp target/release/veilkey-cli /usr/local/bin/veilkey-cli
    ln -sf /usr/local/bin/veilkey-cli /usr/local/bin/veilkey
    echo "  Installed: /usr/local/bin/veilkey"
elif [ -d "services/veil-cli" ]; then
    echo "  Rust not found. Skipping CLI build (install rust: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh)"
else
    echo "  veil-cli source not found. Skipping."
fi

# --- [5/10] TLS certificate ---
echo "[5/10] TLS certificate..."
if [ -f "$TLS_DIR/cert.pem" ] && [ -f "$TLS_DIR/key.pem" ]; then
    echo "  Existing certificate preserved."
else
    mkdir -p "$TLS_DIR"
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

# --- [6/10] Setup .env ---
echo "[6/10] Setting up config..."
mkdir -p "$DATA_DIR" "$BACKUP_DIR"

if [ -f "$ENV_FILE" ]; then
    echo "  Existing config preserved: $ENV_FILE"
    # Remove obsolete plaintext credentials without creating a backup copy.
    sed -i -e '/^[[:space:]]*\(export[[:space:]][[:space:]]*\)\{0,1\}VEILKEY_[A-Z_]*PASSWORD[A-Z_]*=/d' \
        -e '/^VEILKEY_TRUSTED_IPS=/d' -e '/^VEILKEY_TLS_INSECURE=/d' "$ENV_FILE"
    printf '\nVEILKEY_TRUSTED_IPS=%s\nVEILKEY_TLS_INSECURE=0\n' "$TRUSTED_IPS" >> "$ENV_FILE"
    echo "  Updated trust settings; removed legacy password configuration."
    # Update bulk-apply paths if provided
    if [[ -n "$BULK_PATHS" ]]; then
        if grep -q "^VEILKEY_BULK_APPLY_ALLOWED_PATHS=" "$ENV_FILE" 2>/dev/null; then
            sed -i "s|^VEILKEY_BULK_APPLY_ALLOWED_PATHS=.*|VEILKEY_BULK_APPLY_ALLOWED_PATHS=$BULK_PATHS|" "$ENV_FILE"
        else
            echo "VEILKEY_BULK_APPLY_ALLOWED_PATHS=$BULK_PATHS" >> "$ENV_FILE"
        fi
        echo "  Updated bulk-apply paths."
    fi
else
    cat > "$ENV_FILE" << ENVEOF
VEILKEY_DB_PATH=$DATA_DIR/veilkey.db
VEILKEY_ADDR=:$PORT
VEILKEY_TRUSTED_IPS=$TRUSTED_IPS
VEILKEY_MODE=root
VEILKEY_VAULT_NAME=$VAULT_NAME
VEILKEY_VAULTCENTER_URL=$CENTER_URL
VEILKEY_TLS_INSECURE=0
VEILKEY_TLS_CERT=$TLS_DIR/cert.pem
VEILKEY_TLS_KEY=$TLS_DIR/key.pem
ENVEOF
    if [[ -n "$BULK_PATHS" ]]; then
        echo "" >> "$ENV_FILE"
        echo "VEILKEY_BULK_APPLY_ALLOWED_PATHS=$BULK_PATHS" >> "$ENV_FILE"
    fi
    echo "  Config created: $ENV_FILE"
fi

# Preserve only public CA paths, never credentials, for the service process.
for ca_key in SSL_CERT_FILE SSL_CERT_DIR; do
    if [[ -n "${!ca_key:-}" ]]; then
        sed -i "/^${ca_key}=/d" "$ENV_FILE"
        printf '%s=%s\n' "$ca_key" "${!ca_key}" >> "$ENV_FILE"
    fi
done

# --- [7/10] Stop existing process ---
echo "[7/10] Stopping existing process..."
if systemctl is-active "$SERVICE_NAME" &>/dev/null; then
    systemctl stop "$SERVICE_NAME"
    echo "  Stopped systemd service."
elif [ -f "$INSTALL_DIR/localvault.pid" ]; then
    OLD_PID=$(cat "$INSTALL_DIR/localvault.pid" 2>/dev/null || echo "")
    if [[ -n "$OLD_PID" ]] && kill -0 "$OLD_PID" 2>/dev/null; then
        kill "$OLD_PID" 2>/dev/null || true
        echo "  Stopped legacy PID process."
        sleep 2
    fi
    rm -f "$INSTALL_DIR/localvault.pid"
else
    echo "  Not running."
fi

# --- [8/10] Init (if not already initialized) ---
echo "[8/10] Initializing..."
if [ -f "$DATA_DIR/salt" ]; then
    echo "  Already initialized (salt exists). Skipping init."
else
    cd "$INSTALL_DIR"
    set -a; source "$ENV_FILE"; set +a
    "$BIN" init --root --center "$CENTER_URL" --token "$VEILKEY_REG_TOKEN"
    cd "$REPO_ROOT"
fi

# --- [9/10] Register systemd service ---
echo "[9/10] Registering systemd service..."
cat > "$SERVICE_FILE" << SVCEOF
[Unit]
Description=VeilKey LocalVault
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
EnvironmentFile=$ENV_FILE
ExecStart=$BIN server
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
SVCEOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" --quiet
systemctl start "$SERVICE_NAME"
echo "  Service registered and started."

# Clean up legacy PID file
rm -f "$INSTALL_DIR/localvault.pid"

# --- [10/10] Setup backup cron ---
echo "[10/10] Setting up daily backup..."
BACKUP_SCRIPT="/usr/local/bin/veilkey-backup.sh"
cat > "$BACKUP_SCRIPT" << BKEOF
#!/bin/bash
DB=$DATA_DIR/veilkey.db
SALT=$DATA_DIR/salt
TS=\$(date +%Y%m%d-%H%M%S)
mkdir -p $BACKUP_DIR
cp "\$DB" "$BACKUP_DIR/veilkey.db.\$TS"
cp "\$SALT" "$BACKUP_DIR/salt.\$TS"
find $BACKUP_DIR -name "*.20*" -mtime +7 -delete
BKEOF
chmod +x "$BACKUP_SCRIPT"

if ! crontab -l 2>/dev/null | grep -q "veilkey-backup"; then
    (crontab -l 2>/dev/null; echo "0 4 * * * $BACKUP_SCRIPT") | crontab -
    echo "  Daily backup cron registered (04:00, 7-day retention)."
else
    echo "  Backup cron already exists."
fi

# --- Verify ---
echo ""
sleep 3
HEALTH=$(curl -sS --cacert "$TLS_DIR/cert.pem" "https://127.0.0.1:$PORT/api/status" 2>/dev/null || echo '{"error":"unreachable"}')
LOCKED=$(echo "$HEALTH" | grep -o '"locked":[a-z]*' | cut -d: -f2)

if [[ "$LOCKED" == "false" ]]; then
    SECRETS=$(echo "$HEALTH" | grep -o '"secrets_count":[0-9]*' | cut -d: -f2)
    echo "=== Installation complete ==="
    echo ""
    echo "  Status:   unlocked"
    echo "  Secrets:  ${SECRETS:-0}"
    echo "  Port:     $PORT (HTTPS)"
    echo "  Service:  systemctl status $SERVICE_NAME"
    echo "  Log:      journalctl -u $SERVICE_NAME -f"
    echo "  Backup:   $BACKUP_DIR (daily 04:00, 7-day retention)"
    echo "  Center:   $CENTER_URL"
    if command -v veilkey &>/dev/null; then
        echo "  CLI:      veilkey status"
    fi
    [[ -n "$BULK_PATHS" ]] && echo "  Bulk:     $BULK_PATHS"
    echo ""
    echo "Management:"
    echo "  systemctl restart $SERVICE_NAME"
    echo "  systemctl stop $SERVICE_NAME"
    echo "  bash install/proxmox-lxc-debian/uninstall-localvault.sh"
    echo ""
elif [[ "$LOCKED" == "true" ]]; then
    echo "⚠️  Started but still LOCKED."
    echo "  Unlock VaultCenter and check registration and CA trust:"
    echo "    journalctl -u $SERVICE_NAME -n 20"
    echo "  See docs/setup/tls.md and docs/setup/localvault/registration.md."
else
    echo "❌ Health check failed: $HEALTH"
    echo "  journalctl -u $SERVICE_NAME -n 20"
    exit 1
fi
