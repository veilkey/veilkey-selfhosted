#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_state="$(mktemp -d)"
trap 'rm -rf "$tmp_state"' EXIT

export VEILKEY_INSTALLER_SESSION_STATE_DIR="$tmp_state"
export TMUX_PANE='%test-pane'

status_before="$(./scripts/proxmox-live-session.sh status)"
printf '%s\n' "$status_before" | grep -F 'CURRENT_SESSION=tmux:%test-pane' >/dev/null
printf '%s\n' "$status_before" | grep -F 'WORK_SESSION=unclaimed' >/dev/null

./scripts/proxmox-live-session.sh claim runtime-validate >/dev/null

status_after="$(./scripts/proxmox-live-session.sh status)"
printf '%s\n' "$status_after" | grep -F 'CURRENT_SESSION=tmux:%test-pane' >/dev/null
printf '%s\n' "$status_after" | grep -F 'SESSION_ID=tmux:%test-pane' >/dev/null
printf '%s\n' "$status_after" | grep -F 'LABEL=runtime-validate' >/dev/null

./scripts/proxmox-live-session.sh assert proxmox-lxc-runtime-install
./scripts/proxmox-live-session.sh clear >/dev/null

if ./scripts/proxmox-live-session.sh assert proxmox-lxc-runtime-install >/dev/null 2>&1; then
  echo "expected assert to fail without a claimed session" >&2
  exit 1
fi

echo "ok: proxmox live session guard"
