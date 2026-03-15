#!/bin/bash
set -euo pipefail

SERVICE_NAME="${VEILKEY_DEPLOY_SERVICE_NAME:-veilkey-keycenter}"
TARGET_NAME="${VEILKEY_DEPLOY_TARGET_NAME:-veilkey-keycenter}"

require_cmd() {
  local cmd="$1"
  local hint="$2"
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Error: required command not found: $cmd ($hint)" >&2
    exit 1
  }
}

require_cmd pct "deploy-lxc.sh must run on a Proxmox host"

lxc_exec() {
  local vmid="$1"
  shift
  pct exec "$vmid" -- bash -lc "$*"
}

resolve_target() {
  local vmid
  vmid="$(pct list 2>/dev/null | awk -v name="$TARGET_NAME" '$3==name{print $1; exit}')"
  if [[ -n "$vmid" ]]; then
    printf '%s' "$vmid"
    return 0
  fi

  if [[ -z "${VEILKEY_DEPLOY_TARGET_NAME:-}" && -z "${VEILKEY_DEPLOY_SERVICE_NAME:-}" ]]; then
    vmid="$(pct list 2>/dev/null | awk '$3=="veilkey-allinone"{print $1; exit}')"
    if [[ -n "$vmid" ]]; then
      TARGET_NAME="veilkey-allinone"
      SERVICE_NAME="veilkey-server"
      printf '%s' "$vmid"
      return 0
    fi
  fi

  return 1
}

BUILD_BIN="${CI_PROJECT_DIR:-$(pwd)}/.tmp/${SERVICE_NAME}"
mkdir -p "$(dirname "$BUILD_BIN")"
CGO_ENABLED=1 go build -o "$BUILD_BIN" ./cmd/
build_sha="$(sha256sum "$BUILD_BIN" | awk '{print $1}')"

vmid="$(resolve_target)"
[[ -n "$vmid" ]] || { echo "Error: target LXC not found: $TARGET_NAME" >&2; exit 1; }

service_unit="$(lxc_exec "$vmid" "systemctl cat ${SERVICE_NAME}.service" 2>/dev/null)"
exec_path="$(printf '%s\n' "$service_unit" | awk -F= '/^ExecStart=/{print $2; exit}')"
env_file="$(printf '%s\n' "$service_unit" | awk -F= '/^EnvironmentFile=/{print $2; exit}')"
[[ -n "$exec_path" && -n "$env_file" ]] || { echo "Error: could not read service paths for $SERVICE_NAME on $vmid" >&2; exit 1; }

lxc_exec "$vmid" "systemctl stop ${SERVICE_NAME}"
pct push "$vmid" "$BUILD_BIN" "$exec_path"
lxc_exec "$vmid" "chmod +x '$exec_path' && systemctl start ${SERVICE_NAME}"
remote_sha="$(lxc_exec "$vmid" "sha256sum '$exec_path' | cut -d' ' -f1")"
[[ "$remote_sha" == "$build_sha" ]] || { echo "Error: deployed binary hash mismatch" >&2; exit 1; }

addr="$(lxc_exec "$vmid" "awk -F= '/^VEILKEY_ADDR=/{print \$2; exit}' '$env_file'")"
port="${addr##*:}"
[[ -n "$port" ]] || { echo "Error: could not parse VEILKEY_ADDR from $env_file" >&2; exit 1; }

lxc_exec "$vmid" "curl -sf http://127.0.0.1:${port}/api/status >/dev/null || curl -sf http://127.0.0.1:${port}/health >/dev/null"
lxc_exec "$vmid" "systemctl is-active ${SERVICE_NAME} >/dev/null"

echo "Deployed ${SERVICE_NAME} to LXC ${vmid} via ${exec_path}"
