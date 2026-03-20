#!/bin/bash
set -euo pipefail

# VeilKey CLI installer for macOS
# Builds veil CLI from source, installs via npm, and codesigns.
#
# Usage:
#   bash install/macos/veil-cli/install.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

# Must run from repo root
if [ ! -f "Cargo.toml" ] || [ ! -d "services/veil-cli" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    exit 1
fi

REPO_ROOT="$(pwd)"
VC_PORT="${VAULTCENTER_HOST_PORT:-11181}"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:${VC_PORT}}"

echo "=== VeilKey CLI installer (macOS) ==="
echo ""

# [1/4] Check prerequisites
for cmd in npm cargo; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found."
        case $cmd in
            npm)    echo "  Install: brew install node" ;;
            cargo)  echo "  Install: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh" ;;
        esac
        exit 1
    fi
done
echo "[1/4] Prerequisites OK"

# [2/4] Build CLI
echo "[2/4] Building CLI..."
cargo build --release --quiet 2>&1 | tail -3
echo "  Built"

# [3/4] Install via npm + codesign (Gatekeeper-safe)
echo "[3/4] Installing via npm + codesign..."
mkdir -p "$REPO_ROOT/packages/veil-cli/native"
for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$REPO_ROOT/target/release/$bin" ]; then
        cp "$REPO_ROOT/target/release/$bin" "$REPO_ROOT/packages/veil-cli/native/$bin"
    fi
done
npm install -g "$REPO_ROOT/packages/veil-cli" 2>&1 | tail -2

NPM_NATIVE="$(npm prefix -g)/lib/node_modules/veilkey-cli/native"
echo "  Signing binaries (sudo required)..."
for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$NPM_NATIVE/$bin" ]; then
        sudo codesign --force --sign - "$NPM_NATIVE/$bin" 2>/dev/null || true
    fi
done
echo "  Installed + signed"

# [4/4] Create .veilkey/env (project-local)
echo "[4/4] Creating .veilkey/env..."
mkdir -p "$REPO_ROOT/.veilkey/config"
cat > "$REPO_ROOT/.veilkey/env" << EOF
#!/bin/sh
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="$REPO_ROOT/.veilkey/config/veilkey.yml"
export VEILKEY_CLI_BIN=$(npm prefix -g)/lib/node_modules/veilkey-cli/native/veilkey-cli
EOF

if [ ! -f "$REPO_ROOT/.veilkey/config/veilkey.yml" ]; then
    echo "threshold: 0.7" > "$REPO_ROOT/.veilkey/config/veilkey.yml"
fi

echo ""
echo "=== CLI installation complete ==="
echo "  Run: cd $REPO_ROOT && veil"
echo ""
