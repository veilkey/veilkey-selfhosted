#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROFILE="${1:-proxmox-lxc-allinone}"
REMOTE_WORKDIR="${VEILKEY_E2E_REMOTE_DIR:-/root/veilkey-installer-e2e}"
LOCAL_BUNDLE_DIR="${VEILKEY_E2E_BUNDLE_DIR:-${ROOT_DIR}/.tmp/e2e-bundle-${PROFILE}}"
LOCAL_SRC_TARBALL="${VEILKEY_E2E_SRC_TARBALL:-${ROOT_DIR}/.tmp/veilkey-installer-src-${PROFILE}.tar.gz}"

: "${PROXMOX_TEMPLATE_VMID:?PROXMOX_TEMPLATE_VMID required}"
: "${PROXMOX_TEST_VMID:?PROXMOX_TEST_VMID required}"
: "${PROXMOX_TEST_IP:?PROXMOX_TEST_IP required}"

PROXMOX_NODE="${PROXMOX_NODE:-$(hostname -s)}"
PROXMOX_TEST_NAME="${PROXMOX_TEST_NAME:-veilkey-installer-e2e-${PROFILE}-${PROXMOX_TEST_VMID}}"
PROXMOX_SSH_USER="${PROXMOX_SSH_USER:-root}"
PROXMOX_TEST_USER="${PROXMOX_TEST_USER:-}"
PROXMOX_SSH_PORT="${PROXMOX_SSH_PORT:-22}"
PROXMOX_TEST_GW="${PROXMOX_TEST_GW:-}"
PROXMOX_SSH_KEY_PATH="${PROXMOX_SSH_KEY_PATH:-/root/.ssh/id_rsa.pub}"
PROXMOX_SSH_PRIVATE_KEY="${PROXMOX_SSH_PRIVATE_KEY:-}"
PROXMOX_CLONE_MODE="${PROXMOX_CLONE_MODE:-linked}"
KEEP_FAILED_VM="${KEEP_FAILED_VM:-0}"
VEILKEY_E2E_ACCOUNT_SMOKE="${VEILKEY_E2E_ACCOUNT_SMOKE:-0}"

resolve_package_pat() {
  local candidate
  for candidate in \
    "protocol=http\nhost=10.60.100.210\n\n" \
    "protocol=https\nhost=gitlab.ranode.net\n\n" \
    "protocol=http\nhost=10.50.100.210\n\n"
  do
    if printf '%b' "${candidate}" | git credential fill 2>/dev/null | awk -F= '/^password=/{print $2; found=1; exit} END{exit(found?0:1)}'; then
      return 0
    fi
  done
  return 1
}

ssh_opts=(-o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -p "${PROXMOX_SSH_PORT}")
scp_opts=(-q -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -P "${PROXMOX_SSH_PORT}")
if [[ -n "${PROXMOX_SSH_PRIVATE_KEY}" ]]; then
  ssh_opts=(-i "${PROXMOX_SSH_PRIVATE_KEY}" "${ssh_opts[@]}")
  scp_opts=(-i "${PROXMOX_SSH_PRIVATE_KEY}" "${scp_opts[@]}")
fi

cleanup() {
  local rc=$?
  if [[ "${KEEP_FAILED_VM}" = "1" && $rc -ne 0 ]]; then
    echo "KEEP_FAILED_VM=1, keeping VM ${PROXMOX_TEST_VMID}" >&2
    exit $rc
  fi
  qm shutdown "${PROXMOX_TEST_VMID}" --timeout 20 >/dev/null 2>&1 || true
  sleep 2
  qm stop "${PROXMOX_TEST_VMID}" >/dev/null 2>&1 || true
  qm destroy "${PROXMOX_TEST_VMID}" --purge >/dev/null 2>&1 || true
  exit $rc
}
trap cleanup EXIT

wait_for_ssh() {
  local host="$1"
  local tries="${2:-60}"
  local i
  for i in $(seq 1 "${tries}"); do
    if ssh "${ssh_opts[@]}" "${PROXMOX_SSH_USER}@${host}" "echo ok" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "SSH did not become ready: ${host}" >&2
  return 1
}

wait_for_ssh_user() {
  local host="$1"
  local user="$2"
  local tries="${3:-60}"
  local i
  for i in $(seq 1 "${tries}"); do
    if ssh "${ssh_opts[@]}" "${user}@${host}" "echo ok" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "SSH did not become ready: ${user}@${host}" >&2
  return 1
}

guest_exec_bash() {
  local command="$1"
  qm guest exec "${PROXMOX_TEST_VMID}" -- bash -lc "${command}"
}

