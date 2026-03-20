#!/bin/bash
set -euo pipefail

# veil-cli installer for Linux
# Builds and installs veil CLI to /usr/local/bin, configures connection to VeilKey.
# Works on: Proxmox host, LXC containers, any Linux with Rust toolchain.
#
# Usage:
#   VEILKEY_URL=https://<HOST>:<VC_PORT> bash install/common/install-veil-cli.sh
#
# Options (env vars):
#   VEILKEY_URL=   VeilKey server URL (required)
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

if [ ! -f "Cargo.toml" ] || [ ! -d "services/veil-cli" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

if [[ -z "${VEILKEY_URL:-}" ]]; then
    echo "ERROR: VEILKEY_URL is required."
    echo ""
    echo "Usage:"
    echo "  VEILKEY_URL=https://<HOST>:<VC_PORT> bash install/common/install-veil-cli.sh"
    exit 1
fi

REPO_ROOT="$(pwd)"
BIN_DIR="/usr/local/bin"
CONFIG_DIR="$HOME/.veilkey"

echo "=== veil-cli installer (Linux) ==="
echo ""
echo "  URL: $VEILKEY_URL"
echo ""

# [1/3] Check prerequisites
if ! command -v cargo &>/dev/null; then
    echo "ERROR: cargo not found."
    echo "  Install: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
    exit 1
fi
echo "[1/3] Prerequisites OK"

# [2/3] Build
echo "[2/3] Building veil CLI..."
cargo build --release --quiet 2>&1 | tail -3

for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$REPO_ROOT/target/release/$bin" ]; then
        cp "$REPO_ROOT/target/release/$bin" "$BIN_DIR/$bin"
    fi
done
echo "  Installed to $BIN_DIR"

# [3/3] Configure
echo "[3/3] Creating config..."
mkdir -p "$CONFIG_DIR/config"

cat > "$CONFIG_DIR/env" << EOF
#!/bin/sh
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="$CONFIG_DIR/config/veilkey.yml"
export VEILKEY_CLI_BIN=$BIN_DIR/veilkey-cli
EOF

[ -f "$CONFIG_DIR/config/veilkey.yml" ] || echo "threshold: 0.7" > "$CONFIG_DIR/config/veilkey.yml"

echo ""
echo "=== Installation complete ==="
echo ""
echo "Usage:"
echo "  source $CONFIG_DIR/env && veil"
echo ""
echo "Or add to your shell profile:"
echo "  echo 'source $CONFIG_DIR/env' >> ~/.bashrc"
echo ""
