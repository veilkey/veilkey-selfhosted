#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

usage() {
  cat <<'EOF'
Usage: ./scripts/proxmox-host-cli-install.sh [--activate] [--health] [root] [bundle_root]

Install the Proxmox host CLI profile:
  1. proxy assets via install.sh install-profile
  2. veilroot boundary (user + scripts + systemd) — only on live root
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

root="${1:-/}"
bundle_root="${2:-}"

# Step 1: Install proxy assets via manifest engine
printf '[host-cli] step 1/2: proxy assets\n'
if [[ -n "${bundle_root}" ]]; then
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-host-cli "${root}" "${bundle_root}"
else
  "${ROOT_DIR}/install.sh" install-profile "${args[@]}" proxmox-host-cli "${root}"
fi

# Step 2: Set up veilroot boundary — ONLY on live root (/) to avoid
# polluting a non-live staging rootfs with user/service changes.
if [[ "${root}" == "/" ]]; then
  BOUNDARY_SCRIPT="/usr/local/lib/veilkey-proxy/install-veilroot-boundary.sh"
  if [[ -x "${BOUNDARY_SCRIPT}" ]]; then
    printf '[host-cli] step 2/2: veilroot boundary\n'
    bash "${BOUNDARY_SCRIPT}" "/etc/veilkey/session-tools.toml"
  else
    printf '[host-cli] step 2/2: skipped (install-veilroot-boundary.sh not found at %s)\n' "${BOUNDARY_SCRIPT}"
  fi
else
  printf '[host-cli] step 2/2: skipped (veilroot boundary only runs on live root, got %s)\n' "${root}"
fi

# NOTE: CLI installation is intentionally NOT included here.
# client/cli/install.sh is not root-aware and would install to the
# host machine regardless of the target root. Install CLI separately
# on the target machine after this profile completes.

printf '[host-cli] completed\n'
