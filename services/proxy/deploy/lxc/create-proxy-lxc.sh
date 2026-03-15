#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

usage() {
  cat <<'EOF'
usage: create-proxy-lxc.sh <vmid> <hostname> <ip> [options]

required:
  vmid                      target VMID
  hostname                  target hostname
  ip                        IPv4 CIDR for net0 (example: 10.0.0.9/8)

options:
  --template <volid>        LXC template volid
  --storage <storage>       rootfs storage
  --disk <size>             rootfs size (default: 20G)
  --bridge <bridge>         bridge name (default: vmbr1)
  --gw <ip>                 gateway (default: $VEILKEY_LXC_GATEWAY)
  --nameserver <ip>         nameserver (default: $VEILKEY_LXC_NAMESERVER)
  --cores <n>               cpu cores (default: 2)
  --memory <mb>             memory mb (default: 2048)
  --swap <mb>               swap mb (default: 512)
  --onboot <0|1>            onboot flag (default: 1)
  --admin-user <name>       create admin user inside LXC
  --admin-pubkey <path>     public key file for the admin user
  --no-sudo-nopasswd        do not grant passwordless sudo
  --description <text>      LXC description
  --skip-install            skip proxy payload install
  --skip-verify             skip verify-proxy-lxc.sh

example:
  ./deploy/lxc/create-proxy-lxc.sh 100209 veilkey-proxy-test 10.0.0.9/8 \
    --admin-user proxyops \
    --admin-pubkey ~/.ssh/id_ed25519.pub \
    --onboot 0
EOF
}

[[ $# -ge 3 ]] || { usage >&2; exit 2; }

vmid="$1"
hostname="$2"
ip_cidr="$3"
shift 3

template="local:vztmpl/debian-12-standard_12.12-1_amd64.tar.zst"
storage="nvme-hp-4tb"
disk_size="20G"
bridge="vmbr1"
gateway="${VEILKEY_LXC_GATEWAY:?VEILKEY_LXC_GATEWAY must be set}"
nameserver="${VEILKEY_LXC_NAMESERVER:?VEILKEY_LXC_NAMESERVER must be set}"
cores="2"
memory="2048"
swap="512"
onboot="1"
admin_user=""
admin_pubkey=""
grant_nopasswd="1"
skip_install="0"
skip_verify="0"
description=""

while (($#)); do
  case "$1" in
    --template) template="$2"; shift 2 ;;
    --storage) storage="$2"; shift 2 ;;
    --disk) disk_size="$2"; shift 2 ;;
    --bridge) bridge="$2"; shift 2 ;;
    --gw) gateway="$2"; shift 2 ;;
    --nameserver) nameserver="$2"; shift 2 ;;
    --cores) cores="$2"; shift 2 ;;
    --memory) memory="$2"; shift 2 ;;
    --swap) swap="$2"; shift 2 ;;
    --onboot) onboot="$2"; shift 2 ;;
    --admin-user) admin_user="$2"; shift 2 ;;
    --admin-pubkey) admin_pubkey="$2"; shift 2 ;;
    --no-sudo-nopasswd) grant_nopasswd="0"; shift ;;
    --description) description="$2"; shift 2 ;;
    --skip-install) skip_install="1"; shift ;;
    --skip-verify) skip_verify="1"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unsupported option: $1" >&2; exit 2 ;;
  esac
done

if [[ -n "$admin_user" ]] && ! [[ "$admin_user" =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  echo "invalid admin user name: $admin_user" >&2
  exit 2
fi

if pct status "$vmid" >/dev/null 2>&1; then
  echo "VMID $vmid already exists" >&2
  exit 1
fi

if [[ -n "$admin_pubkey" && ! -f "$admin_pubkey" ]]; then
  echo "missing admin public key: $admin_pubkey" >&2
  exit 1
fi

if [[ "$skip_install" == "1" && "$skip_verify" != "1" ]]; then
  echo "--skip-install requires --skip-verify (verify-proxy-lxc.sh is installed by the payload step)" >&2
  exit 1
fi

if [[ -z "$description" ]]; then
  description=$'## '"$hostname"$'\n\n| 항목 | 값 |\n|------|-----|\n| IP | '"$ip_cidr"$' |\n| 역할 | VeilKey outbound proxy |\n'
fi

disk_size_normalized="${disk_size%G}"
disk_size_normalized="${disk_size_normalized%g}"

echo "== create lxc =="
echo "vmid=$vmid"
echo "hostname=$hostname"
echo "ip=$ip_cidr"
echo "template=$template"
echo "storage=$storage"

pct create "$vmid" "$template" \
  --hostname "$hostname" \
  --arch amd64 \
  --ostype debian \
  --unprivileged 1 \
  --features nesting=1 \
  --cores "$cores" \
  --memory "$memory" \
  --swap "$swap" \
  --rootfs "${storage}:${disk_size_normalized}" \
  --nameserver "$nameserver" \
  --net0 "name=eth0,bridge=${bridge},gw=${gateway},ip=${ip_cidr},type=veth" \
  --onboot "$onboot" \
  --description "$description"

pct start "$vmid"

echo "== base packages =="
vibe_lxc_ops "$vmid" "apt-get update && apt-get install -y ca-certificates curl jq python3 tmux sudo openssh-server"

if [[ -n "$admin_user" ]]; then
  echo "== admin user =="
  vibe_lxc_ops --stdin "$vmid" <<EOF
set -euo pipefail
id -u '$admin_user' >/dev/null 2>&1 || useradd -m -s /bin/bash '$admin_user'
usermod -aG sudo '$admin_user'
passwd -l '$admin_user' >/dev/null 2>&1 || true
install -d -m 0700 -o '$admin_user' -g '$admin_user' /home/'$admin_user'/.ssh
EOF
  if [[ "$grant_nopasswd" == "1" ]]; then
    vibe_lxc_ops "$vmid" "printf '%s\n' '$admin_user ALL=(ALL) NOPASSWD:ALL' > '/etc/sudoers.d/90-$admin_user' && chmod 0440 '/etc/sudoers.d/90-$admin_user'"
  fi
  if [[ -n "$admin_pubkey" ]]; then
    pubkey_contents="$(<"$admin_pubkey")"
    vibe_lxc_ops --stdin "$vmid" <<EOF
set -euo pipefail
cat > /home/'$admin_user'/.ssh/authorized_keys <<'KEY'
$pubkey_contents
KEY
chown '$admin_user':'$admin_user' /home/'$admin_user'/.ssh/authorized_keys
chmod 0600 /home/'$admin_user'/.ssh/authorized_keys
EOF
  fi
fi

if [[ "$skip_install" != "1" ]]; then
  echo "== proxy payload install =="
  "$repo_root/deploy/lxc/install-proxy-lxc.sh" "$vmid"
fi

if [[ "$skip_verify" != "1" ]]; then
  echo "== proxy verify =="
  vibe_lxc_ops "$vmid" "/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh"
fi

echo "== done =="
echo "created VMID $vmid ($hostname)"
if [[ -n "$admin_user" ]]; then
  echo "admin user: $admin_user"
fi
