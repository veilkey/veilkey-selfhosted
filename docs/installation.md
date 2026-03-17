# Installation

VeilKey supports multiple installation targets. Choose the one that matches your environment.

## Requirements

- **Go 1.24+** (auto-installed on macOS via Homebrew)
- **SQLite** (bundled via CGO)
- **curl**, **openssl** (for fresh installs)
- **Linux with eBPF** (proxy only, not required for core functionality)

## macOS

The Mac installer builds all components from source and runs KeyCenter + LocalVault as launchd services.

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
./installer/scripts/install-mac.sh install
./installer/scripts/install-mac.sh start
```

This will:
1. Install Go via Homebrew (if missing)
2. Build all binaries (keycenter, localvault, proxy, session-config, cli)
3. Create directories, config files, and auto-generated passwords
4. Initialize KeyCenter as HKM root node
5. Initialize LocalVault and connect to KeyCenter
6. Register launchd services
7. Add shell aliases to `~/.zshrc`

### Mac Commands

```bash
./installer/scripts/install-mac.sh status     # service status
./installer/scripts/install-mac.sh health      # HTTP health check
./installer/scripts/install-mac.sh verify      # full installation verify
./installer/scripts/install-mac.sh logs        # view logs
./installer/scripts/install-mac.sh stop        # stop services
./installer/scripts/install-mac.sh restart     # restart services
./installer/scripts/install-mac.sh uninstall   # remove everything
```

### Mac File Layout

| Path | Purpose |
|------|---------|
| `/usr/local/bin/veilkey-*` | Binaries |
| `/usr/local/etc/veilkey/` | Config and password files |
| `/usr/local/var/veilkey/` | Database and runtime data |
| `/usr/local/var/log/veilkey/` | Logs |
| `~/Library/LaunchAgents/net.veilkey.*.plist` | launchd services |
| `~/.veilkey.sh` | Shell aliases and environment |

### Mac Shell Aliases

After installation, open a new terminal or `source ~/.zshrc`:

| Alias | Command |
|-------|---------|
| `vk` | `veilkey-cli` |
| `vks` | `veilkey-cli scan` |
| `vkf` | `veilkey-cli filter` |
| `vkw` | `veilkey-cli wrap` |
| `vke` | `veilkey-cli exec` |
| `vkr` | `veilkey-cli resolve` |
| `vk-start` | Start services |
| `vk-stop` | Stop services |
| `vk-status` | Health check |
| `vk-logs` | View logs |

## Proxmox LXC — All-in-One

Deploys KeyCenter + LocalVault inside a single LXC container.

### Prerequisites

- Proxmox VE host
- Debian-based LXC container
- Network access to artifact source (or pre-bundled)

### Install

```bash
cd installer
export VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4"
./install.sh init

# Set passwords
echo -n 'your-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
echo -n 'your-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password

# Install and activate
./scripts/proxmox-lxc-allinone-install.sh --activate /

# Verify
./scripts/proxmox-lxc-allinone-health.sh /
curl http://localhost:10181/health
curl http://localhost:10180/health
```

### Verify Agent Registration

```bash
# Unlock KeyCenter
curl -X POST http://127.0.0.1:10181/api/unlock \
  -H 'Content-Type: application/json' \
  --data '{"password":"your-keycenter-password"}'

# Check agents
curl http://127.0.0.1:10181/api/agents
```

### Ports

| Service | Port |
|---------|------|
| KeyCenter | 10181 |
| LocalVault | 10180 |

## Proxmox LXC — Runtime Only

Deploys LocalVault only, connected to an existing KeyCenter.

```bash
echo -n 'your-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password

VEILKEY_KEYCENTER_URL='http://<keycenter-ip>:10181' \
  ./scripts/proxmox-lxc-runtime-install.sh --activate /

./scripts/proxmox-lxc-runtime-health.sh /
```

## Proxmox Host — LocalVault

Deploys LocalVault directly on the Proxmox host (not in a container).

```bash
echo -n 'your-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password

export VEILKEY_KEYCENTER_URL='https://<keycenter-host>'
./scripts/proxmox-host-localvault/install.sh --activate /
./scripts/proxmox-host-localvault/health.sh /
```

### Purge

```bash
./scripts/proxmox-host-localvault/purge.sh /
```

## Proxmox Host — Boundary (Proxy + CLI)

Deploys the proxy enforcement layer on the Proxmox host. This is the companion to an all-in-one LXC.

```bash
./scripts/proxmox-host-cli-install.sh /
```

Or use the combined stack installer:

```bash
./scripts/proxmox-allinone-stack-install.sh / / \
  "${VEILKEY_ALLINONE_BUNDLE_ROOT}" \
  "${VEILKEY_HOST_CLI_BUNDLE_ROOT}"
```

## Install Profiles

| Profile | Components | Use Case |
|---------|-----------|----------|
| `proxmox-lxc-allinone` | KeyCenter + LocalVault | Single-node control plane |
| `proxmox-lxc-runtime` | LocalVault | Additional node |
| `proxmox-host-localvault` | LocalVault | Host-side agent |
| `proxmox-host-cli` | Proxy + CLI | Host boundary enforcement |
| `proxmox-host` | Proxy | Host proxy assets only |

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `VEILKEY_PASSWORD_FILE` | Path to KEK password file | — |
| `VEILKEY_DB_PATH` | SQLite database path | `/opt/veilkey/*/data/veilkey.db` |
| `VEILKEY_ADDR` | Bind address | `:10180` |
| `VEILKEY_KEYCENTER_URL` | KeyCenter endpoint (for LocalVault) | — |
| `VEILKEY_TRUSTED_IPS` | Comma-separated CIDRs for write access | — |
| `VEILKEY_TLS_CERT` | TLS certificate path | — |
| `VEILKEY_TLS_KEY` | TLS private key path | — |
| `VEILKEY_LOCALVAULT_URL` | LocalVault endpoint (for CLI) | — |

## Troubleshooting

### "Salt file not found"

KeyCenter needs initialization:

```bash
cat /path/to/password | VEILKEY_DB_PATH=/path/to/veilkey.db veilkey-keycenter init --root
```

### LocalVault shows "setup" status

LocalVault hasn't been initialized yet. Visit `http://localhost:10180` for the setup wizard, or call:

```bash
curl -X POST http://127.0.0.1:10180/api/install/init \
  -H 'Content-Type: application/json' \
  -d '{"password":"your-password","keycenter_url":"http://127.0.0.1:10181"}'
```

### Permission denied on password file

Password files must be readable by the service user:

```bash
sudo chown $(whoami) /usr/local/etc/veilkey/*.password
chmod 600 /usr/local/etc/veilkey/*.password
```

### Agent not appearing in KeyCenter

Restart LocalVault to force a heartbeat:

```bash
# macOS
./installer/scripts/install-mac.sh restart

# systemd
systemctl restart veilkey-localvault
```
