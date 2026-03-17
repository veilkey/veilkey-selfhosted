#!/bin/bash
set -euo pipefail

SERVICE_NAME="${VEILKEY_DEPLOY_SERVICE_NAME:-veilkey-keycenter}"
TARGET_NAME="${VEILKEY_DEPLOY_TARGET_NAME:-veilkey-keycenter}"
RESOLVED_VMID=""

require_cmd() {
  local cmd="$1"
  local hint="$2"
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Error: required command not found: $cmd ($hint)" >&2
    exit 1
  }
}

lxc_exec() {
  local vmid="$1"
  shift
  pct exec "$vmid" -- bash -lc "$*"
}

migrate_legacy_password_env() {
  local vmid="$1"
  local env_file="$2"
  local pw_file="${3:-/etc/veilkey/${SERVICE_NAME}.password}"
  local q_env_file q_pw_file

  printf -v q_env_file '%q' "$env_file"
  printf -v q_pw_file '%q' "$pw_file"

  lxc_exec "$vmid" "
    set -euo pipefail
    env_file=${q_env_file}
    pw_file=${q_pw_file}
    grep -q '^VEILKEY_PASSWORD=' \"\$env_file\" || exit 0
    grep -q '^VEILKEY_PASSWORD_FILE=' \"\$env_file\" && exit 0
    password=\$(grep '^VEILKEY_PASSWORD=' \"\$env_file\" | head -n1 | cut -d= -f2-)
    [ -n \"\$password\" ] || { echo 'Error: VEILKEY_PASSWORD is empty in legacy env file' >&2; exit 1; }
    install -d -m 700 \"\$(dirname \"\$pw_file\")\"
    umask 077
    printf '%s\n' \"\$password\" >\"\$pw_file\"
    chmod 600 \"\$pw_file\"
    grep -v '^VEILKEY_PASSWORD=' \"\$env_file\" >\"\$env_file.tmp\"
    printf 'VEILKEY_PASSWORD_FILE=%s\n' \"\$pw_file\" >>\"\$env_file.tmp\"
    mv \"\$env_file.tmp\" \"\$env_file\"
  "
}

resolve_target() {
  local vmid
  vmid="$(pct list 2>/dev/null | awk -v name="$TARGET_NAME" 'NR>1 && $NF==name{print $1; exit}')"
  if [[ -n "$vmid" ]]; then
    RESOLVED_VMID="$vmid"
    return 0
  fi

  if [[ -z "${VEILKEY_DEPLOY_TARGET_NAME:-}" && -z "${VEILKEY_DEPLOY_SERVICE_NAME:-}" ]]; then
    vmid="$(pct list 2>/dev/null | awk 'NR>1 && $NF=="veilkey-allinone"{print $1; exit}')"
    if [[ -n "$vmid" ]]; then
      TARGET_NAME="veilkey-allinone"
      SERVICE_NAME="veilkey-server"
      RESOLVED_VMID="$vmid"
      return 0
    fi
  fi

  return 1
}

main() {
  local build_bin build_sha vmid service_unit exec_path env_file addr port

  require_cmd pct "deploy-lxc.sh must run on a Proxmox host"
  build_bin="${CI_PROJECT_DIR:-$(pwd)}/.tmp/${SERVICE_NAME}"
  mkdir -p "$(dirname "$build_bin")"
  CGO_ENABLED=1 go build -o "$build_bin" ./cmd/
  build_sha="$(sha256sum "$build_bin" | awk '{print $1}')"

  resolve_target
  vmid="${RESOLVED_VMID:-}"
  [[ -n "$vmid" ]] || { echo "Error: target LXC not found: $TARGET_NAME" >&2; exit 1; }

  service_unit="$(lxc_exec "$vmid" "systemctl cat ${SERVICE_NAME}.service" 2>/dev/null)"
  exec_path="$(printf '%s\n' "$service_unit" | awk -F= '/^ExecStart=/{print $2; exit}')"
  env_file="$(printf '%s\n' "$service_unit" | awk -F= '/^EnvironmentFile=/{print $2; exit}')"
  [[ -n "$exec_path" && -n "$env_file" ]] || { echo "Error: could not read service paths for $SERVICE_NAME on $vmid" >&2; exit 1; }

  migrate_legacy_password_env "$vmid" "$env_file"

  lxc_exec "$vmid" "systemctl stop ${SERVICE_NAME}"
  pct push "$vmid" "$build_bin" "$exec_path"
  lxc_exec "$vmid" "chmod +x '$exec_path' && systemctl start ${SERVICE_NAME}"
  remote_sha="$(lxc_exec "$vmid" "sha256sum '$exec_path' | cut -d' ' -f1")"
  [[ "$remote_sha" == "$build_sha" ]] || { echo "Error: deployed binary hash mismatch" >&2; exit 1; }

  addr="$(lxc_exec "$vmid" "awk -F= '/^VEILKEY_ADDR=/{print \$2; exit}' '$env_file'")"
  port="${addr##*:}"
  [[ -n "$port" ]] || { echo "Error: could not parse VEILKEY_ADDR from $env_file" >&2; exit 1; }

  lxc_exec "$vmid" "curl -sfk https://127.0.0.1:${port}/api/status >/dev/null || curl -sfk https://127.0.0.1:${port}/health >/dev/null || curl -sf http://127.0.0.1:${port}/api/status >/dev/null || curl -sf http://127.0.0.1:${port}/health >/dev/null"
  lxc_exec "$vmid" "systemctl is-active ${SERVICE_NAME} >/dev/null"

  echo "Deployed ${SERVICE_NAME} to LXC ${vmid} via ${exec_path}"
}

if [[ "${VEILKEY_SOURCE_ONLY:-0}" == "1" ]]; then
  return 0 2>/dev/null || exit 0
fi

main "$@"
