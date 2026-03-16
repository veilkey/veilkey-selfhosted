#!/usr/bin/env bash
set -euo pipefail

STATE_DIR="${VEILKEY_INSTALLER_SESSION_STATE_DIR:-${TMPDIR:-/tmp}/veilkey-installer}"
STATE_FILE="${STATE_DIR}/proxmox-live-session.env"

current_session_id() {
  if [[ -n "${TMUX_PANE:-}" ]]; then
    printf 'tmux:%s\n' "${TMUX_PANE}"
    return 0
  fi
  if tty >/dev/null 2>&1; then
    printf 'tty:%s:ppid:%s\n' "$(tty)" "${PPID}"
    return 0
  fi
  printf 'pid:%s:ppid:%s\n' "$$" "${PPID}"
}

claim_session() {
  local label="${1:-proxmox-live}"
  local session_id
  session_id="$(current_session_id)"
  mkdir -p "${STATE_DIR}"
  umask 077
  cat > "${STATE_FILE}" <<EOF
SESSION_ID=${session_id}
LABEL=${label}
HOSTNAME=$(hostname)
PID=$$
CREATED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
EOF
  printf 'claimed live Proxmox session: %s (%s)\n' "${label}" "${session_id}"
}

show_session() {
  [[ -f "${STATE_FILE}" ]] || {
    echo "no claimed live Proxmox session"
    return 0
  }
  cat "${STATE_FILE}"
}

status_session() {
  local current
  current="$(current_session_id)"
  printf 'CURRENT_SESSION=%s\n' "${current}"
  if [[ -f "${STATE_FILE}" ]]; then
    cat "${STATE_FILE}"
  else
    echo "WORK_SESSION=unclaimed"
  fi
}

clear_session() {
  rm -f "${STATE_FILE}"
  echo "cleared live Proxmox session claim"
}

require_live_session() {
  local operation="${1:-proxmox-live-op}"
  local session_id
  session_id="$(current_session_id)"
  [[ -f "${STATE_FILE}" ]] || {
    cat >&2 <<EOF
Error: live Proxmox operation '${operation}' requires a claimed validation session.
Open a dedicated shell/tmux session first, then run:
  ./scripts/proxmox-live-session.sh claim ${operation}
EOF
    exit 1
  }
  # shellcheck disable=SC1090
  source "${STATE_FILE}"
  if [[ "${SESSION_ID:-}" != "${session_id}" ]]; then
    cat >&2 <<EOF
Error: live Proxmox operation '${operation}' is claimed by another session.
  claimed: ${SESSION_ID:-unknown}
  current: ${session_id}
Run this from the claimed session, or clear/re-claim:
  ./scripts/proxmox-live-session.sh show
  ./scripts/proxmox-live-session.sh clear
  ./scripts/proxmox-live-session.sh claim ${operation}
EOF
    exit 1
  fi
}

cmd="${1:-show}"
shift || true

case "${cmd}" in
  claim)
    claim_session "${1:-proxmox-live}"
    ;;
  show)
    show_session
    ;;
  status)
    status_session
    ;;
  clear)
    clear_session
    ;;
  assert)
    require_live_session "${1:-proxmox-live-op}"
    ;;
  *)
    echo "Usage: $0 {claim [label]|show|status|clear|assert [operation]}" >&2
    exit 2
    ;;
esac
