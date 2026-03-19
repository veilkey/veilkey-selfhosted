#!/bin/bash
set -euo pipefail

# VeilKey veil CLI installer for macOS
# Usage: bash scripts/install-veil-mac.sh

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="/usr/local/bin"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:11181}"

echo "=== VeilKey veil CLI installer (macOS) ==="
echo "Repo: $REPO_ROOT"
echo "URL:  $VEILKEY_URL"
echo ""

# 1. Check Rust toolchain
if ! command -v cargo &>/dev/null; then
    echo "ERROR: Rust toolchain not found. Install from https://rustup.rs"
    exit 1
fi
echo "[1/6] Rust toolchain OK"

# 2. Build
echo "[2/6] Building veilkey-cli + veil-cli..."
cd "$REPO_ROOT"
cargo build --release --quiet
echo "  Built successfully"

# 3. Install binaries
echo "[3/6] Installing to $BIN_DIR (requires sudo)..."
RELEASE="$REPO_ROOT/target/release"
sudo cp "$RELEASE/veilkey-cli" "$BIN_DIR/veilkey-cli"
sudo cp "$RELEASE/veilkey-session-config" "$BIN_DIR/veilkey-session-config"
sudo cp "$RELEASE/veil" "$BIN_DIR/veil"
sudo cp "$RELEASE/veilkey" "$BIN_DIR/veilkey"

# 4. Ad-hoc code sign (required for macOS Gatekeeper)
echo "[4/6] Code signing (ad-hoc)..."
sudo codesign --force --sign - "$BIN_DIR/veilkey-cli"
sudo codesign --force --sign - "$BIN_DIR/veilkey-session-config"
sudo codesign --force --sign - "$BIN_DIR/veil"
sudo codesign --force --sign - "$BIN_DIR/veilkey"
echo "  Signed"

# 5. Shell config
echo "[5/6] Configuring shell..."
SHELL_RC="$HOME/.zshrc"
if [[ "$SHELL" == *"bash"* ]]; then
    SHELL_RC="$HOME/.bashrc"
fi

MARKER="# VeilKey veil CLI"
if ! grep -q "$MARKER" "$SHELL_RC" 2>/dev/null; then
    cat >> "$SHELL_RC" << SHELLEOF

$MARKER
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="\$HOME/.veilkey.yml"
export VEILKEY_BIN=$BIN_DIR/veilkey
export VEILKEY_CLI_BIN=$BIN_DIR/veilkey-cli
export VEILKEY_VK_BIN=$BIN_DIR/veilkey
export VEILKEY_SESSION_CONFIG_BIN=$BIN_DIR/veilkey-session-config
SHELLEOF
    echo "  Added to $SHELL_RC"
else
    echo "  Already in $SHELL_RC (skipped)"
fi

# 6. Config file
if [ ! -f "$HOME/.veilkey.yml" ]; then
    cp "$REPO_ROOT/services/veilkey-cli/examples/.veilkey.yml" "$HOME/.veilkey.yml"
    echo "  Created ~/.veilkey.yml"
else
    echo "  ~/.veilkey.yml exists (skipped)"
fi

# 7. Trust VaultCenter self-signed cert (optional)
echo "[6/6] TLS certificate..."
CERT_PATH="$REPO_ROOT/data/vaultcenter/certs/server.crt"
if [ -f "$CERT_PATH" ]; then
    echo "  Found VaultCenter cert at $CERT_PATH"
    read -p "  Trust this cert in system keychain? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "$CERT_PATH"
        echo "  Trusted"
    else
        echo "  Skipped (using VEILKEY_TLS_INSECURE=1 instead)"
    fi
else
    echo "  No cert found (using VEILKEY_TLS_INSECURE=1)"
fi

echo ""
echo "=== Installation complete ==="
echo ""
echo "Restart your terminal, then:"
echo "  veil              # Enter protected session"
echo "  veil status       # Check connection"
echo "  veil help         # Show commands"
echo ""
echo "Make sure docker compose is running:"
echo "  cd $REPO_ROOT && docker compose up -d"
