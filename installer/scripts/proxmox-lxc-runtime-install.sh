#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
activate_after_install=0

stage() {
  printf '[lxc-runtime/install] %s\n' "$*"
}

ensure_runtime_tools() {
  command -v curl >/dev/null 2>&1 && return 0
  if command -v apt-get >/dev/null 2>&1; then
    stage "installing runtime health dependency (curl)"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y curl >/dev/null
  fi
  command -v curl >/dev/null 2>&1 || {
    echo "Error: curl is required for runtime install health checks" >&2
    exit 1
  }
}

usage() {
  cat <<'EOF'
Usage: ./scripts/proxmox-lxc-runtime-install.sh [--activate] [--health] [root] [bundle_root]

Install the Proxmox LXC runtime profile:
  proxmox-lxc-runtime = localvault
EOF
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

if [[ "${1:-}" =~ ^(-h|--help)$ ]]; then
  usage
  exit 0
fi

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

ensure_runtime_tools
resolve_localvault_password

if [[ -n "${bundle_root}" ]]; then
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-lxc-runtime "${root}" "${bundle_root}"
else
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-lxc-runtime "${root}"
fi
init_localvault_if_needed "${root}"

if [[ "${activate_after_install}" = "1" ]]; then
  stage "stage: reactivate"
  "${ROOT_DIR}/install.sh" activate "${root}"
fi

stage "completed"
