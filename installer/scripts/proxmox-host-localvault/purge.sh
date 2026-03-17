#!/usr/bin/env bash
set -euo pipefail

root="${1:-/}"

stage() {
  printf '[host-localvault/purge] %s\n' "$*"
}

disconnect_from_keycenter() {
  local status_json node_id keycenter_url

  stage "checking current LocalVault node_id"
  status_json="$(curl -sfk https://127.0.0.1:10180/api/status 2>/dev/null || curl -sf http://127.0.0.1:10180/api/status 2>/dev/null || true)"
  [[ -n "${status_json}" ]] || return 0

  node_id="$(printf '%s' "${status_json}" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("node_id",""))' 2>/dev/null || true)"
  [[ -n "${node_id}" ]] || return 0

  if [[ -f /etc/veilkey/localvault.env ]]; then
    # shellcheck disable=SC1091
    . /etc/veilkey/localvault.env
  fi
  keycenter_url="${VEILKEY_KEYCENTER_URL:-}"
  [[ -n "${keycenter_url}" ]] || return 0

  stage "unregistering ${node_id} from ${keycenter_url}"
  curl -kfsS -X DELETE "${keycenter_url%/}/api/agents/by-node/${node_id}" >/dev/null
}

if [[ "${root}" != "/" ]]; then
  stage "purging staged root ${root}"
  rm -f "${root%/}/etc/systemd/system/veilkey-localvault.service"
  rm -f "${root%/}/etc/veilkey/localvault.env"
  rm -f "${root%/}/etc/veilkey/localvault.env.example"
  rm -f "${root%/}/usr/local/bin/veilkey-localvault"
  rm -rf "${root%/}/opt/veilkey/localvault"
  exit 0
fi

stage "this removes local service, env, binary, data, and KeyCenter registration"
disconnect_from_keycenter
stage "stopping service"
systemctl disable --now veilkey-localvault.service 2>/dev/null || true
stage "removing local files"
rm -f /etc/systemd/system/veilkey-localvault.service
rm -f /etc/veilkey/localvault.env
rm -f /etc/veilkey/localvault.env.example
rm -f /usr/local/bin/veilkey-localvault
rm -rf /opt/veilkey/localvault
systemctl daemon-reload
stage "completed"
