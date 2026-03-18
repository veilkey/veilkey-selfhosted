#!/usr/bin/env bash
# VeilKey VaultCenter — 통합 제거 스크립트
# Docker / Shell 설치 자동 감지 후 제거
set -euo pipefail

SERVICE_NAME="veilkey-vaultcenter"
BINARY_PATH="/usr/local/bin/veilkey-vaultcenter"
DOCKER_DIR="${VEILKEY_INSTALL_DIR:-/opt/veilkey-vaultcenter}"
DATA_DIR="${VEILKEY_DATA_DIR:-/opt/veilkey-vaultcenter/data}"

# --- Colors ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

info()  { echo -e "${GREEN}[+]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*"; exit 1; }

# --- Root check ---
[[ $EUID -eq 0 ]] || error "root 권한이 필요합니다. sudo bash uninstall.sh"

# --- Detect install type ---
HAS_DOCKER=false
HAS_SHELL=false

if [[ -f "$DOCKER_DIR/docker-compose.yml" ]]; then
    HAS_DOCKER=true
fi
if [[ -f "$BINARY_PATH" ]] && systemctl list-unit-files "${SERVICE_NAME}.service" >/dev/null 2>&1; then
    HAS_SHELL=true
fi

if ! $HAS_DOCKER && ! $HAS_SHELL; then
    # 추가 감지: 컨테이너가 실행 중인지 확인
    if command -v docker >/dev/null 2>&1 && docker ps -a --format '{{.Names}}' | grep -q "^veilkey-vaultcenter$"; then
        HAS_DOCKER=true
    fi
    if [[ -f "$BINARY_PATH" ]]; then
        HAS_SHELL=true
    fi
fi

if ! $HAS_DOCKER && ! $HAS_SHELL; then
    error "VeilKey VaultCenter 설치를 찾을 수 없습니다."
fi

echo ""
echo "  ╔══════════════════════════════════════════════════════╗"
echo "  ║           VeilKey VaultCenter — Uninstall                   ║"
echo "  ╚══════════════════════════════════════════════════════╝"
echo ""

if $HAS_DOCKER; then
    info "감지: Docker 설치 ($DOCKER_DIR)"
fi
if $HAS_SHELL; then
    info "감지: Shell 설치 ($BINARY_PATH)"
fi
echo ""

# --- Confirmation ---
if [[ "${NONINTERACTIVE:-}" != "1" ]]; then
    read -rp "  VeilKey VaultCenter를 제거하시겠습니까? [y/N]: " confirm
    [[ "$confirm" =~ ^[yY]$ ]] || { echo "취소됨."; exit 0; }
fi

# --- Remove Docker install ---
if $HAS_DOCKER; then
    info "Docker 설치 제거..."
    if command -v docker >/dev/null 2>&1; then
        cd "$DOCKER_DIR" 2>/dev/null || true
        if [[ -f "$DOCKER_DIR/docker-compose.yml" ]]; then
            docker compose -f "$DOCKER_DIR/docker-compose.yml" down 2>/dev/null || true
        fi
        # 볼륨 삭제 확인
        if [[ "${NONINTERACTIVE:-}" != "1" ]]; then
            read -rp "  Docker 볼륨(데이터)도 삭제하시겠습니까? [y/N]: " del_volume
            if [[ "$del_volume" =~ ^[yY]$ ]]; then
                docker volume rm veilkey-data 2>/dev/null || true
                docker volume rm "${SERVICE_NAME}_veilkey-data" 2>/dev/null || true
                info "Docker 볼륨 삭제됨"
            fi
        fi
    fi
    # compose 파일 및 .env 제거
    rm -f "$DOCKER_DIR/docker-compose.yml"
    rm -f "$DOCKER_DIR/.env"
    info "Docker 설치 제거 완료"
fi

# --- Remove Shell install ---
if $HAS_SHELL; then
    info "Shell 설치 제거..."
    # systemd 서비스 중지/비활성화
    if systemctl is-active "$SERVICE_NAME" >/dev/null 2>&1; then
        systemctl stop "$SERVICE_NAME"
        info "서비스 중지됨"
    fi
    if systemctl is-enabled "$SERVICE_NAME" >/dev/null 2>&1; then
        systemctl disable "$SERVICE_NAME" >/dev/null 2>&1
    fi
    # 서비스 파일 삭제
    rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
    systemctl daemon-reload
    # 바이너리 삭제
    rm -f "$BINARY_PATH"
    info "Shell 설치 제거 완료"
fi

# --- Data directory ---
if [[ -d "$DATA_DIR" ]]; then
    if [[ "${NONINTERACTIVE:-}" != "1" ]]; then
        echo ""
        warn "데이터 디렉토리가 존재합니다: $DATA_DIR"
        read -rp "  데이터를 삭제하시겠습니까? (DB, 키 포함) [y/N]: " del_data
        if [[ "$del_data" =~ ^[yY]$ ]]; then
            rm -rf "$DATA_DIR"
            info "데이터 디렉토리 삭제됨"
        else
            info "데이터 보존됨: $DATA_DIR"
        fi
    else
        info "데이터 보존됨 (자동 모드): $DATA_DIR"
    fi
fi

# 빈 설치 디렉토리 정리
if [[ -d "$DOCKER_DIR" ]] && [[ -z "$(ls -A "$DOCKER_DIR" 2>/dev/null)" ]]; then
    rmdir "$DOCKER_DIR" 2>/dev/null || true
fi

echo ""
info "VeilKey VaultCenter 제거 완료!"
