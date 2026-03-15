#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

stage() {
  printf '[lxc-allinone/install] %s\n' "$*"
}

usage() {
  cat <<'EOF'
Usage: ./scripts/proxmox-lxc-allinone-install.sh [--activate] [--health] [root] [bundle_root]

Install the Proxmox LXC all-in-one profile:
  proxmox-lxc-allinone = keycenter + localvault + proxy
EOF
}

ensure_manifest() {
  if [[ -f "${ROOT_DIR}/components.toml" ]]; then
    stage "using existing manifest ${ROOT_DIR}/components.toml"
    return 0
  fi
  stage "bootstrapping ${ROOT_DIR}/components.toml from canonical example"
  cp "${ROOT_DIR}/components.toml.example" "${ROOT_DIR}/components.toml"
}

ensure_bootstrap_tools() {
  local missing=0
  command -v ssh-keygen >/dev/null 2>&1 || missing=1
  command -v openssl >/dev/null 2>&1 || missing=1
  if [[ "${missing}" = "0" ]]; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    stage "installing bootstrap tools (openssh-client openssl)"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y openssh-client openssl >/dev/null
  fi
  command -v ssh-keygen >/dev/null 2>&1 || {
    echo "Error: ssh-keygen is required for all-in-one bootstrap" >&2
    exit 1
  }
  command -v openssl >/dev/null 2>&1 || {
    echo "Error: openssl is required for all-in-one bootstrap" >&2
    exit 1
  }
}

resolve_passwords() {
  if [[ -z "${VEILKEY_KEYCENTER_PASSWORD:-}" || -z "${VEILKEY_LOCALVAULT_PASSWORD:-}" ]]; then
    if [[ -f /opt/veilkey/data/password ]]; then
      stage "loading missing passwords from /opt/veilkey/data/password"
      # shellcheck disable=SC1091
      . /opt/veilkey/data/password
    fi
  fi
  export VEILKEY_KEYCENTER_PASSWORD="${VEILKEY_KEYCENTER_PASSWORD:-${VEILKEY_PASSWORD:-}}"
  export VEILKEY_LOCALVAULT_PASSWORD="${VEILKEY_LOCALVAULT_PASSWORD:-${VEILKEY_PASSWORD:-}}"
  [[ -n "${VEILKEY_KEYCENTER_PASSWORD:-}" ]] || {
    echo "Error: VEILKEY_KEYCENTER_PASSWORD is required" >&2
    exit 1
  }
  [[ -n "${VEILKEY_LOCALVAULT_PASSWORD:-}" ]] || {
    echo "Error: VEILKEY_LOCALVAULT_PASSWORD is required" >&2
    exit 1
  }
}

set_default_env() {
  export VEILKEY_KEYCENTER_ADDR="${VEILKEY_KEYCENTER_ADDR:-:10181}"
  export VEILKEY_LOCALVAULT_ADDR="${VEILKEY_LOCALVAULT_ADDR:-:10180}"
  export VEILKEY_KEYCENTER_URL="${VEILKEY_KEYCENTER_URL:-http://127.0.0.1:10181}"
  export VEILKEY_LOCALVAULT_DB_PATH="${VEILKEY_LOCALVAULT_DB_PATH:-/opt/veilkey/localvault/data/veilkey.db}"
  export VEILKEY_KEYCENTER_DB_PATH="${VEILKEY_KEYCENTER_DB_PATH:-/opt/veilkey/keycenter/data/veilkey.db}"
}

init_keycenter_if_needed() {
  local root="$1"
  local db_path="${VEILKEY_KEYCENTER_DB_PATH}"
  if [[ "${root}" != "/" ]]; then
    stage "skipping KeyCenter init for non-live root ${root}"
    return 0
  fi
  if [[ -f "${db_path}" ]]; then
    stage "existing KeyCenter DB found at ${db_path}; init skipped"
    return 0
  fi
  mkdir -p "$(dirname "${db_path}")"
  stage "initializing KeyCenter root node at ${db_path}"
  echo "${VEILKEY_KEYCENTER_PASSWORD}" | VEILKEY_DB_PATH="${db_path}" /usr/local/bin/veilkey-keycenter init --root
}

