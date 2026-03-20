# Proxmox Host — Standalone LocalVault Installation

Install a standalone LocalVault on the Proxmox host, connecting to an existing VaultCenter (e.g. running in LXC).

## Prerequisites

- Go: `apt install golang`
- VaultCenter running and unlocked

## Install

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<CT_IP>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

The script handles everything: build, init, start, and unlock.

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | - | VaultCenter URL (required) |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_NAME` | `$(hostname)` | Vault display name |
| `VEILKEY_PASSWORD` | - | Master password (prompted if not set) |

## After install

The vault auto-registers with VaultCenter via heartbeat. Check the keycenter UI or API:

```bash
curl -sk <VC_URL>/api/agents
```

## Management

```bash
# Logs
tail -f .localvault/localvault.log

# Stop
kill $(cat .localvault/localvault.pid)

# Restart (re-run installer — it skips init if already initialized)
VEILKEY_CENTER_URL=https://<CT_IP>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

## Uninstall

```bash
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```

See [uninstall-localvault.sh](./uninstall-localvault.sh) for details.
