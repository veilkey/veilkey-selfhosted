#!/bin/bash
set -euo pipefail

# VeilKey installer for macOS
# Run from the cloned repo directory:
#   git clone https://github.com/veilkey/veilkey-selfhosted.git
#   cd veilkey-selfhosted
#   bash scripts/install-veil-mac.sh
#
# Multiple instances: clone to different directories, use different ports:
#   VEILKEY_URL=https://localhost:11182 bash scripts/install-veil-mac.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

# Must run from repo root
if [ ! -f "docker-compose.yml" ] || [ ! -d "services" ]; then
    echo "ERROR: veilkey-selfhosted repo root에서 실행하세요."
    echo "  git clone https://github.com/veilkey/veilkey-selfhosted.git"
    echo "  cd veilkey-selfhosted"
    echo "  bash scripts/install-veil-mac.sh"
    exit 1
fi

REPO_ROOT="$(pwd)"
BIN_DIR="${VEILKEY_BIN_DIR:-$HOME/.local/bin}"
VEILKEY_URL="${VEILKEY_URL:-https://localhost:11181}"

echo "=== VeilKey installer (macOS) ==="
echo ""
echo "  Project:  $REPO_ROOT"
echo "  Binaries: $BIN_DIR"
echo "  URL:      $VEILKEY_URL"
echo ""

# Ensure bin dir exists
mkdir -p "$BIN_DIR"

# Check prerequisites
for cmd in cargo docker; do
    if ! command -v "$cmd" &>/dev/null; then
        echo "ERROR: $cmd not found."
        case $cmd in
            cargo)  echo "  Install: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh" ;;
            docker) echo "  Install: https://docs.docker.com/desktop/install/mac-install/" ;;
        esac
        exit 1
    fi
done
echo "[1/5] Prerequisites OK"

# Build CLI
echo "[2/5] Building CLI (first run may take a few minutes)..."
cargo build --release --quiet 2>&1 | tail -3
echo "  Built"

# Install binaries
echo "[3/5] Installing to $BIN_DIR..."
RELEASE="$REPO_ROOT/target/release"
for bin in veil veilkey veilkey-cli veilkey-session-config; do
    if [ -f "$RELEASE/$bin" ]; then
        cp "$RELEASE/$bin" "$BIN_DIR/$bin"
        xattr -cr "$BIN_DIR/$bin" 2>/dev/null || true
        echo "  $bin ✓"
    fi
done

# Create .veilkey/env (project-local)
echo "[4/5] Creating .veilkey/env..."
mkdir -p "$REPO_ROOT/.veilkey/config"
cat > "$REPO_ROOT/.veilkey/env" << EOF
#!/bin/sh
export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="$REPO_ROOT/.veilkey/config/veilkey.yml"
export VEILKEY_CLI_BIN=$BIN_DIR/veilkey-cli
EOF

if [ ! -f "$REPO_ROOT/.veilkey/config/veilkey.yml" ]; then
    echo "threshold: 0.7" > "$REPO_ROOT/.veilkey/config/veilkey.yml"
fi

# Docker
echo "[5/5] Starting services..."
docker compose up --build -d 2>&1 | tail -5

echo ""
# Check PATH
if ! echo "$PATH" | tr ':' '\n' | grep -q "^$BIN_DIR$"; then
    echo "⚠️  $BIN_DIR 가 PATH에 없습니다:"
    echo "   echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
    echo ""
fi

echo "=== Installation complete ==="
echo ""
echo "1. 초기 설정:"
echo "   https://localhost:${VEILKEY_URL##*:} 접속 → 마스터/관리자 비밀번호 설정"
echo ""
echo "2. 사용:"
echo "   cd $REPO_ROOT"
echo "   source .veilkey/env && veil"
echo ""
echo "3. 여러 인스턴스:"
echo "   다른 폴더에 clone → 다른 포트로 설치:"
echo "   VEILKEY_URL=https://localhost:11182 bash scripts/install-veil-mac.sh"
echo ""
echo "4. 서버 재시작 후:"
echo "   마스터 비밀번호 입력 필요 (비밀번호는 메모리에만 존재)"
echo ""
