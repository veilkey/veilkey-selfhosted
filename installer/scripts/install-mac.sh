#!/usr/bin/env bash
set -euo pipefail

#
# VeilKey Mac Installer
# 소스로부터 빌드하여 macOS에 설치합니다.
#
# 사용법:
#   ./install-mac.sh install          # 전체 설치 (Go 설치 + 빌드 + 배치 + launchd)
#   ./install-mac.sh build            # 빌드만
#   ./install-mac.sh deploy           # 이미 빌드된 바이너리 배치 + launchd 등록
#   ./install-mac.sh start            # 서비스 시작
#   ./install-mac.sh stop             # 서비스 중지
#   ./install-mac.sh status           # 서비스 상태 확인
#   ./install-mac.sh health           # 헬스 체크
#   ./install-mac.sh uninstall        # 전체 제거
#   ./install-mac.sh logs [service]   # 로그 보기
#

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Resolve repo root: installer/scripts/ → repo root
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
SOURCE_DIR="${VEILKEY_SOURCE_DIR:-${REPO_ROOT}}"

# 설치 경로
VEILKEY_BIN="/usr/local/bin"
VEILKEY_ETC="/usr/local/etc/veilkey"
VEILKEY_DATA="/usr/local/var/veilkey"
VEILKEY_LOG="/usr/local/var/log/veilkey"
LAUNCHD_DIR="${HOME}/Library/LaunchAgents"

# 기본 포트
KEYCENTER_PORT="${VEILKEY_KEYCENTER_PORT:-10181}"
LOCALVAULT_PORT="${VEILKEY_LOCALVAULT_PORT:-10180}"

# 빌드 출력 디렉토리
BUILD_DIR="${SCRIPT_DIR}/build"

# 색상
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

