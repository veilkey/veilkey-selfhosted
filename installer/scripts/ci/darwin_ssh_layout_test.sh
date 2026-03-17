#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROFILE="${1:-proxmox-lxc-runtime}"
REMOTE_HOST="${DARWIN_TEST_HOST:?DARWIN_TEST_HOST required}"
REMOTE_USER="${DARWIN_TEST_USER:-$USER}"
REMOTE_PORT="${DARWIN_TEST_PORT:-22}"
REMOTE_WORKDIR="${DARWIN_TEST_REMOTE_DIR:-/tmp/veilkey-installer-darwin}"
LOCAL_BUNDLE_DIR="${VEILKEY_E2E_BUNDLE_DIR:-${ROOT_DIR}/.tmp/darwin-bundle-${PROFILE}}"
LOCAL_SRC_TARBALL="${VEILKEY_E2E_SRC_TARBALL:-${ROOT_DIR}/.tmp/veilkey-installer-darwin-src-${PROFILE}.tar.gz}"

resolve_package_pat() {
  local candidate
  local hosts=()
  [[ -n "${VEILKEY_MIRROR_IP:-}" ]] && hosts+=("protocol=http\nhost=${VEILKEY_MIRROR_IP}\n\n")
  [[ -n "${VEILKEY_GITLAB_HOST:-}" ]] && hosts+=("protocol=https\nhost=${VEILKEY_GITLAB_HOST}\n\n")
  [[ -n "${VEILKEY_VAULT_IP:-}" ]] && hosts+=("protocol=http\nhost=${VEILKEY_VAULT_IP}\n\n")
  for candidate in \
    "${hosts[@]}"
  do
    if printf '%b' "${candidate}" | git credential fill 2>/dev/null | awk -F= '/^password=/{print $2; found=1; exit} END{exit(found?0:1)}'; then
      return 0
    fi
  done
  return 1
}

cleanup() {
  ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "${REMOTE_PORT}" \
    "${REMOTE_USER}@${REMOTE_HOST}" "rm -rf '${REMOTE_WORKDIR}'" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "${ROOT_DIR}/.tmp"
"${ROOT_DIR}/install.sh" init
export VEILKEY_GITLAB_PACKAGE_PAT="${VEILKEY_GITLAB_PACKAGE_PAT:-$(resolve_package_pat || true)}"
"${ROOT_DIR}/install.sh" bundle "${PROFILE}" "${LOCAL_BUNDLE_DIR}"
tar -C "${ROOT_DIR}" -czf "${LOCAL_SRC_TARBALL}" \
  install.sh components.toml components.toml.example README.md docs scripts profiles

ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "${REMOTE_PORT}" \
  "${REMOTE_USER}@${REMOTE_HOST}" "rm -rf '${REMOTE_WORKDIR}' && mkdir -p '${REMOTE_WORKDIR}'"
scp -q -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -P "${REMOTE_PORT}" \
  "${LOCAL_SRC_TARBALL}" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_WORKDIR}/installer-src.tgz"
scp -q -r -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -P "${REMOTE_PORT}" \
  "${LOCAL_BUNDLE_DIR}" "${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_WORKDIR}/bundle"

ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -p "${REMOTE_PORT}" \
  "${REMOTE_USER}@${REMOTE_HOST}" "set -euo pipefail
    cd '${REMOTE_WORKDIR}'
    mkdir -p src rootfs
    tar -xzf installer-src.tgz -C src
    cd src
    export VEILKEY_INSTALLER_OS_FAMILY='darwin'
    export VEILKEY_KEYCENTER_PASSWORD='e2e-keycenter'
    export VEILKEY_LOCALVAULT_PASSWORD='e2e-localvault'
    export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
    ./install.sh detect-os
    ./install.sh install '${PROFILE}' '${REMOTE_WORKDIR}/rootfs' '${REMOTE_WORKDIR}/bundle'
    ./install.sh configure '${PROFILE}' '${REMOTE_WORKDIR}/rootfs'
    ./install.sh plan-activate '${REMOTE_WORKDIR}/rootfs'
    ./install.sh post-install-health '${REMOTE_WORKDIR}/rootfs'
  "

echo "Darwin SSH layout test passed for ${PROFILE} on ${REMOTE_HOST}"