guest_ipv4_from_agent() {
  local output
  output="$(qm guest cmd "${PROXMOX_TEST_VMID}" network-get-interfaces 2>/dev/null || true)"
  if [[ -z "${output}" ]]; then
    return 1
  fi
  python3 - <<'PY' <<<"${output}"
import json
import sys

try:
    data = json.load(sys.stdin)
except Exception:
    sys.exit(1)

for iface in data:
    for addr in iface.get("ip-addresses", []):
        ip = addr.get("ip-address", "")
        if addr.get("ip-address-type") != "ipv4":
            continue
        if ip.startswith("127."):
            continue
        print(ip)
        sys.exit(0)

sys.exit(1)
PY
}

resolve_guest_host() {
  local requested_host="${PROXMOX_TEST_IP%%/*}"
  local detected_host=""
  local i

  for i in $(seq 1 60); do
    detected_host="$(guest_ipv4_from_agent || true)"
    if [[ -n "${detected_host}" ]]; then
      printf '%s\n' "${detected_host}"
      return 0
    fi
    sleep 2
  done

  printf '%s\n' "${requested_host}"
}

ensure_guest_ssh() {
  local host="$1"
  local key_b64=""

  if wait_for_ssh "${host}" 3; then
    return 0
  fi

  if [[ -f "${PROXMOX_SSH_KEY_PATH}" ]]; then
    key_b64="$(base64 -w0 < "${PROXMOX_SSH_KEY_PATH}")"
  fi

  echo "Bootstrapping SSH inside guest ${PROXMOX_TEST_VMID}"
  guest_exec_bash '
    set -e
    if command -v apt-get >/dev/null 2>&1; then
      export DEBIAN_FRONTEND=noninteractive
      apt-get update
      apt-get install -y openssh-server
      systemctl enable --now ssh
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y openssh-server
      systemctl enable --now sshd
    elif command -v yum >/dev/null 2>&1; then
      yum install -y openssh-server
      systemctl enable --now sshd
    else
      echo "Unsupported package manager for SSH bootstrap" >&2
      exit 1
    fi
  ' >/dev/null

  if [[ -n "${key_b64}" ]]; then
    qm guest exec "${PROXMOX_TEST_VMID}" -- bash -lc "
      set -e
      id -u '${PROXMOX_SSH_USER}' >/dev/null 2>&1 || useradd -m -s /bin/bash '${PROXMOX_SSH_USER}'
      install -d -m 700 -o '${PROXMOX_SSH_USER}' -g '${PROXMOX_SSH_USER}' '/home/${PROXMOX_SSH_USER}/.ssh'
      printf '%s' '${key_b64}' | base64 -d > '/home/${PROXMOX_SSH_USER}/.ssh/authorized_keys'
      chown '${PROXMOX_SSH_USER}:${PROXMOX_SSH_USER}' '/home/${PROXMOX_SSH_USER}/.ssh/authorized_keys'
      chmod 600 '/home/${PROXMOX_SSH_USER}/.ssh/authorized_keys'
    " >/dev/null
  fi

  wait_for_ssh "${host}" 60
}

wait_for_vm_unlock() {
  local tries="${1:-60}"
  local i
  for i in $(seq 1 "${tries}"); do
    if ! qm config "${PROXMOX_TEST_VMID}" | grep -q '^lock:'; then
      return 0
    fi
    sleep 2
  done
  echo "VM lock did not clear: ${PROXMOX_TEST_VMID}" >&2
  return 1
}

prepare_bundle() {
  mkdir -p "${ROOT_DIR}/.tmp"
  rm -rf "${LOCAL_BUNDLE_DIR}"
  "${ROOT_DIR}/install.sh" init
  export VEILKEY_GITLAB_PACKAGE_PAT="${VEILKEY_GITLAB_PACKAGE_PAT:-$(resolve_package_pat || true)}"
  "${ROOT_DIR}/install.sh" bundle "${PROFILE}" "${LOCAL_BUNDLE_DIR}"
  tar -C "${ROOT_DIR}" -czf "${LOCAL_SRC_TARBALL}" \
    install.sh components.toml components.toml.example README.md docs scripts profiles
}

