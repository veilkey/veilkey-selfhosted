#!/bin/bash
set -euo pipefail

KEYCENTER_NAME="veilkey-keycenter"
SERVICE_NAME="veilkey-localvault"

lxc_exec() {
  local vmid="$1"
  shift
  pct exec "$vmid" -- bash -lc "$*"
}

keycenter_vmid="$(pct list 2>/dev/null | awk -v name="$KEYCENTER_NAME" '$3==name{print $1; exit}')"
[[ -n "$keycenter_vmid" ]] || { echo "Error: keycenter LXC not found: $KEYCENTER_NAME" >&2; exit 1; }

env_file="$(lxc_exec "$keycenter_vmid" "systemctl cat ${KEYCENTER_NAME}.service" 2>/dev/null | awk -F= '/^EnvironmentFile=/{print $2; exit}')"
[[ -n "$env_file" ]] || { echo "Error: could not locate keycenter env file" >&2; exit 1; }

db_path="$(lxc_exec "$keycenter_vmid" "awk -F= '/^VEILKEY_DB_PATH=/{print \$2; exit}' '$env_file'")"
[[ -n "$db_path" ]] || { echo "Error: could not locate keycenter DB path" >&2; exit 1; }

scan_tmp="$(mktemp)"
status_tmp="$(mktemp)"
trap 'rm -f "$scan_tmp" "$status_tmp"' EXIT

while read -r vmid; do
  [[ -n "$vmid" ]] || continue
  if timeout 20s pct exec "$vmid" -- bash -lc "systemctl list-unit-files | grep -q '^${SERVICE_NAME}.service'" >/dev/null 2>&1; then
    printf '%s\n' "$vmid" >> "$scan_tmp"
  fi
done < <(pct list 2>/dev/null | awk 'NR>1 {print $1}')

mapfile -t vmids < <(sort -n "$scan_tmp")
[[ ${#vmids[@]} -gt 0 ]] || { echo "Error: no LXC with ${SERVICE_NAME}.service found" >&2; exit 1; }

synced=0
skipped=0

for vmid in "${vmids[@]}"; do
  service_unit="$(timeout 20s pct exec "$vmid" -- bash -lc "systemctl cat ${SERVICE_NAME}.service" 2>/dev/null || true)"
  env_file="$(printf '%s\n' "$service_unit" | awk -F= '/^EnvironmentFile=/{print $2; exit}')"
  addr=""
  if [[ -n "$env_file" ]]; then
    addr="$(timeout 20s pct exec "$vmid" -- bash -lc "awk -F= '/^VEILKEY_ADDR=/{print \$2; exit}' '$env_file'" 2>/dev/null || true)"
  fi
  port="${addr##*:}"
  [[ -n "$port" ]] || port="10180"

  status_json="$(timeout 20s pct exec "$vmid" -- bash -lc "curl -sf http://127.0.0.1:${port}/api/status" 2>/dev/null || true)"
  if [[ -z "$status_json" ]]; then
    echo "skip ${vmid}: status unavailable on :${port}" >&2
    skipped=$((skipped + 1))
    continue
  fi

  printf '%s\n' "$status_json" > "$status_tmp"

  vault_node_uuid="$(jq -r '.vault_node_uuid // .node_id // empty' < "$status_tmp")"
  vault_hash="$(jq -r '.vault_hash // empty' < "$status_tmp")"
  vault_name="$(jq -r '.vault_name // empty' < "$status_tmp")"
  key_version="$(jq -r '.version // 1' < "$status_tmp")"
  secrets_count="$(jq -r '.secrets_count // 0' < "$status_tmp")"
  configs_count="$(jq -r '.configs_count // 0' < "$status_tmp")"

  # Validate numeric fields to prevent SQL injection
  [[ "$key_version" =~ ^[0-9]+$ ]] || key_version=0
  [[ "$secrets_count" =~ ^[0-9]+$ ]] || secrets_count=0
  [[ "$configs_count" =~ ^[0-9]+$ ]] || configs_count=0

  if [[ -z "$vault_node_uuid" || -z "$vault_hash" || -z "$vault_name" ]]; then
    echo "skip ${vmid}: incomplete status payload" >&2
    skipped=$((skipped + 1))
    continue
  fi

  sql_vault_hash="${vault_hash//\'/''}"
  sql_vault_name="${vault_name//\'/''}"
  sql_node_id="${vault_node_uuid//\'/''}"

  lxc_exec "$keycenter_vmid" "sqlite3 '$db_path' \"UPDATE agents SET vault_hash='${sql_vault_hash}', vault_name='${sql_vault_name}', key_version=${key_version}, secrets_count=${secrets_count}, configs_count=${configs_count}, version=${key_version} WHERE node_id='${sql_node_id}';\""
  echo "synced ${vmid}: ${vault_name}:${vault_hash} v${key_version}"
  synced=$((synced + 1))
done

echo "Synced ${synced} agent rows; skipped ${skipped}."
