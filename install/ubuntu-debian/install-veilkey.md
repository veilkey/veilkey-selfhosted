# Ubuntu / Debian Installation

Run VeilKey Self-Hosted directly on an Ubuntu or Debian server using Docker.

> **Tested on:** Ubuntu 22.04 LTS, Ubuntu 24.04 LTS, Debian 12 (bookworm), Docker 26+

## Quick Start (script)

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
sudo bash install/ubuntu-debian/install-veilkey.sh
```

The script installs Docker, clones the repository, and starts all VeilKey services automatically.
See [install-veilkey.sh](./install-veilkey.sh) for all available options.

> **Note:** Commands below use `<VC_PORT>` and `<LV_PORT>` as port placeholders.
> Defaults: VaultCenter `11181`, LocalVault `11180`. Change in `.env` (`VAULTCENTER_HOST_PORT`, `LOCALVAULT_HOST_PORT`).

---

## Requirements

| Resource | Minimum | Recommended |
|----------|---------|-------------|
| RAM | 1 GB | 2 GB |
| Disk | 8 GB | 16 GB |
| OS | Ubuntu 22.04+ / Debian 12+ | — |

## 1. Install Docker

Skip this step if Docker is already installed.

```bash
# Add Docker's official GPG key
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -m 0755 -d /etc/apt/keyrings
sudo curl -fsSL https://download.docker.com/linux/ubuntu/gpg \
    -o /etc/apt/keyrings/docker.asc
sudo chmod a+r /etc/apt/keyrings/docker.asc

# Add Docker apt repository
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.asc] \
  https://download.docker.com/linux/ubuntu \
  $(. /etc/os-release && echo "$VERSION_CODENAME") stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker Engine + Compose plugin
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io \
    docker-buildx-plugin docker-compose-plugin

sudo systemctl enable --now docker
```

## 2. Clone and Configure

```bash
sudo git clone https://github.com/veilkey/veilkey-selfhosted.git /opt/veilkey
cd /opt/veilkey
sudo cp .env.example .env
```

Edit `.env` to customize ports or other settings:

```bash
sudo nano /opt/veilkey/.env
```

## 3. Start Services

```bash
cd /opt/veilkey
sudo docker compose up -d
```

First run builds all images (VaultCenter, LocalVault, veil CLI). This takes a few minutes.

## 4. Verify

```bash
# Check all 3 services are running
sudo docker compose -f /opt/veilkey/docker-compose.yml ps

# Health check
curl -sk https://localhost:<VC_PORT>/health
# Expected: {"status":"setup"}
```

## 5. Initial Setup

### VaultCenter initialization

```bash
# First run (status: "setup")
curl -sk -X POST https://localhost:<VC_PORT>/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>","admin_password":"<ADMIN_PASSWORD>"}'

# Subsequent unlocks (status: "locked")
curl -sk -X POST https://localhost:<VC_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### LocalVault registration

```bash
# Init LocalVault
cd /opt/veilkey
sudo docker compose exec -T localvault sh -c \
  'echo "<MASTER_PASSWORD>" | veilkey-localvault init --root --center https://vaultcenter:10181'

# Restart and unlock
sudo docker compose restart localvault
sleep 3
curl -sk -X POST https://localhost:<LV_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### Verify both services

```bash
curl -sk https://localhost:<VC_PORT>/health && echo '' && curl -sk https://localhost:<LV_PORT>/health
# Expected: {"status":"ok"} for both
```

## Management Commands

```bash
cd /opt/veilkey
sudo docker compose ps          # 상태 확인
sudo docker compose logs -f     # 로그 보기 (실시간)
sudo docker compose down        # 서비스 중지
sudo docker compose pull && sudo docker compose up -d   # 업데이트
```

## Troubleshooting

### Port already in use

Change `VAULTCENTER_HOST_PORT` / `LOCALVAULT_HOST_PORT` in `.env` and restart:

```bash
sudo docker compose down && sudo docker compose up -d
```

### `LOCALVAULT_CHAIN_PEERS` warning

Harmless. To suppress, add to `.env`:

```bash
LOCALVAULT_CHAIN_PEERS=
```

### Services not starting after reboot

Enable Docker to start on boot and bring services up automatically:

```bash
sudo systemctl enable docker

# Add restart policy to docker-compose.yml or run:
sudo docker compose -f /opt/veilkey/docker-compose.yml up -d
```

For full setup details, see [Post-Install Setup](../../docs/setup/README.md).

To add a standalone LocalVault on another server, see [install-localvault.md](../common/install-localvault.md).
