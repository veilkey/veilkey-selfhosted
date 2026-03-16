#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SESSION_GUARD="${ROOT_DIR}/scripts/proxmox-live-session.sh"

usage() {
  cat <<'EOF'
Usage: ./scripts/proxmox-lxc-allinone-export-bootstrap.sh <vmid> [dest_root]

Export bootstrap SSH artifacts from a Proxmox LXC all-in-one node to the host.

- reads `.pub` and `.enc` only
- writes a host-side manifest with sha256
- keeps the unencrypted private key inside the LXC
EOF
}

if [[ "${1:-}" =~ ^(-h|--help)$ || $# -lt 1 ]]; then
  usage
  exit $([[ $# -lt 1 ]] && echo 1 || echo 0)
fi

vmid="$1"
dest_root="${2:-/opt/veilkey/bootstrap-exports}"
bootstrap_dir="${VEILKEY_BOOTSTRAP_SSH_DIR:-/etc/veilkey/bootstrap/ssh}"
key_name="${VEILKEY_BOOTSTRAP_SSH_KEY_NAME:-veilkey-admin}"

"${SESSION_GUARD}" assert "proxmox-lxc-allinone-export-bootstrap"

stage() {
  printf '[lxc-allinone/export] %s\n' "$*"
}

run_in_lxc() {
  local command="$1"
  if command -v vibe_lxc_ops >/dev/null 2>&1; then
    vibe_lxc_ops "${vmid}" "${command}"
    return 0
  fi
  pct exec "${vmid}" -- bash -lc "${command}"
}

read_from_lxc() {
  local path="$1"
  run_in_lxc "base64 -w0 '${path}'"
}

hostname_from_lxc="$(run_in_lxc 'hostname' | tail -n 1)"
[[ -n "${hostname_from_lxc}" ]] || {
  echo "Error: unable to resolve hostname from LXC ${vmid}" >&2
  exit 1
}

dest_dir="${dest_root%/}/${vmid}-${hostname_from_lxc}"
pub_src="${bootstrap_dir}/${key_name}.pub"
enc_src="${bootstrap_dir}/${key_name}.enc"

stage "exporting ${pub_src} and ${enc_src} from LXC ${vmid} (${hostname_from_lxc})"
mkdir -p "${dest_dir}"

read_from_lxc "${pub_src}" | base64 -d > "${dest_dir}/${key_name}.pub"
read_from_lxc "${enc_src}" | base64 -d > "${dest_dir}/${key_name}.enc"

pub_sha="$(sha256sum "${dest_dir}/${key_name}.pub" | awk '{print $1}')"
enc_sha="$(sha256sum "${dest_dir}/${key_name}.enc" | awk '{print $1}')"

cat > "${dest_dir}/manifest.json" <<EOF
{
  "vmid": "${vmid}",
  "hostname": "${hostname_from_lxc}",
  "exported_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "source_dir": "${bootstrap_dir}",
  "key_name": "${key_name}",
  "files": {
    "public_key": {
      "path": "${dest_dir}/${key_name}.pub",
      "sha256": "${pub_sha}"
    },
    "encrypted_private_key": {
      "path": "${dest_dir}/${key_name}.enc",
      "sha256": "${enc_sha}"
    }
  }
}
EOF

chmod 600 "${dest_dir}/${key_name}.enc"
chmod 644 "${dest_dir}/${key_name}.pub" "${dest_dir}/manifest.json"

stage "exported to ${dest_dir}"
