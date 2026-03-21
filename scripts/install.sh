#!/bin/bash
set -euo pipefail

# VeilKey bootstrap installer
# Downloads the veilkey-installer binary and runs platform detection.
#
# Usage: curl -sL https://veilkey.dev/install | bash
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO="veilkey/veilkey-selfhosted"
BIN_NAME="veilkey-installer"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64) ARCH="x86_64" ;;
    aarch64|arm64) ARCH="aarch64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux) TARGET="${BIN_NAME}-${OS}-${ARCH}" ;;
    darwin) TARGET="${BIN_NAME}-${OS}-${ARCH}" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

echo "=== VeilKey Installer ==="
echo ""
echo "  OS:   $OS"
echo "  Arch: $ARCH"
echo ""

# Try to download pre-built binary from GitHub Releases
RELEASE_URL="https://github.com/$REPO/releases/latest/download/$TARGET"
INSTALL_DIR="${VEILKEY_INSTALL_DIR:-/usr/local/bin}"

echo "Downloading $BIN_NAME..."
if curl -fsSL "$RELEASE_URL" -o "/tmp/$BIN_NAME" 2>/dev/null; then
    chmod +x "/tmp/$BIN_NAME"
    echo "Downloaded pre-built binary."
else
    echo "No pre-built binary available."
    echo ""
    echo "Install from source instead:"
    echo "  git clone https://github.com/$REPO.git"
    echo "  cd veilkey-selfhosted"
    echo "  cargo build --release -p veilkey-installer"
    echo "  ./target/release/veilkey-installer detect"
    exit 1
fi

# Run detection
echo ""
"/tmp/$BIN_NAME" detect

echo ""
echo "To install, move the binary and run:"
echo "  sudo mv /tmp/$BIN_NAME $INSTALL_DIR/$BIN_NAME"
echo "  $BIN_NAME <platform> [options]"
echo ""
