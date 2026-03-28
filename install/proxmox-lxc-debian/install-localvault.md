# Standalone LocalVault Installation

Install a standalone LocalVault on the Proxmox host or any LXC, connecting to an existing VaultCenter.

## Prerequisites

- Go: `apt install golang`
- openssl (for TLS certificate generation)
- VaultCenter running and unlocked

## Install

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

The script handles: source update, build, TLS cert generation, `init`, start, bootstrap auto-unlock, and health check.

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | - | VaultCenter URL (required) |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_LABEL` | `$(hostname)` | Vault display name |
| `VEILKEY_BULK_APPLY_ALLOWED_PATHS` | - | Comma-separated absolute paths for bulk-apply targets |

## What it does

| Step | First run | Re-run (update) |
|------|-----------|-----------------|
| Source update | - | `git pull` |
| Build | Go build | Rebuild with latest |
| TLS certificate | Auto-generate (self-signed, 10yr) | Preserved |
| .env config | Rewritten from current env | Rewritten from current env |
| Init | Auto-generated vault unlock key → salt/DB | Skipped (salt exists) |
| Start | HTTPS start + bootstrap auto-unlock | Restart + VC-managed auto-unlock |

## After install

The vault auto-registers with VaultCenter via heartbeat:

```bash
curl -sk <VC_URL>/api/agents
```

## Management

```bash
# Logs
tail -f .localvault/localvault.log

# Stop
kill $(cat .localvault/localvault.pid)

# Update (re-run — pulls latest, rebuilds, restarts)
VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

## Uninstall

```bash
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```
