#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: ./scripts/proxmox-allinone-stack-install.sh [--activate] [lxc_root] [host_root] [lxc_bundle_root] [host_bundle_root]

Install the Proxmox all-in-one stack in two parts:
  1. proxmox-lxc-allinone inside the LXC runtime root
  2. proxmox-host-cli on the Proxmox host root

This wrapper keeps the runtime contract explicit:
  - the LXC owns KeyCenter + LocalVault
  - the Proxmox host owns the companion boundary/proxy CLI
EOF
}

if [[ "${1:-}" =~ ^(-h|--help)$ ]]; then
  usage
  exit 0
fi

args=()
while [[ $# -gt 0 && "${1:-}" == --* ]]; do
  args+=("$1")
  shift
done

lxc_root="${1:-/}"
host_root="${2:-/}"
lxc_bundle_root="${3:-}"
host_bundle_root="${4:-}"

printf '[proxmox-allinone-stack] install LXC runtime root=%s\n' "$lxc_root"
if [[ -n "${lxc_bundle_root}" ]]; then
  "${ROOT_DIR}/scripts/proxmox-lxc-allinone-install.sh" "${args[@]}" "${lxc_root}" "${lxc_bundle_root}"
else
  "${ROOT_DIR}/scripts/proxmox-lxc-allinone-install.sh" "${args[@]}" "${lxc_root}"
fi

printf '[proxmox-allinone-stack] install host companion root=%s\n' "$host_root"
if [[ -n "${host_bundle_root}" ]]; then
  "${ROOT_DIR}/scripts/proxmox-host-cli-install.sh" "${args[@]}" "${host_root}" "${host_bundle_root}"
else
  "${ROOT_DIR}/scripts/proxmox-host-cli-install.sh" "${args[@]}" "${host_root}"
fi

printf '[proxmox-allinone-stack] completed\n'
