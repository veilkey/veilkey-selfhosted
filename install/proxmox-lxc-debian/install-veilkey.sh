#!/bin/bash
set -euo pipefail

# VeilKey installer for Proxmox LXC (Debian)
# Run from the Proxmox host (not inside the container).
#
# Usage:
#   bash install/proxmox-lxc-debian/install.sh
#
# Options (env vars):
#   CTID=110                          Container ID (default: next available)
#   CT_HOSTNAME=veilkey               Hostname (default: veilkey)
#   CT_IP=10.50.0.110/16              IP address (required)
#   CT_GW=10.50.0.1                   Gateway (required)
#   CT_BRIDGE=vmbr1                   Network bridge (default: vmbr1)
#   CT_PASSWORD=                      Root password (prompted if not set)
#   CT_MEMORY=2048                    Memory in MB (default: 2048)
#   CT_CORES=2                        CPU cores (default: 2)
#   CT_DISK=16                        Disk size in GB (default: 16)
#   CT_STORAGE=local-lvm              Storage backend (default: local-lvm)
#   CT_TEMPLATE=debian-13-standard_13.1-2_amd64.tar.zst
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

# --- Defaults ---
CT_HOSTNAME="${CT_HOSTNAME:-veilkey}"
CT_BRIDGE="${CT_BRIDGE:-vmbr1}"
CT_MEMORY="${CT_MEMORY:-2048}"
CT_CORES="${CT_CORES:-2}"
CT_DISK="${CT_DISK:-16}"
CT_STORAGE="${CT_STORAGE:-local-lvm}"
CT_TEMPLATE="${CT_TEMPLATE:-debian-13-standard_13.1-2_amd64.tar.zst}"

# --- Validation ---
if ! command -v pct &>/dev/null; then
    echo "ERROR: pct not found. This script must run on a Proxmox host."
    exit 1
fi

if [[ -z "${CT_IP:-}" ]] || [[ -z "${CT_GW:-}" ]]; then
    echo "ERROR: CT_IP and CT_GW are required."
    echo ""
    echo "Usage:"
    echo "  CT_IP=10.50.0.110/16 CT_GW=10.50.0.1 bash install/proxmox-lxc-debian/install.sh"
    exit 1
fi

if [[ -z "${CT_PASSWORD:-}" ]]; then
    read -s -p "Container root password: " CT_PASSWORD
    echo ""
    if [[ -z "$CT_PASSWORD" ]]; then
        echo "ERROR: Password cannot be empty."
        exit 1
    fi
fi

CTID="${CTID:-$(pvesh get /cluster/nextid)}"

echo "=== VeilKey installer (Proxmox LXC Debian) ==="
echo ""
echo "  CTID:     $CTID"
echo "  Hostname: $CT_HOSTNAME"
echo "  IP:       $CT_IP"
echo "  Gateway:  $CT_GW"
echo "  Bridge:   $CT_BRIDGE"
echo "  Memory:   ${CT_MEMORY}MB"
echo "  Cores:    $CT_CORES"
echo "  Disk:     ${CT_DISK}GB"
echo "  Storage:  $CT_STORAGE"
echo ""

# --- [1/6] Download template ---
echo "[1/6] Downloading template..."
TEMPLATE_PATH="/var/lib/vz/template/cache/$CT_TEMPLATE"
if [[ -f "$TEMPLATE_PATH" ]]; then
    echo "  Template already cached."
else
    pveam download local "$CT_TEMPLATE"
fi

# --- [2/6] Create container ---
echo "[2/6] Creating LXC container (privileged)..."
pct create "$CTID" "local:vztmpl/$CT_TEMPLATE" \
    --hostname "$CT_HOSTNAME" \
    --memory "$CT_MEMORY" \
    --cores "$CT_CORES" \
    --rootfs "$CT_STORAGE:$CT_DISK" \
    --net0 "name=eth0,bridge=$CT_BRIDGE,ip=$CT_IP,gw=$CT_GW" \
    --password "$CT_PASSWORD" \
    --unprivileged 0 \
    --features nesting=1 \
    --start 1
