#!/bin/bash
set -euo pipefail

# veil-cli installer
# All configuration via env vars — no hardcoded values.
#
# Required:
#   VEILKEY_URL              VaultCenter URL (for login + mask-map)
#
# Optional:
#   VEILKEY_BIN_DIR          바이너리 경로 (default: /usr/local/bin)
#   VEILKEY_CONFIG_DIR       설정 경로 (default: ~/.veilkey)
#   VEILKEY_TLS_INSECURE     TLS 검증 스킵 (default: 1)
#   VEILKEY_BINARY_URL       바이너리 다운로드 URL

VEILKEY_URL="${VEILKEY_URL:-}"
if [[ -z "$VEILKEY_URL" ]]; then
    echo "ERROR: VEILKEY_URL is required"
    echo "  export VEILKEY_URL=https://<vaultcenter-host>:<port>"
    exit 1
fi

BIN_DIR="${VEILKEY_BIN_DIR:-/usr/local/bin}"
CONFIG_DIR="${VEILKEY_CONFIG_DIR:-$HOME/.veilkey}"
TLS_INSECURE="${VEILKEY_TLS_INSECURE:-1}"
BINARY_URL="${VEILKEY_BINARY_URL:-}"

echo "=== veil-cli installer ==="
echo "  URL: $VEILKEY_URL"

# [1] Binary
if [[ -n "$BINARY_URL" ]]; then
    echo "[1/3] 바이너리 다운로드..."
    mkdir -p "$BIN_DIR"
    for bin in veil veilkey-cli veilkey-session-config; do
        curl -sL "$BINARY_URL/$bin" -o "$BIN_DIR/$bin" && chmod +x "$BIN_DIR/$bin" 2>/dev/null || true
    done
elif command -v cargo &>/dev/null; then
    echo "[1/3] 소스 빌드..."
    REPO_ROOT="${VEILKEY_REPO_DIR:-$(pwd)}"
    if [ ! -f "$REPO_ROOT/Cargo.toml" ] || [ ! -d "$REPO_ROOT/services/veil-cli" ]; then
        echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
        exit 1
    fi
    cd "$REPO_ROOT"
    cargo build --release --quiet 2>&1 | tail -3
    for bin in veil veilkey-cli veilkey-session-config; do
        [ -f "target/release/$bin" ] && cp "target/release/$bin" "$BIN_DIR/$bin"
    done
else
    echo "ERROR: cargo 없음, 바이너리 없음. VEILKEY_BINARY_URL을 설정하세요."
    exit 1
fi
echo "  설치 완료: $BIN_DIR"

# [2] Config
echo "[2/3] 설정..."
mkdir -p "$CONFIG_DIR/config"

cat > "$CONFIG_DIR/env" << ENVEOF
#!/bin/sh
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=$TLS_INSECURE
export VEILKEY_CONFIG="$CONFIG_DIR/config/veilkey.yml"
export VEILKEY_CLI_BIN=$BIN_DIR/veilkey-cli
ENVEOF

[ -f "$CONFIG_DIR/config/veilkey.yml" ] || echo "threshold: 0.7" > "$CONFIG_DIR/config/veilkey.yml"

# [3] Shell integration
echo "[3/3] 셸 연동..."
SHELL_RC=""
if [ -f "$HOME/.bashrc" ]; then SHELL_RC="$HOME/.bashrc"
elif [ -f "$HOME/.zshrc" ]; then SHELL_RC="$HOME/.zshrc"
elif [ -f "$HOME/.profile" ]; then SHELL_RC="$HOME/.profile"
fi

if [[ -n "$SHELL_RC" ]] && ! grep -q "veilkey/env" "$SHELL_RC" 2>/dev/null; then
    echo "source $CONFIG_DIR/env" >> "$SHELL_RC"
    echo "  추가됨: $SHELL_RC"
fi

echo ""
echo "=== 설치 완료 ==="
echo "  source $CONFIG_DIR/env && veil"
