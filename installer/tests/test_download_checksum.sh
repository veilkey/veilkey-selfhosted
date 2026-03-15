#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

# --- Test 1: verify_sha256 passes on correct checksum ---
source <(sed -n '/^verify_sha256()/,/^}/p' install.sh)

tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT
echo "test-artifact-content" > "$tmp_file"

expected="$(sha256sum "$tmp_file" | awk '{print $1}')"
verify_sha256 "$tmp_file" "$expected"
echo "ok: verify_sha256 passes with correct checksum"

# --- Test 2: verify_sha256 fails on wrong checksum ---
if verify_sha256 "$tmp_file" "0000000000000000000000000000000000000000000000000000000000000000" 2>/dev/null; then
  echo "FAIL: verify_sha256 should have rejected wrong checksum" >&2
  exit 1
fi
echo "ok: verify_sha256 rejects incorrect checksum"

# Recreate file (verify_sha256 deletes it on mismatch)
echo "test-artifact-content" > "$tmp_file"

# --- Test 3: verify_sha256 skips when checksum is "none" ---
verify_sha256 "$tmp_file" "none" 2>/dev/null
echo "ok: verify_sha256 skips when checksum is 'none'"

# --- Test 4: verify_sha256 skips when checksum is empty ---
verify_sha256 "$tmp_file" "" 2>/dev/null
echo "ok: verify_sha256 skips when checksum is empty"

# --- Test 5: manifest parser includes sha256 field ---
tmp_manifest="$(mktemp)"
trap 'rm -f "$tmp_file" "$tmp_manifest"' EXIT
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null

download_plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan-download proxmox-lxc-allinone)"
# Each non-header line should have 5 fields (order name filename url sha256)
while read -r line; do
  [[ "$line" == "[profile]"* ]] && continue
  field_count=$(echo "$line" | awk '{print NF}')
  if [[ "$field_count" -lt 5 ]]; then
    echo "FAIL: download plan line has $field_count fields, expected >= 5: $line" >&2
    exit 1
  fi
done <<< "$download_plan"
echo "ok: plan-download includes sha256 field"

# --- Test 6: stage plan includes sha256 field ---
stage_plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan-stage proxmox-lxc-allinone)"
while read -r line; do
  [[ "$line" == release_* || "$line" == profile=* ]] && continue
  if ! echo "$line" | grep -q 'sha256='; then
    echo "FAIL: stage plan line missing sha256 field: $line" >&2
    exit 1
  fi
done <<< "$stage_plan"
echo "ok: plan-stage includes sha256 field"

echo ""
echo "all download checksum tests passed"