echo "  Created CTID $CTID"

# Wait for container to be ready
sleep 3

# --- [3/6] Install dependencies ---
echo "[3/6] Installing dependencies..."
pct exec "$CTID" -- bash -c "apt-get update -qq && apt-get install -y -qq \
    git docker.io nodejs npm curl ca-certificates" >/dev/null 2>&1
echo "  apt packages installed."

# Docker Compose plugin (not in Debian 13 apt)
echo "  Installing Docker Compose plugin..."
pct exec "$CTID" -- bash -c "
    mkdir -p /usr/lib/docker/cli-plugins
    curl -sL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
        -o /usr/lib/docker/cli-plugins/docker-compose
    chmod +x /usr/lib/docker/cli-plugins/docker-compose" >/dev/null 2>&1
echo "  Docker Compose $(pct exec "$CTID" -- docker compose version --short 2>/dev/null || echo 'installed')."

# Docker Buildx (required for compose build)
echo "  Installing Docker Buildx..."
pct exec "$CTID" -- bash -c '
    BUILDX_VER=$(curl -sL https://api.github.com/repos/docker/buildx/releases/latest | grep tag_name | cut -d"\"" -f4)
    curl -sL "https://github.com/docker/buildx/releases/download/${BUILDX_VER}/buildx-${BUILDX_VER}.linux-amd64" \
        -o /usr/lib/docker/cli-plugins/docker-buildx
    chmod +x /usr/lib/docker/cli-plugins/docker-buildx' >/dev/null 2>&1
echo "  Docker Buildx installed."

# --- [4/6] Locale ---
echo "[4/6] Setting up locale..."
pct exec "$CTID" -- bash -c "
    apt-get install -y -qq locales >/dev/null 2>&1
    localedef -i en_US -f UTF-8 en_US.UTF-8
    echo 'LANG=en_US.UTF-8' > /etc/default/locale" >/dev/null 2>&1
echo "  en_US.UTF-8 configured."

# --- [5/6] Clone and start ---
echo "[5/6] Cloning VeilKey and starting services..."
pct exec "$CTID" -- bash -c "
    cd /root
    git clone --quiet https://github.com/veilkey/veilkey-selfhosted.git
    cd veilkey-selfhosted
    cp .env.example .env
    docker compose up -d" 2>&1 | tail -5
echo "  Services starting (first build may take a few minutes)..."

# --- [6/6] Wait and verify ---
echo "[6/6] Waiting for VaultCenter health check..."
HEALTH=""
for i in $(seq 1 30); do
    HEALTH=$(pct exec "$CTID" -- curl -sk https://localhost:11181/health 2>/dev/null || true)
    if echo "$HEALTH" | grep -q '"status"'; then
        break
    fi
    sleep 5
done

CT_ADDR="${CT_IP%%/*}"

if echo "$HEALTH" | grep -q '"status"'; then
    echo ""
    echo "=== Installation complete ==="
    echo ""
    echo "  VaultCenter: https://$CT_ADDR:11181"
    echo "  LocalVault:  https://$CT_ADDR:11180"
    echo "  Status:      $HEALTH"
    echo ""
    echo "Next steps:"
    echo "  1. Open https://$CT_ADDR:11181 → set master + admin password"
    echo "  2. See docs/setup.md for LocalVault registration"
    echo ""
else
    echo ""
    echo "⚠️  Health check did not respond in time."
    echo "  Services may still be building. Check manually:"
    echo "    pct exec $CTID -- bash -c 'cd /root/veilkey-selfhosted && docker compose ps'"
    echo "    pct exec $CTID -- bash -c 'cd /root/veilkey-selfhosted && docker compose logs'"
fi
