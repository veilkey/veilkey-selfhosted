#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
root="${1:-/}"

stage() {
  printf '[lxc-allinone/health] %s\n' "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Error: required command '$1' not found" >&2
    exit 1
  }
}

addr_to_url() {
  local addr="$1"
  local port="${addr##*:}"
  echo "https://127.0.0.1:${port}"
}

read_addr_from_env() {
  local env_file="$1"
  local default_addr="$2"
  local value
  [[ -f "${env_file}" ]] || {
    echo "${default_addr}"
    return 0
  }
  value="$(awk -F= '$1=="VEILKEY_ADDR" {print $2}' "${env_file}" | tail -n 1)"
  if [[ -n "${value}" ]]; then
    echo "${value}"
  else
    echo "${default_addr}"
  fi
}

check_live_service() {
  local unit="$1"
  systemctl is-active --quiet "${unit}" || {
    echo "Error: service ${unit} is not active" >&2
    exit 1
  }
}

check_live_http() {
  local url="$1"
  curl -sfk "${url}" >/dev/null || {
    echo "Error: health request failed for ${url}" >&2
    exit 1
  }
}

check_bootstrap_files() {
  local ssh_dir="${VEILKEY_BOOTSTRAP_SSH_DIR:-/etc/veilkey/bootstrap/ssh}"
  local key_name="${VEILKEY_BOOTSTRAP_SSH_KEY_NAME:-veilkey-admin}"
  local required=(
    "${ssh_dir}/${key_name}"
    "${ssh_dir}/${key_name}.pub"
    "${ssh_dir}/${key_name}.enc"
  )
  local path
  for path in "${required[@]}"; do
    [[ -f "${path}" ]] || {
      echo "Error: missing bootstrap artifact ${path}" >&2
      exit 1
    }
  done
}

stage "verifying scaffold in ${root}"
"${ROOT_DIR}/install.sh" post-install-health "${root}"

if [[ "${root}" != "/" ]]; then
  stage "staged root verification complete"
  exit 0
fi

require_cmd curl

keycenter_addr="$(read_addr_from_env /etc/veilkey/keycenter.env :10181)"
localvault_addr="$(read_addr_from_env /etc/veilkey/localvault.env :10180)"
keycenter_url="${VEILKEY_KEYCENTER_URL:-$(addr_to_url "${keycenter_addr}")}"
localvault_url="$(addr_to_url "${localvault_addr}")"

stage "checking live services"
check_live_service veilkey-keycenter.service
check_live_service veilkey-localvault.service

stage "checking live health endpoints"
check_live_http "${keycenter_url}/health"
check_live_http "${localvault_url}/health"

stage "checking bootstrap SSH artifacts"
check_bootstrap_files

stage "live verification complete"