provision_vm() {
  qm destroy "${PROXMOX_TEST_VMID}" --purge >/dev/null 2>&1 || true
  if [[ "${PROXMOX_CLONE_MODE}" = "full" ]]; then
    qm clone "${PROXMOX_TEMPLATE_VMID}" "${PROXMOX_TEST_VMID}" --name "${PROXMOX_TEST_NAME}" --full true >/dev/null
  else
    qm clone "${PROXMOX_TEMPLATE_VMID}" "${PROXMOX_TEST_VMID}" --name "${PROXMOX_TEST_NAME}" >/dev/null
  fi
  wait_for_vm_unlock

  if [[ -f "${PROXMOX_SSH_KEY_PATH}" ]]; then
    qm set "${PROXMOX_TEST_VMID}" --ciuser "${PROXMOX_SSH_USER}" --sshkeys "${PROXMOX_SSH_KEY_PATH}" >/dev/null
  fi
  if [[ -n "${PROXMOX_TEST_GW}" ]]; then
    qm set "${PROXMOX_TEST_VMID}" --ipconfig0 "ip=${PROXMOX_TEST_IP},gw=${PROXMOX_TEST_GW}" >/dev/null
  else
    qm set "${PROXMOX_TEST_VMID}" --ipconfig0 "ip=${PROXMOX_TEST_IP}" >/dev/null
  fi
  qm cloudinit update "${PROXMOX_TEST_VMID}" >/dev/null 2>&1 || true
  qm start "${PROXMOX_TEST_VMID}" >/dev/null
}

run_remote_test() {
  local host
  host="$(resolve_guest_host)"
  echo "Using guest host ${host} for VM ${PROXMOX_TEST_VMID}"
  ensure_guest_ssh "${host}"

  ssh "${ssh_opts[@]}" "${PROXMOX_SSH_USER}@${host}" "rm -rf '${REMOTE_WORKDIR}' && mkdir -p '${REMOTE_WORKDIR}'"

  scp "${scp_opts[@]}" "${LOCAL_SRC_TARBALL}" "${PROXMOX_SSH_USER}@${host}:${REMOTE_WORKDIR}/installer-src.tgz"
  scp "${scp_opts[@]}" -r "${LOCAL_BUNDLE_DIR}" "${PROXMOX_SSH_USER}@${host}:${REMOTE_WORKDIR}/bundle"

  ssh "${ssh_opts[@]}" "${PROXMOX_SSH_USER}@${host}" "set -euo pipefail
      cd '${REMOTE_WORKDIR}'
      mkdir -p src
      tar -xzf installer-src.tgz -C src
      cd src
      export VEILKEY_KEYCENTER_PASSWORD='e2e-keycenter'
      export VEILKEY_LOCALVAULT_PASSWORD='e2e-localvault'
      export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
      export VEILKEY_HOSTVAULT_LOCALVAULT_URL='http://127.0.0.1:10180'
      ./install.sh install '${PROFILE}' / '${REMOTE_WORKDIR}/bundle'
      ./install.sh configure '${PROFILE}' /
      ./install.sh plan-activate /
      ./install.sh post-install-health /
    "

  if [[ "${VEILKEY_E2E_ACCOUNT_SMOKE}" = "1" && -n "${PROXMOX_TEST_USER}" ]]; then
    local key_b64
    key_b64="$(base64 -w0 < "${PROXMOX_SSH_KEY_PATH}")"

    ssh "${ssh_opts[@]}" "${PROXMOX_SSH_USER}@${host}" "set -euo pipefail
      id -u '${PROXMOX_TEST_USER}' >/dev/null 2>&1 || useradd -m -s /bin/bash '${PROXMOX_TEST_USER}'
      install -d -m 700 -o '${PROXMOX_TEST_USER}' -g '${PROXMOX_TEST_USER}' '/home/${PROXMOX_TEST_USER}/.ssh'
      printf '%s' '${key_b64}' | base64 -d > '/home/${PROXMOX_TEST_USER}/.ssh/authorized_keys'
      chown '${PROXMOX_TEST_USER}:${PROXMOX_TEST_USER}' '/home/${PROXMOX_TEST_USER}/.ssh/authorized_keys'
      chmod 600 '/home/${PROXMOX_TEST_USER}/.ssh/authorized_keys'
    "

    wait_for_ssh_user "${host}" "${PROXMOX_TEST_USER}" 30

    ssh "${ssh_opts[@]}" "${PROXMOX_TEST_USER}@${host}" "set -euo pipefail
      id
      test -d \"\$HOME\"
      curl -fsS http://127.0.0.1:10180/health >/dev/null || curl -fsS http://127.0.0.1:10180/api/status >/dev/null
    "

    if [[ "${PROFILE}" = "proxmox-lxc-allinone" ]]; then
      ssh "${ssh_opts[@]}" "${PROXMOX_TEST_USER}@${host}" "curl -fsS http://127.0.0.1:10181/health >/dev/null"
    fi
  fi
}

prepare_bundle
provision_vm
run_remote_test
echo "Proxmox VM layout test passed for ${PROFILE} on VM ${PROXMOX_TEST_VMID}"
