#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
root="${1:-/}"

stage() {
  printf '[lxc-runtime/health] %s\n' "$*"
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Error: required command '$1' not found" >&2
    exit 1
  }
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

stage "verifying scaffold in ${root}"
"${ROOT_DIR}/install.sh" post-install-health "${root}"

if [[ "${root}" != "/" ]]; then
  stage "staged root verification complete"
  exit 0
fi

require_cmd curl
systemctl is-active --quiet veilkey-localvault.service || {
  echo "Error: service veilkey-localvault.service is not active" >&2
  exit 1
}

localvault_addr="$(read_addr_from_env /etc/veilkey/localvault.env :10180)"
localvault_port="${localvault_addr##*:}"
curl -sf "http://127.0.0.1:${localvault_port}/health" >/dev/null || {
  echo "Error: health request failed for localvault on ${localvault_addr}" >&2
  exit 1
}

stage "live verification complete"
