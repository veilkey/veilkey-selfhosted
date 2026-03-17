#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
check_temp_issuance=0
localvault_health_url="${VEILKEY_LOCALVAULT_HEALTH_URL:-https://127.0.0.1:10180/health}"
keycenter_health_url="${VEILKEY_KEYCENTER_HEALTH_URL:?VEILKEY_KEYCENTER_HEALTH_URL must be set}"
keycenter_vmid="${VEILKEY_KEYCENTER_VMID:-100206}"
veilroot_verify_bin="${VEILKEY_VEILROOT_VERIFY_BIN:-/usr/local/bin/verify-veilroot-session}"
veilroot_default_profile="${VEILKEY_VEILROOT_DEFAULT_PROFILE:-$(/usr/local/bin/veilkey-session-config veilroot-default-profile 2>/dev/null || echo codex)}"
veilroot_user="${VEILKEY_VEILROOT_USER:-veilroot}"
veilroot_codex_config="${VEILKEY_VEILROOT_CODEX_CONFIG:-/home/${veilroot_user}/.codex/config.toml}"

usage() {
  cat <<'EOF'
usage: doctor-veilkey.sh [--check-temp-issuance] [proxy_vmid] [lines]

  --check-temp-issuance   issue a real agent temp secret and verify TEMP semantics
  proxy_vmid              proxy LXC VMID (default: 100208)
  lines                   number of recent audit lines to show (default: 5)
EOF
}

args=()
while (($#)); do
  case "$1" in
    --check-temp-issuance)
      check_temp_issuance=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      args+=("$1")
      shift
      ;;
  esac
done

proxy_vmid="${args[0]:-100208}"
lines="${args[1]:-5}"

echo "== veilkey health =="
echo "-- localvault --"
curl -fsSk "$localvault_health_url"
echo
echo "-- keycenter --"
curl -fsSk "$keycenter_health_url"
echo
echo

echo "== veilroot boundary =="
if command -v "$veilroot_verify_bin" >/dev/null 2>&1; then
  "$veilroot_verify_bin"
else
  echo "missing: $veilroot_verify_bin"
fi

if [[ -e "$veilroot_codex_config" ]]; then
  if id "$veilroot_user" >/dev/null 2>&1; then
    if su -s /bin/bash - "$veilroot_user" -c "test -r '$veilroot_codex_config'" >/dev/null 2>&1; then
      echo "veilroot codex config readable: $veilroot_codex_config"
    else
      echo "WARN veilroot cannot read codex config: $veilroot_codex_config"
    fi
  else
    echo "skip veilroot codex config check: user not found ($veilroot_user)"
  fi
  resolved_cfg="$(readlink -f "$veilroot_codex_config" 2>/dev/null || printf '%s' "$veilroot_codex_config")"
  stat -c 'codex-config target: %A %U:%G %n' "$resolved_cfg" 2>/dev/null || true
else
  echo "missing veilroot codex config: $veilroot_codex_config"
fi

for unit in "veilkey-veilroot-observe@${veilroot_default_profile}.service" "veilkey-veilroot-egress-guard@${veilroot_default_profile}.service"; do
  if systemctl list-unit-files "$unit" >/dev/null 2>&1; then
    systemctl is-enabled "$unit" 2>/dev/null || true
    systemctl is-active "$unit" 2>/dev/null || true
  else
    echo "missing unit: $unit"
  fi
done
echo "supported host boundary: veilroot only"
echo

if [[ "$check_temp_issuance" == "1" ]]; then
  echo "== keycenter temp issuance =="
  resp="$(vibe_lxc_ops "$keycenter_vmid" "curl -fsSk -X POST https://127.0.0.1:10180/api/agents/veilkey-proxy/secrets -H 'Content-Type: application/json' -d '{\"name\":\"doctor-temp-check\",\"value\":\"doctor-temp-check-value\"}'")"
  printf '%s\n' "$resp"
  python3 - <<'PY' "$resp"
import json, sys
obj = json.loads(sys.argv[1])
scope = obj.get("scope")
status = obj.get("status")
token = obj.get("token", "")
if scope != "TEMP" or status != "temp" or not token.startswith("VK:TEMP:"):
    raise SystemExit(f"unexpected temp issuance: scope={scope} status={status} token={token}")
print("temp issuance OK")
PY
  echo
else
  echo "== keycenter temp issuance =="
  echo "skipped (use --check-temp-issuance to run a real TEMP issuance check)"
  echo
fi

echo "== recent rewrite scope sanity =="
for profile in default codex claude opencode; do
  echo "-- $profile --"
  vibe_lxc_ops "$proxy_vmid" "python3 - <<'PY'
from pathlib import Path
import json
path = Path('/var/log/veilkey-proxy/${profile}-rewrite.jsonl')
if not path.exists():
    print('missing')
    raise SystemExit(0)
rows = []
for line in path.read_text(encoding='utf-8', errors='ignore').splitlines():
    line = line.strip()
    if not line:
        continue
    try:
        rows.append(json.loads(line))
    except Exception:
        pass
recent = rows[-5:]
vals = [r.get('veilkey','') for r in recent]
print(vals if vals else 'empty')
bad = [v for v in vals if v and not v.startswith('VK:TEMP:')]
if bad:
    raise SystemExit('non-temp refs in recent rewrite log: ' + ', '.join(bad))
PY"
  echo
done
