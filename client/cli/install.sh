#!/usr/bin/env bash
# VeilKey CLI — 원라인 설치 스크립트
# Usage:
#   curl -fsSL https://your-gitlab/veilkey/cli/install.sh | bash
#   bash install.sh                    # 소스에서 빌드
#   VEILKEY_CLI_BIN=/path/to/bin bash install.sh  # 커스텀 경로
set -euo pipefail

INSTALL_BIN="${VEILKEY_CLI_BIN:-/usr/local/bin/veilkey}"
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

# --- Root check (only if installing to /usr/local/bin) ---
if [[ "$INSTALL_BIN" == /usr/local/* ]] && [[ $EUID -ne 0 ]]; then
  error "root 권한이 필요합니다. sudo bash install.sh"
fi

# --- Build from source ---
if [[ -f "$SOURCE_DIR/go.mod" ]]; then
  info "소스에서 빌드..."
  command -v go >/dev/null 2>&1 || error "Go가 설치되어 있지 않습니다."
  cd "$SOURCE_DIR"
  CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_BIN" .
  chmod +x "$INSTALL_BIN"
  info "빌드 완료: $INSTALL_BIN"
else
  error "소스 디렉토리가 아닙니다. go.mod가 없습니다: $SOURCE_DIR"
fi

# --- Verify ---
CLI_OUTPUT=$("$INSTALL_BIN" 2>&1 || true)
if echo "$CLI_OUTPUT" | grep -qE 'veilkey|Usage|scan'; then
  echo ""
  info "VeilKey CLI 설치 완료!"
  echo ""
  echo "  Binary : $INSTALL_BIN"
  echo "  Version: $("$INSTALL_BIN" 2>&1 | head -1)"
  echo ""
  echo "  사용법:"
  echo "    veilkey scan <file>       시크릿 감지"
  echo "    veilkey filter <file>     시크릿 자동 치환"
  echo "    veilkey wrap <command>    명령 실행 + 시크릿 치환"
  echo "    veilkey exec <command>    VK: 해시 resolve + 실행"
  echo "    veilkey resolve <ref>     VK: 해시 → 원본 값"
  echo ""
else
  error "설치 후 검증 실패. $INSTALL_BIN 확인 필요."
fi