info()  { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
die()   { err "$@"; exit 1; }

# ── Go 설치 확인 및 설치 ───────────────────────────────────────────

ensure_go() {
  if command -v go >/dev/null 2>&1; then
    info "Go 이미 설치됨: $(go version)"
    return 0
  fi

  info "Go가 없습니다. Homebrew로 설치합니다..."
  if ! command -v brew >/dev/null 2>&1; then
    die "Homebrew가 필요합니다. https://brew.sh 에서 설치해주세요."
  fi

  brew install go
  ok "Go 설치 완료: $(go version)"
}

# ── 빌드 ──────────────────────────────────────────────────────────

build_service() {
  local name="$1" src_dir="$2" binary_name="$3"
  shift 3
  local build_tags="${1:-}"; [[ -n "${build_tags}" ]] && shift || true

  info "빌드 중: ${name} → ${binary_name}"
  [[ -d "${src_dir}" ]] || die "소스 디렉토리 없음: ${src_dir}"
  mkdir -p "${BUILD_DIR}"

  local build_args=(-ldflags="-s -w" -o "${BUILD_DIR}/${binary_name}")
  [[ -n "${build_tags}" ]] && build_args+=(-tags "${build_tags}")

  (cd "${src_dir}" && CGO_ENABLED=1 go build "${build_args[@]}" "$@")
  ok "빌드 완료: ${binary_name}"
}

cmd_build() {
  ensure_go

  info "=== VeilKey 빌드 시작 ==="

  build_service "KeyCenter" \
    "${SOURCE_DIR}/services/keycenter" "veilkey-keycenter" "" ./cmd

  build_service "LocalVault" \
    "${SOURCE_DIR}/services/localvault" "veilkey-localvault" "" ./cmd

  build_service "Proxy" \
    "${SOURCE_DIR}/services/proxy" "veilkey-proxy" "" ./cmd/veilkey-proxy

  build_service "Session Config" \
    "${SOURCE_DIR}/services/proxy" "veilkey-session-config" "" ./cmd/veilkey-session-config

  # CLI는 CGO 불필요
  info "빌드 중: CLI → veilkey"
  mkdir -p "${BUILD_DIR}"
  (cd "${SOURCE_DIR}/client/cli" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BUILD_DIR}/veilkey" .)
  ok "빌드 완료: veilkey"

  echo ""
  ok "=== 전체 빌드 완료 ==="
  ls -lh "${BUILD_DIR}/"
}

# ── 배치 ──────────────────────────────────────────────────────────

cmd_deploy() {
  info "=== VeilKey 배치 시작 ==="

  local binaries=(veilkey-keycenter veilkey-localvault veilkey-proxy veilkey-session-config veilkey)
  for bin in "${binaries[@]}"; do
    [[ -f "${BUILD_DIR}/${bin}" ]] || die "빌드 결과물 없음: ${BUILD_DIR}/${bin} — 먼저 build를 실행하세요."
  done

  info "디렉토리 생성..."
  sudo mkdir -p "${VEILKEY_ETC}" "${VEILKEY_DATA}/keycenter" "${VEILKEY_DATA}/localvault" "${VEILKEY_LOG}"
  mkdir -p "${LAUNCHD_DIR}"
  sudo chown -R "$(whoami)" "${VEILKEY_DATA}" "${VEILKEY_LOG}"

  info "바이너리 설치..."
  for bin in "${binaries[@]}"; do
    sudo install -m 0755 "${BUILD_DIR}/${bin}" "${VEILKEY_BIN}/${bin}"
    ok "  ${VEILKEY_BIN}/${bin}"
  done

  # veilroot scripts (curl/wget/http 래퍼)
  for script in veilkey-veilroot-curl veilkey-veilroot-wget veilkey-veilroot-http; do
    local src="${SOURCE_DIR}/services/proxy/deploy/host/${script}"
    [[ -f "${src}" ]] && sudo install -m 0755 "${src}" "${VEILKEY_BIN}/${script}"
  done

  write_config_files
  write_password_files
  write_launchd_plists
  write_shell_profile

  init_keycenter
  init_localvault

  echo ""
  ok "=== 배치 완료 ==="
  echo ""
  info "다음 단계:"
  echo "  1. 셸 반영:       source ~/.zshrc  (또는 새 터미널)"
  echo "  2. 서비스 시작:   $0 start"
}

write_config_files() {
  info "설정 파일 생성..."

  if [[ ! -f "${VEILKEY_ETC}/keycenter.env" ]]; then
    sudo tee "${VEILKEY_ETC}/keycenter.env" >/dev/null <<EOF
VEILKEY_PASSWORD_FILE=${VEILKEY_ETC}/keycenter.password
VEILKEY_ADDR=:${KEYCENTER_PORT}
VEILKEY_DB_PATH=${VEILKEY_DATA}/keycenter/veilkey.db
VEILKEY_TLS_CERT=${VEILKEY_ETC}/tls/server.crt
VEILKEY_TLS_KEY=${VEILKEY_ETC}/tls/server.key
EOF
    ok "  ${VEILKEY_ETC}/keycenter.env"
  else
    warn "  ${VEILKEY_ETC}/keycenter.env (이미 존재, 건너뜀)"
  fi

  if [[ ! -f "${VEILKEY_ETC}/localvault.env" ]]; then
    sudo tee "${VEILKEY_ETC}/localvault.env" >/dev/null <<EOF
VEILKEY_PASSWORD_FILE=${VEILKEY_ETC}/localvault.password
VEILKEY_ADDR=:${LOCALVAULT_PORT}
VEILKEY_DB_PATH=${VEILKEY_DATA}/localvault/veilkey.db
VEILKEY_KEYCENTER_URL=https://127.0.0.1:${KEYCENTER_PORT}
VEILKEY_TLS_CERT=${VEILKEY_ETC}/tls/server.crt
VEILKEY_TLS_KEY=${VEILKEY_ETC}/tls/server.key
EOF
    ok "  ${VEILKEY_ETC}/localvault.env"
  else
    warn "  ${VEILKEY_ETC}/localvault.env (이미 존재, 건너뜀)"
  fi

  if [[ ! -f "${VEILKEY_ETC}/proxy.env" ]]; then
    sudo tee "${VEILKEY_ETC}/proxy.env" >/dev/null <<EOF
VEILKEY_LOCALVAULT_URL=https://127.0.0.1:${LOCALVAULT_PORT}
VEILKEY_HUB_URL=https://127.0.0.1:${KEYCENTER_PORT}
VEILKEY_PROXY_ACCESS_LOG_FORMAT=jsonl
EOF
    ok "  ${VEILKEY_ETC}/proxy.env"
  else
    warn "  ${VEILKEY_ETC}/proxy.env (이미 존재, 건너뜀)"
  fi

  if [[ ! -f "${VEILKEY_ETC}/session-tools.toml" ]]; then
    local example="${SOURCE_DIR}/services/proxy/deploy/host/session-tools.toml.example"
    if [[ -f "${example}" ]]; then
      sudo cp "${example}" "${VEILKEY_ETC}/session-tools.toml"
    else
      sudo touch "${VEILKEY_ETC}/session-tools.toml"
    fi
    ok "  ${VEILKEY_ETC}/session-tools.toml"
  fi
}

write_password_files() {
  info "비밀번호 파일 생성..."

  for svc in keycenter localvault; do
    local pw_file="${VEILKEY_ETC}/${svc}.password"
    if [[ ! -f "${pw_file}" ]]; then
      local tmp_pw
      tmp_pw="$(openssl rand -base64 24)"
      sudo bash -c "printf '%s' '${tmp_pw}' > '${pw_file}'"
      sudo chown "$(whoami)" "${pw_file}"
      chmod 600 "${pw_file}"
      ok "  ${pw_file} (자동 생성됨 — 필요 시 변경하세요)"
    else
      warn "  ${pw_file} (이미 존재, 건너뜀)"
    fi
  done
}

init_keycenter() {
  local salt_file="${VEILKEY_DATA}/keycenter/salt"
  if [[ -f "${salt_file}" ]]; then
    warn "KeyCenter 이미 초기화됨 (건너뜀)"
    return 0
  fi

  info "KeyCenter HKM 루트 노드 초기화..."
  cat "${VEILKEY_ETC}/keycenter.password" | \
    VEILKEY_DB_PATH="${VEILKEY_DATA}/keycenter/veilkey.db" \
    "${VEILKEY_BIN}/veilkey-keycenter" init --root 2>&1
  ok "KeyCenter 초기화 완료"
}

init_localvault() {
  local salt_file="${VEILKEY_DATA}/localvault/salt"
  if [[ -f "${salt_file}" ]]; then
    warn "LocalVault 이미 초기화됨 (건너뜀)"
    return 0
  fi

  info "LocalVault 초기화..."
  cat "${VEILKEY_ETC}/localvault.password" | \
    VEILKEY_DB_PATH="${VEILKEY_DATA}/localvault/veilkey.db" \
    "${VEILKEY_BIN}/veilkey-localvault" init --root 2>&1
  ok "LocalVault 초기화 완료"
}

write_shell_profile() {
  info "셸 프로파일 설정..."

  local profile_file="${HOME}/.veilkey.sh"
  cat > "${profile_file}" <<PROFILE
# ── VeilKey Shell Profile ──────────────────────────────────────────
# 자동 생성됨 (install-mac.sh). 수동 편집 가능.

# 환경변수
export VEILKEY_LOCALVAULT_URL="https://127.0.0.1:${LOCALVAULT_PORT}"
export VEILKEY_KEYCENTER_URL="https://127.0.0.1:${KEYCENTER_PORT}"
export VEILKEY_API="\${VEILKEY_LOCALVAULT_URL}"

# alias
alias vk='veilkey'
alias vks='veilkey scan'
alias vkf='veilkey filter'
alias vkw='veilkey wrap'
alias vke='veilkey exec'
alias vkr='veilkey resolve'
alias vkl='veilkey list'
alias vkst='veilkey status'

# 서비스 관리
alias vk-start='launchctl load ~/Library/LaunchAgents/net.veilkey.keycenter.plist ~/Library/LaunchAgents/net.veilkey.localvault.plist 2>/dev/null'
alias vk-stop='launchctl unload ~/Library/LaunchAgents/net.veilkey.localvault.plist ~/Library/LaunchAgents/net.veilkey.keycenter.plist 2>/dev/null'
alias vk-status='echo "--- KeyCenter ---"; curl -sfk https://127.0.0.1:${KEYCENTER_PORT}/health 2>/dev/null || echo "down"; echo "--- LocalVault ---"; curl -sfk https://127.0.0.1:${LOCALVAULT_PORT}/health 2>/dev/null || echo "down"'
alias vk-logs='tail -30 /usr/local/var/log/veilkey/keycenter.log /usr/local/var/log/veilkey/localvault.log 2>/dev/null'
PROFILE

  ok "  ${profile_file}"

  local rc_file
  if [[ "${SHELL}" == *zsh* ]]; then
    rc_file="${HOME}/.zshrc"
  else
    rc_file="${HOME}/.bashrc"
  fi

  if [[ -f "${rc_file}" ]] && grep -q 'source.*\.veilkey\.sh' "${rc_file}" 2>/dev/null; then
    warn "  ${rc_file} (이미 등록됨, 건너뜀)"
  else
    echo '' >> "${rc_file}"
    echo '# VeilKey' >> "${rc_file}"
    echo '[ -f "$HOME/.veilkey.sh" ] && source "$HOME/.veilkey.sh"' >> "${rc_file}"
    ok "  ${rc_file} 에 source 라인 추가"
  fi
}

write_launchd_plists() {
  info "launchd plist 생성..."

  cat > "${LAUNCHD_DIR}/net.veilkey.keycenter.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>net.veilkey.keycenter</string>
  <key>ProgramArguments</key><array><string>${VEILKEY_BIN}/veilkey-keycenter</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>VEILKEY_PASSWORD_FILE</key><string>${VEILKEY_ETC}/keycenter.password</string>
    <key>VEILKEY_ADDR</key><string>:${KEYCENTER_PORT}</string>
    <key>VEILKEY_DB_PATH</key><string>${VEILKEY_DATA}/keycenter/veilkey.db</string>
  </dict>
  <key>WorkingDirectory</key><string>${VEILKEY_DATA}/keycenter</string>
  <key>RunAtLoad</key><false/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>${VEILKEY_LOG}/keycenter.log</string>
  <key>StandardErrorPath</key><string>${VEILKEY_LOG}/keycenter.err.log</string>
</dict>
</plist>
EOF
  ok "  ${LAUNCHD_DIR}/net.veilkey.keycenter.plist"

  cat > "${LAUNCHD_DIR}/net.veilkey.localvault.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>net.veilkey.localvault</string>
  <key>ProgramArguments</key><array><string>${VEILKEY_BIN}/veilkey-localvault</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>VEILKEY_PASSWORD_FILE</key><string>${VEILKEY_ETC}/localvault.password</string>
    <key>VEILKEY_ADDR</key><string>:${LOCALVAULT_PORT}</string>
    <key>VEILKEY_DB_PATH</key><string>${VEILKEY_DATA}/localvault/veilkey.db</string>
    <key>VEILKEY_KEYCENTER_URL</key><string>https://127.0.0.1:${KEYCENTER_PORT}</string>
  </dict>
  <key>WorkingDirectory</key><string>${VEILKEY_DATA}/localvault</string>
  <key>RunAtLoad</key><false/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>${VEILKEY_LOG}/localvault.log</string>
  <key>StandardErrorPath</key><string>${VEILKEY_LOG}/localvault.err.log</string>
</dict>
</plist>
EOF
  ok "  ${LAUNCHD_DIR}/net.veilkey.localvault.plist"
}

# ── 서비스 제어 ───────────────────────────────────────────────────

cmd_start() {
  info "=== VeilKey 서비스 시작 ==="
  launchctl load "${LAUNCHD_DIR}/net.veilkey.keycenter.plist" 2>/dev/null || true
  ok "KeyCenter 시작됨 (포트 ${KEYCENTER_PORT})"
  sleep 2
  launchctl load "${LAUNCHD_DIR}/net.veilkey.localvault.plist" 2>/dev/null || true
  ok "LocalVault 시작됨 (포트 ${LOCALVAULT_PORT})"
}

cmd_stop() {
  info "=== VeilKey 서비스 중지 ==="
  launchctl unload "${LAUNCHD_DIR}/net.veilkey.localvault.plist" 2>/dev/null || true
  ok "LocalVault 중지됨"
  launchctl unload "${LAUNCHD_DIR}/net.veilkey.keycenter.plist" 2>/dev/null || true
  ok "KeyCenter 중지됨"
}

cmd_restart() { cmd_stop; sleep 1; cmd_start; }

cmd_status() {
  echo ""
  echo "=== VeilKey 서비스 상태 ==="
  echo ""
  for svc in keycenter localvault; do
    local label="net.veilkey.${svc}"
    local pid
    pid="$(launchctl list 2>/dev/null | grep "${label}" | awk '{print $1}')" || true
    if [[ -n "${pid}" && "${pid}" != "-" ]]; then
      echo -e "  ${GREEN}●${NC} ${svc}: 실행 중 (PID ${pid})"
    else
      echo -e "  ${RED}●${NC} ${svc}: 중지됨"
    fi
  done
  echo ""
}

cmd_health() {
  echo ""
  echo "=== VeilKey 헬스 체크 ==="
  echo ""
  for pair in "KeyCenter:${KEYCENTER_PORT}" "LocalVault:${LOCALVAULT_PORT}"; do
    local name="${pair%%:*}" port="${pair##*:}"
    if curl -sfk "https://127.0.0.1:${port}/health" >/dev/null 2>&1 || curl -sf "http://127.0.0.1:${port}/health" >/dev/null 2>&1; then
      echo -e "  ${GREEN}✓${NC} ${name} (:${port})"
    else
      echo -e "  ${RED}✗${NC} ${name} (:${port})"
    fi
  done
  echo ""
}

cmd_logs() {
  local service="${1:-all}"
  if [[ "${service}" == "all" ]]; then
    for svc in keycenter localvault; do
      info "=== ${svc} 로그 (최근 20줄) ==="
      tail -20 "${VEILKEY_LOG}/${svc}.log" 2>/dev/null || echo "  (로그 없음)"
      tail -5 "${VEILKEY_LOG}/${svc}.err.log" 2>/dev/null || true
      echo ""
    done
  else
    tail -50 "${VEILKEY_LOG}/${service}.log" 2>/dev/null || echo "  (로그 없음)"
    tail -10 "${VEILKEY_LOG}/${service}.err.log" 2>/dev/null || true
  fi
}

cmd_uninstall() {
  warn "=== VeilKey 전체 제거 ==="
  echo ""
  echo "제거 대상: 바이너리, 설정, 데이터, 로그, launchd, 셸 프로파일"
  read -rp "정말 제거하시겠습니까? (yes/no): " confirm
  [[ "${confirm}" == "yes" ]] || { info "취소됨"; exit 0; }

  cmd_stop 2>/dev/null || true
  rm -f "${LAUNCHD_DIR}/net.veilkey.keycenter.plist" "${LAUNCHD_DIR}/net.veilkey.localvault.plist"
  for bin in veilkey-keycenter veilkey-localvault veilkey-proxy veilkey-session-config veilkey veilkey-veilroot-curl veilkey-veilroot-wget veilkey-veilroot-http; do
    sudo rm -f "${VEILKEY_BIN}/${bin}"
  done
  sudo rm -rf "${VEILKEY_ETC}" "${VEILKEY_DATA}" "${VEILKEY_LOG}" "/usr/local/lib/veilkey"
  rm -rf "${BUILD_DIR}"
  rm -f "${HOME}/.veilkey.sh"
  for rc in "${HOME}/.zshrc" "${HOME}/.bashrc"; do
    [[ -f "${rc}" ]] && sed -i '' '/# VeilKey/d;/\.veilkey\.sh/d' "${rc}" 2>/dev/null || true
  done
  ok "=== VeilKey 제거 완료 ==="
}

cmd_install() {
  echo ""
  echo "╔══════════════════════════════════════╗"
  echo "║    VeilKey Mac Installer             ║"
  echo "║    KeyCenter + LocalVault + CLI      ║"
  echo "╚══════════════════════════════════════╝"
  echo ""
  cmd_build
  echo ""
  cmd_deploy
  echo ""
  cmd_start
  sleep 3
  cmd_health
  echo ""
  echo "  상태:     $0 status"
  echo "  로그:     $0 logs"
  echo "  제거:     $0 uninstall"
  echo ""
  warn "새 터미널을 열거나 'source ~/.zshrc' 하면 vk, vks 등 alias를 쓸 수 있습니다."
}

# ── 메인 ──────────────────────────────────────────────────────────

cmd="${1:-help}"
shift || true

case "${cmd}" in
  install)   cmd_install ;;
  build)     cmd_build ;;
  deploy)    cmd_deploy ;;
  start)     cmd_start ;;
  stop)      cmd_stop ;;
  restart)   cmd_restart ;;
  status)    cmd_status ;;
  health)    cmd_health ;;
  logs)      cmd_logs "${1:-all}" ;;
  uninstall) cmd_uninstall ;;
  help|-h|--help)
    echo "사용법: $0 <command>"
    echo ""
    echo "  install     전체 설치 (Go + 빌드 + 배치 + launchd)"
    echo "  build       소스에서 바이너리 빌드만"
    echo "  deploy      빌드된 바이너리 배치 + 설정 + launchd"
    echo "  start       서비스 시작"
    echo "  stop        서비스 중지"
    echo "  restart     서비스 재시작"
    echo "  status      서비스 상태"
    echo "  health      헬스 체크"
    echo "  logs [svc]  로그 (keycenter|localvault|all)"
    echo "  uninstall   전체 제거"
    ;;
  *) die "알 수 없는 명령: ${cmd}" ;;
esac
