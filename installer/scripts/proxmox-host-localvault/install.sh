#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
activate_after_install=0

stage() {
  printf '[host-localvault/install] %s\n' "$*"
}

ensure_manifest() {
  if [[ -f "${ROOT_DIR}/components.toml" ]]; then
    stage "using existing manifest ${ROOT_DIR}/components.toml"
    return 0
  fi
  stage "bootstrapping ${ROOT_DIR}/components.toml from canonical example"
  "${ROOT_DIR}/install.sh" init >/dev/null
}

resolve_localvault_password() {
  if [[ -n "${VEILKEY_LOCALVAULT_PASSWORD:-}" ]]; then
    stage "using VEILKEY_LOCALVAULT_PASSWORD from environment"
    return 0
  fi
  if [[ -f /opt/veilkey/data/password ]]; then
    stage "loading password from /opt/veilkey/data/password"
    # shellcheck disable=SC1091
    . /opt/veilkey/data/password
    export VEILKEY_LOCALVAULT_PASSWORD="${VEILKEY_PASSWORD:-}"
  fi
  [[ -n "${VEILKEY_LOCALVAULT_PASSWORD:-}" ]] || {
    echo "Error: VEILKEY_LOCALVAULT_PASSWORD is required" >&2
    exit 1
  }
}

resolve_keycenter_url() {
  if [[ -n "${VEILKEY_KEYCENTER_URL:-}" ]]; then
    stage "using VEILKEY_KEYCENTER_URL from environment: ${VEILKEY_KEYCENTER_URL}"
    return 0
  fi
  if curl -kfsS https://keycenter.60.internal.kr/health >/dev/null 2>&1; then
    export VEILKEY_KEYCENTER_URL="https://keycenter.60.internal.kr"
    stage "auto-detected KeyCenter at ${VEILKEY_KEYCENTER_URL}"
    return 0
  fi
  echo "Error: VEILKEY_KEYCENTER_URL is required" >&2
  exit 1
}

init_localvault_if_needed() {
  local root="$1"
  local db_path="${VEILKEY_LOCALVAULT_DB_PATH:-/opt/veilkey/localvault/data/veilkey.db}"

  if [[ "${root}" != "/" ]]; then
    stage "skipping init for non-live root ${root}"
    return 0
  fi
  if [[ -f "${db_path}" ]]; then
    stage "existing DB found at ${db_path}; init skipped"
    return 0
  fi

  stage "initializing LocalVault root node at ${db_path}"
  mkdir -p "$(dirname "${db_path}")"
  echo "${VEILKEY_LOCALVAULT_PASSWORD}" | VEILKEY_DB_PATH="${db_path}" /usr/local/bin/veilkey-localvault init --root
}

args=()
while [[ $# -gt 0 && "${1:-}" == --* ]]; do
  case "${1}" in
    --activate)
      activate_after_install=1
      ;;
    *)
      args+=("$1")
      ;;
  esac
  shift
done

root="${1:-/}"
bundle_root="${2:-}"

export VEILKEY_LOCALVAULT_ADDR="${VEILKEY_LOCALVAULT_ADDR:-0.0.0.0:10180}"
export VEILKEY_LOCALVAULT_DB_PATH="${VEILKEY_LOCALVAULT_DB_PATH:-/opt/veilkey/localvault/data/veilkey.db}"
export VEILKEY_LOCALVAULT_TRUSTED_IPS="${VEILKEY_LOCALVAULT_TRUSTED_IPS:-10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1}"

stage "target root: ${root}"
stage "listen addr: ${VEILKEY_LOCALVAULT_ADDR}"
stage "db path: ${VEILKEY_LOCALVAULT_DB_PATH}"

ensure_manifest
resolve_localvault_password
resolve_keycenter_url

stage "stage: install-profile"
if [[ -n "${bundle_root}" ]]; then
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-host-localvault "${root}" "${bundle_root}"
else
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-host-localvault "${root}"
fi

stage "stage: init"
init_localvault_if_needed "${root}"

if [[ "${activate_after_install}" = "1" ]]; then
  stage "stage: activate"
  "${ROOT_DIR}/install.sh" activate "${root}"
fi

stage "completed"
