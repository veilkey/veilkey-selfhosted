#!/bin/bash
set -euo pipefail

SERVICE_NAME="veilkey-localvault"

require_cmd() {
  local cmd="$1"
  local hint="$2"
  command -v "$cmd" >/dev/null 2>&1 || {
    echo "Error: required command not found: $cmd ($hint)" >&2
    exit 1
  }
}

ensure_keycenter_env() {
  local vmid="$1"
  local env_file="$2"
  timeout 20s vibe_lxc_ops "$vmid" "python3 - <<'PY'
from pathlib import Path
path = Path('$env_file')
if not path.exists():
    raise SystemExit(0)
lines = path.read_text().splitlines()
has_keycenter = any(line.startswith('VEILKEY_KEYCENTER_URL=') for line in lines)
if not has_keycenter:
    lines.append('VEILKEY_KEYCENTER_URL=')
    path.write_text('\\n'.join(lines) + '\\n')
PY"
}

if [[ "${VEILKEY_SOURCE_ONLY:-0}" == "1" ]]; then
  return 0 2>/dev/null || exit 0
fi

require_cmd pct "deploy-lxc.sh must run on a Proxmox host"
require_cmd vibe_lxc_ops "deploy-lxc.sh must run with vibe_lxc_ops available"

BUILD_BIN="${CI_PROJECT_DIR:-$(pwd)}/.tmp/${SERVICE_NAME}"
mkdir -p "$(dirname "$BUILD_BIN")"
CGO_ENABLED=1 go build -o "$BUILD_BIN" ./cmd/

scan_tmp="$(mktemp)"
trap 'rm -f "$scan_tmp"' EXIT
pct list 2>/dev/null | awk 'NR>1 {print $1}' | xargs -r -P8 -I{} bash -lc '
  vmid="$1"
  service_name="$2"
  if timeout 20s vibe_lxc_ops "$vmid" "systemctl list-unit-files | grep -q '\''^${service_name}.service'\''" >/dev/null 2>&1; then
    printf "%s\n" "$vmid"
  fi
' _ {} "$SERVICE_NAME" >> "$scan_tmp"
mapfile -t vmids < <(sort -n "$scan_tmp")

[[ ${#vmids[@]} -gt 0 ]] || { echo "Error: no LXC with ${SERVICE_NAME}.service found" >&2; exit 1; }

failed_vmids=()

for vmid in "${vmids[@]}"; do
  echo "Checking ${SERVICE_NAME} on LXC ${vmid}..."
  service_unit="$(timeout 20s vibe_lxc_ops "$vmid" "systemctl cat ${SERVICE_NAME}.service" 2>/dev/null || true)"
  exec_path="$(printf '%s\n' "$service_unit" | awk -F= '/^ExecStart=/{print $2; exit}')"
  env_file="$(printf '%s\n' "$service_unit" | awk -F= '/^EnvironmentFile=/{print $2; exit}')"
  if [[ -z "$exec_path" || -z "$env_file" ]]; then
    echo "Warning: could not read service paths for $SERVICE_NAME on $vmid, skipping" >&2
    continue
  fi

  ensure_keycenter_env "$vmid" "$env_file"

  if ! timeout 30s vibe_lxc_ops "$vmid" "systemctl stop ${SERVICE_NAME}"; then
    echo "Warning: failed to stop ${SERVICE_NAME} on ${vmid}, skipping" >&2
    failed_vmids+=("$vmid")
    continue
  fi
  pct push "$vmid" "$BUILD_BIN" "$exec_path"
  if ! timeout 30s vibe_lxc_ops "$vmid" "chmod +x '$exec_path' && systemctl start ${SERVICE_NAME}"; then
    echo "Warning: failed to start ${SERVICE_NAME} on ${vmid}, skipping health check" >&2
    failed_vmids+=("$vmid")
    continue
  fi

  addr="$(timeout 20s vibe_lxc_ops "$vmid" "awk -F= '/^VEILKEY_ADDR=/{print \$2; exit}' '$env_file'")"
  port="${addr##*:}"
  if [[ -z "$port" ]]; then
    echo "Warning: could not parse VEILKEY_ADDR from $env_file on $vmid, skipping health check" >&2
    continue
  fi

  if ! timeout 30s vibe_lxc_ops "$vmid" "curl -sfk https://127.0.0.1:${port}/api/status >/dev/null || curl -sfk https://127.0.0.1:${port}/health >/dev/null || curl -sf http://127.0.0.1:${port}/api/status >/dev/null || curl -sf http://127.0.0.1:${port}/health >/dev/null"; then
    echo "Warning: health check failed for ${SERVICE_NAME} on ${vmid}" >&2
    failed_vmids+=("$vmid")
    continue
  fi
  if ! timeout 20s vibe_lxc_ops "$vmid" "systemctl is-active ${SERVICE_NAME} >/dev/null"; then
    echo "Warning: ${SERVICE_NAME} is not active on ${vmid}" >&2
    failed_vmids+=("$vmid")
    continue
  fi
  echo "Deployed ${SERVICE_NAME} to LXC ${vmid} via ${exec_path}"
done

if [[ ${#failed_vmids[@]} -gt 0 ]]; then
  echo "Deployment completed with failures on: ${failed_vmids[*]}" >&2
  exit 1
fi
