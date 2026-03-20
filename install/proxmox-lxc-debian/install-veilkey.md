# Proxmox LXC Installation (Debian)

Run VeilKey Self-Hosted inside a Proxmox LXC container.

> **Tested on:** Proxmox VE 8.x, Debian 13 (trixie) LXC, Docker 26+

## Quick Start (script)

Run from the Proxmox host:

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
CT_IP=10.50.0.110/16 CT_GW=10.50.0.1 bash install/proxmox-lxc-debian/install.sh
```

The script creates a privileged LXC container, installs all dependencies, and starts VeilKey services. See [install.sh](./install.sh) for all options.

For manual step-by-step installation, continue below.

---

## Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| RAM | 1 GB | 2 GB |
| Disk | 8 GB | 16 GB |
| Cores | 1 | 2 |

## 1. Create LXC Container

> **Important:** The container must be **privileged** (`--unprivileged 0`).
> GitLab-style services and Docker inside LXC require sysctl access that unprivileged containers cannot provide.

```bash
# Download template (if not cached)
pveam download local debian-13-standard_13.1-2_amd64.tar.zst

# Create privileged container
pct create <CTID> local:vztmpl/debian-13-standard_13.1-2_amd64.tar.zst \
  --hostname veilkey \
  --memory 2048 \
  --cores 2 \
  --rootfs local-lvm:16 \
  --net0 name=eth0,bridge=vmbr1,ip=<IP>/16,gw=<GATEWAY> \
  --password '<PASSWORD>' \
  --unprivileged 0 \
  --features nesting=1 \
  --start 1
```

Replace `<CTID>`, `<IP>`, `<GATEWAY>`, `<PASSWORD>` with your values.

### Why privileged?

Docker inside LXC needs kernel sysctl access (`vm.max_map_count`, `kernel.shmmax`, etc.). Unprivileged containers deny these with `sysctl: permission denied`. Even with `lxc.apparmor.profile: unconfined`, some parameters remain blocked.

## 2. Install Dependencies

```bash
pct exec <CTID> -- bash -c "\
  apt-get update -qq && \
  apt-get install -y -qq git docker.io nodejs npm curl ca-certificates"
```

### Docker Compose Plugin

`docker-compose-plugin` is not available in Debian 13 apt. Install manually:

```bash
pct exec <CTID> -- bash -c "\
  mkdir -p /usr/lib/docker/cli-plugins && \
  curl -sL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
    -o /usr/lib/docker/cli-plugins/docker-compose && \
  chmod +x /usr/lib/docker/cli-plugins/docker-compose"
```

### Docker Buildx

Required for `docker compose up --build`. Also not bundled in Debian 13:

```bash
pct exec <CTID> -- bash -c "\
  BUILDX_VER=\$(curl -sL https://api.github.com/repos/docker/buildx/releases/latest | grep tag_name | cut -d'\"' -f4) && \
  curl -sL https://github.com/docker/buildx/releases/download/\${BUILDX_VER}/buildx-\${BUILDX_VER}.linux-amd64 \
    -o /usr/lib/docker/cli-plugins/docker-buildx && \
  chmod +x /usr/lib/docker/cli-plugins/docker-buildx"
```

## 3. Clone and Start

```bash
pct exec <CTID> -- bash -c "\
  cd /root && \
  git clone https://github.com/veilkey/veilkey-selfhosted.git && \
  cd veilkey-selfhosted && \
  cp .env.example .env && \
  docker compose up -d"
```

First run builds all images (VaultCenter, LocalVault, veil CLI). This takes a few minutes.

## 4. Verify

```bash
# Check all 3 services are running
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose ps"

# Health check
pct exec <CTID> -- bash -c "curl -sk https://localhost:11181/health"
# Expected: {"status":"setup"}
```

## 5. Network Access

VeilKey listens on `https://<CT_IP>:11181` inside the internal network. To access from outside:

### Option A: Port forwarding on Proxmox host

```bash
# Forward host port to container
iptables -t nat -A PREROUTING -i vmbr0 -p tcp --dport 11181 \
  -j DNAT --to-destination <CT_IP>:11181
```

### Option B: Access from host network directly

If your client is on the same network as vmbr1, access `https://<CT_IP>:11181` directly.

## 6. Initial Setup (headless)

Proxmox LXC environments typically don't have browser access. Set up via CLI:

```bash
# Initial setup (first run — sets master + admin password)
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:11181/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<master_password>\",\"admin_password\":\"<admin_password>\"}'"

# If already initialized, unlock instead
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:11181/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<master_password>\"}'"
```

For full setup (LocalVault registration, secret storage), see [Post-Install Setup](../../docs/setup.md).

To add a standalone LocalVault, see [install-localvault.md](../common/install-localvault.md).

## Troubleshooting

### `sysctl: permission denied` during build/startup

Container is unprivileged. Recreate as privileged (`--unprivileged 0`).

### `compose build requires buildx 0.17.0 or later`

Install buildx manually (see step 2).

### `LOCALVAULT_CHAIN_PEERS` warning

Harmless. To suppress, add to `.env`:

```bash
LOCALVAULT_CHAIN_PEERS=
```

### Locale errors (PostgreSQL `initdb` failures)

If running services that need locale:

```bash
pct exec <CTID> -- bash -c "\
  apt-get install -y locales && \
  localedef -i en_US -f UTF-8 en_US.UTF-8 && \
  echo 'LANG=en_US.UTF-8' > /etc/default/locale"
```