init_localvault_if_needed() {
  local root="$1"
  local db_path="${VEILKEY_LOCALVAULT_DB_PATH}"
  if [[ "${root}" != "/" ]]; then
    stage "skipping LocalVault init for non-live root ${root}"
    return 0
  fi
  if [[ -f "${db_path}" ]]; then
    stage "existing LocalVault DB found at ${db_path}; init skipped"
    return 0
  fi
  mkdir -p "$(dirname "${db_path}")"
  stage "initializing LocalVault root node at ${db_path}"
  echo "${VEILKEY_LOCALVAULT_PASSWORD}" | VEILKEY_DB_PATH="${db_path}" /usr/local/bin/veilkey-localvault init --root
}

ensure_bootstrap_ssh_material() {
  local ssh_dir key_name priv pub enc passphrase target_root
  target_root="${root%/}"
  ssh_dir="${VEILKEY_BOOTSTRAP_SSH_DIR:-/etc/veilkey/bootstrap/ssh}"
  if [[ "${target_root}" != "/" && -n "${target_root}" ]]; then
    ssh_dir="${target_root}${ssh_dir}"
  fi
  key_name="${VEILKEY_BOOTSTRAP_SSH_KEY_NAME:-veilkey-admin}"
  priv="${ssh_dir}/${key_name}"
  pub="${priv}.pub"
  enc="${priv}.enc"
  passphrase="${VEILKEY_BOOTSTRAP_ENCRYPTION_PASSWORD:-${VEILKEY_KEYCENTER_PASSWORD}}"
  mkdir -p "${ssh_dir}"
  chmod 700 "${ssh_dir}"
  if [[ ! -f "${priv}" ]]; then
    stage "generating bootstrap SSH key at ${priv}"
    ssh-keygen -t ed25519 -N '' -f "${priv}" -C "${HOSTNAME:-veilkey-allinone}" >/dev/null
    chmod 600 "${priv}"
    chmod 644 "${pub}"
  else
    stage "existing bootstrap SSH key found at ${priv}"
  fi
  if [[ ! -f "${enc}" ]]; then
    stage "writing encrypted bootstrap SSH key at ${enc}"
    openssl enc -aes-256-cbc -pbkdf2 -salt -in "${priv}" -out "${enc}" -pass "pass:${passphrase}"
    chmod 600 "${enc}"
  else
    stage "existing encrypted bootstrap SSH key found at ${enc}"
  fi
}

if [[ "${1:-}" =~ ^(-h|--help)$ ]]; then
  usage
  exit 0
fi

args=()
activate_requested=0
while [[ $# -gt 0 && "${1:-}" == --* ]]; do
  args+=("$1")
  [[ "${1}" == "--activate" ]] && activate_requested=1
  shift
done

root="${1:-/}"
bundle_root="${2:-}"

stage "target root: ${root}"
ensure_manifest
ensure_bootstrap_tools
resolve_passwords
set_default_env
stage "keycenter addr: ${VEILKEY_KEYCENTER_ADDR}"
stage "localvault addr: ${VEILKEY_LOCALVAULT_ADDR}"
stage "keycenter url: ${VEILKEY_KEYCENTER_URL}"
stage "stage: install-profile"
if [[ -n "${bundle_root}" ]]; then
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-lxc-allinone "${root}" "${bundle_root}"
else
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-lxc-allinone "${root}"
fi
stage "stage: bootstrap-ssh"
ensure_bootstrap_ssh_material
stage "stage: init"
init_keycenter_if_needed "${root}"
init_localvault_if_needed "${root}"
if [[ "${activate_requested}" = "1" ]]; then
  stage "stage: reactivate"
  "${ROOT_DIR}/install.sh" activate "${root}"
fi
stage "completed"
