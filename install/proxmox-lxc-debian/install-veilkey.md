# Proxmox LXC Installation (Debian)

Run VeilKey Self-Hosted inside a Proxmox LXC container.

> **Tested on:** Proxmox VE 8.x, Debian 13 (trixie) LXC, Docker 26+

## Quick Start (script)

Run from the Proxmox host:

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
CT_IP=<IP>/<MASK> CT_GW=<GATEWAY> bash install/proxmox-lxc-debian/install-veilkey.sh
```

The script creates a privileged LXC container, installs all dependencies, and starts VeilKey services. See [install-veilkey.sh](./install-veilkey.sh) for all options.

For manual step-by-step installation, continue below.

> **Note:** Commands below use `<VC_PORT>` and `<LV_PORT>` as port placeholders.
> Defaults: VaultCenter `11181`, LocalVault `11180`. Change in `.env` (`VAULTCENTER_HOST_PORT`, `LOCALVAULT_HOST_PORT`).

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
pveam download local <TEMPLATE>

# Create privileged container
pct create <CTID> local:vztmpl/<TEMPLATE> \
  --hostname <HOSTNAME> \
  --memory <MEMORY> \
  --cores <CORES> \
  --rootfs <STORAGE>:<DISK_GB> \
  --net0 name=eth0,bridge=<BRIDGE>,ip=<IP>/<MASK>,gw=<GATEWAY> \
  --password '<PASSWORD>' \
  --unprivileged 0 \
  --features nesting=1 \
  --start 1
```

| Placeholder | Default | Description |
|-------------|---------|-------------|
| `<CTID>` | next available | Container ID |
| `<TEMPLATE>` | `debian-13-standard_13.1-2_amd64.tar.zst` | LXC template |
| `<HOSTNAME>` | `veilkey` | Container hostname |
| `<MEMORY>` | `2048` | Memory in MB |
| `<CORES>` | `2` | CPU cores |
| `<STORAGE>` | `local-lvm` | Storage backend |
| `<DISK_GB>` | `16` | Disk size in GB |
| `<BRIDGE>` | `vmbr1` | Network bridge |

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
pct exec <CTID> -- bash -c "curl -sk https://localhost:<VC_PORT>/health"
# Expected: {"status":"setup"}
```

## 5. Network Access

VeilKey listens on `https://<CT_IP>:<VC_PORT>` inside the internal network. To access from outside:

### Option A: Port forwarding on Proxmox host

```bash
# Forward host port to container (adjust interface and port)
iptables -t nat -A PREROUTING -i <WAN_BRIDGE> -p tcp --dport <VC_PORT> \
  -j DNAT --to-destination <CT_IP>:<VC_PORT>
```

### Option B: Access from host network directly

If your client is on the same network as `<BRIDGE>`, access `https://<CT_IP>:<VC_PORT>` directly.

## 6. Initial Setup (headless)

Proxmox LXC environments typically don't have browser access. Set up entirely via CLI.

### VaultCenter unlock

```bash
# If auto-setup completed (status: "locked"), unlock:
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\"}'"

# If first run (status: "setup"), initialize:
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\",\"admin_password\":\"<ADMIN_PASSWORD>\"}'"
```

### LocalVault registration

Docker internal network is trusted — no registration token needed:

```bash
# Init LocalVault
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec localvault sh -c \
    'echo \"<MASTER_PASSWORD>\" | veilkey-localvault init --root --center https://vaultcenter:10181'"

# Restart + unlock
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose restart localvault"
sleep 3
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<LV_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\"}'"
```

### Verify both services

```bash
pct exec <CTID> -- bash -c "curl -sk https://localhost:<VC_PORT>/health && echo '' && curl -sk https://localhost:<LV_PORT>/health"
# Expected: {"status":"ok"} for both
```

### Quick secret test

```bash
# Admin login
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/admin/login \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<ADMIN_PASSWORD>\"}' -c /tmp/vk.txt"

# Create temp secret
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/keycenter/temp-refs \
  -H 'Content-Type: application/json' -b /tmp/vk.txt \
  -d '{\"name\":\"TEST\",\"value\":\"hello-veilkey\"}'"
# Note the ref: VK:TEMP:xxxxxxxx

# Get agent hash
pct exec <CTID> -- bash -c "curl -sk https://localhost:<VC_PORT>/api/agents | grep -o '\"agent_hash\":\"[^\"]*\"'"

# Promote to vault (replace ref and agent_hash)
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/keycenter/promote \
  -H 'Content-Type: application/json' -b /tmp/vk.txt \
  -d '{\"ref\":\"VK:TEMP:xxxxxxxx\",\"name\":\"TEST\",\"vault_hash\":\"<AGENT_HASH>\"}'"
# Note the token: VK:LOCAL:yyyyyyyy

# Resolve
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec veil veilkey-cli resolve VK:LOCAL:yyyyyyyy"
# Expected: hello-veilkey

# PTY masking test
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec veil veilkey-cli wrap-pty sh -c 'echo hello-veilkey'"
# Expected: VK:LOCAL:yyyyyyyy (masked!)
```

For full setup details, see [Post-Install Setup](../../docs/setup.md).

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
