#!/bin/bash
set -euo pipefail

# VeilKey installer for macOS
# Usage:
#   curl -sL https://gist.githubusercontent.com/dalsoop/3136c5c2fd582b44357149771771659e/raw/install-veil-mac.sh | bash
#   OR: bash scripts/install-veil-mac.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

BIN_DIR="${VEILKEY_BIN_DIR:-$HOME/.local/bin}"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:11181}"
INSTALL_DIR="${VEILKEY_INSTALL_DIR:-$HOME/veilkey}"
REPO_URL="https://github.com/veilkey/veilkey-selfhosted.git"

echo "=== VeilKey installer (macOS) ==="
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Binaries:    $BIN_DIR"
echo "  URL:         $VEILKEY_URL"
echo ""

# Ensure bin dir exists
mkdir -p "$BIN_DIR"

# Check if already installed
EXISTING=false
if [ -d "$INSTALL_DIR/.git" ]; then
    EXISTING=true
    cd "$INSTALL_DIR"
    LOCAL_HASH=$(git rev-parse HEAD 2>/dev/null || echo "unknown")
    REMOTE_HASH=$(git ls-remote origin HEAD 2>/dev/null | cut -f1 || echo "unknown")

    if [ "$LOCAL_HASH" = "$REMOTE_HASH" ]; then
        echo "✅ 이미 최신 버전입니다. (${LOCAL_HASH:0:8})"
        echo "   다시 빌드합니다..."
    else
        echo "🔄 업데이트가 있습니다."
        echo "   현재: ${LOCAL_HASH:0:8}"
        echo "   최신: ${REMOTE_HASH:0:8}"
        echo "   업데이트합니다..."
    fi
fi

# Check prerequisites
for cmd in git cargo docker; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found."
        case $cmd in
            git)    echo "  Install: xcode-select --install" ;;
            cargo)  echo "  Install: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh" ;;
            docker) echo "  Install: https://docs.docker.com/desktop/install/mac-install/" ;;
        esac
        exit 1
    fi
done
echo "[1/6] Prerequisites OK"

# Clone or update repo
if [ "$EXISTING" = true ]; then
    echo "[2/6] Updating..."
    cd "$INSTALL_DIR"
    git pull --quiet
else
    if [ -d "$INSTALL_DIR" ] && [ "$(ls -A "$INSTALL_DIR" 2>/dev/null)" ]; then
        echo "ERROR: $INSTALL_DIR exists and is not empty."
        echo "  Remove it first: rm -rf $INSTALL_DIR"
        exit 1
    fi
    echo "[2/6] Cloning repository..."
    git clone --quiet "$REPO_URL" "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# Build
echo "[3/6] Building (this may take a few minutes on first run)..."
cargo build --release --quiet 2>&1 | tail -5
echo "  Built"

# Install binaries
echo "[4/6] Installing to $BIN_DIR..."
RELEASE="$INSTALL_DIR/target/release"
for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$RELEASE/$bin" ]; then
        cp "$RELEASE/$bin" "$BIN_DIR/$bin"
        xattr -cr "$BIN_DIR/$bin" 2>/dev/null || true
        echo "  $bin ✓"
    fi
done

# Create .veilkey/env (project-local config)
VEILKEY_CFG_DIR="$INSTALL_DIR/.veilkey"
echo "[5/6] Creating config..."
mkdir -p "$VEILKEY_CFG_DIR/config"
cat > "$VEILKEY_CFG_DIR/env" << EOF
#!/bin/sh
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="$VEILKEY_CFG_DIR/config/veilkey.yml"
export VEILKEY_CLI_BIN=$BIN_DIR/veilkey-cli
EOF

if [ ! -f "$VEILKEY_CFG_DIR/config/veilkey.yml" ]; then
    echo "threshold: 0.7" > "$VEILKEY_CFG_DIR/config/veilkey.yml"
fi

# Docker
echo "[6/6] Starting services..."
cd "$INSTALL_DIR"
docker compose up --build -d 2>&1 | tail -5

echo ""
# Check PATH
if ! echo "$PATH" | tr ':' '\n' | grep -q "^$BIN_DIR$"; then
    echo "⚠️  $BIN_DIR 가 PATH에 없습니다. 추가하세요:"
    echo "   echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc"
    echo ""
fi

echo "=== Installation complete ==="
echo ""
echo "1. 초기 설정:"
echo "   https://localhost:11181 접속 → 마스터/관리자 비밀번호 설정"
echo ""
echo "2. 사용:"
echo "   cd $INSTALL_DIR"
echo "   source .veilkey/env && veil"
echo ""
echo "3. 서버 재시작 후:"
echo "   마스터 비밀번호 입력 필요 (https://localhost:11181)"
echo "   비밀번호는 메모리에만 존재, 디스크에 저장되지 않음"
echo ""
