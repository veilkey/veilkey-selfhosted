#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT="${VEILKEY_UPDATE_REPO_ROOT:-/opt/veilkey-selfhosted-repo}"
REMOTE_NAME="${VEILKEY_UPDATE_REMOTE:-origin}"
BRANCH_NAME="${VEILKEY_UPDATE_BRANCH:-main}"
SERVICE_NAME="${VEILKEY_UPDATE_SERVICE_NAME:-veilkey-keycenter.service}"
HEALTH_URL="${VEILKEY_UPDATE_HEALTH_URL:-http://127.0.0.1:10181/health}"
INSTALL_BIN="${VEILKEY_UPDATE_INSTALL_BIN:-/usr/local/bin/veilkey-keycenter}"
BUILD_WORKDIR="${REPO_ROOT}/services/keycenter"
TARGET_VERSION="${VEILKEY_UPDATE_TARGET_VERSION:-}"
CURRENT_VERSION="${VEILKEY_UPDATE_CURRENT_VERSION:-}"
RELEASE_CHANNEL="${VEILKEY_UPDATE_RELEASE_CHANNEL:-stable}"

log() {
  printf '[keycenter-update] %s\n' "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    log "missing required command: $1"
    exit 1
  }
}

require_clean_repo() {
  if [[ -n "$(git -C "${REPO_ROOT}" status --short)" ]]; then
    log "repository has local changes: ${REPO_ROOT}"
    exit 1
  fi
}

check_target_version() {
  if [[ -z "${TARGET_VERSION}" ]]; then
    log "VEILKEY_UPDATE_TARGET_VERSION is required"
    exit 1
  fi
  if [[ "${CURRENT_VERSION}" == "${TARGET_VERSION}" ]]; then
    log "current version already matches target version (${TARGET_VERSION}); nothing to do"
    exit 0
  fi
}

update_repo() {
  require_clean_repo
  git -C "${REPO_ROOT}" fetch "${REMOTE_NAME}" "${BRANCH_NAME}"
  git -C "${REPO_ROOT}" checkout "${BRANCH_NAME}"
  git -C "${REPO_ROOT}" merge --ff-only "${REMOTE_NAME}/${BRANCH_NAME}"
}

verify_version_file() {
  local version_file="${REPO_ROOT}/VERSION"
  if [[ ! -f "${version_file}" ]]; then
    log "VERSION file not found: ${version_file}"
    exit 1
  fi
  local repo_version
  repo_version="$(tr -d '[:space:]' < "${version_file}")"
  if [[ "${repo_version}" != "${TARGET_VERSION}" ]]; then
    log "repo version ${repo_version} does not match requested target ${TARGET_VERSION}"
    exit 1
  fi
}

build_and_install() {
  require_cmd go
  install -d "$(dirname "${INSTALL_BIN}")"
  (
    cd "${BUILD_WORKDIR}"
    CGO_ENABLED=1 go build -ldflags="-s -w" -o "${INSTALL_BIN}" ./cmd/main.go
  )
  chmod 0755 "${INSTALL_BIN}"
}

restart_service() {
  require_cmd systemctl
  systemctl restart "${SERVICE_NAME}"
}

check_health() {
  require_cmd curl
  local attempts=30
  local sleep_seconds=2
  local i
  for ((i=1; i<=attempts; i++)); do
    if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      return 0
    fi
    sleep "${sleep_seconds}"
  done
  log "health check failed after restart: ${HEALTH_URL}"
  exit 1
}

main() {
  log "starting update"
  log "repo=${REPO_ROOT} remote=${REMOTE_NAME} branch=${BRANCH_NAME} channel=${RELEASE_CHANNEL}"
  check_target_version
  update_repo
  verify_version_file
  build_and_install
  restart_service
  check_health
  log "update completed target=${TARGET_VERSION}"
}

main "$@"
