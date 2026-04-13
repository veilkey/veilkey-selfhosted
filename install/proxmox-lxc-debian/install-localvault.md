# Standalone LocalVault Installation

Install a standalone LocalVault on the Proxmox host or any LXC, connecting to an existing VaultCenter.

## Prerequisites

- Go: `apt install golang`
- Rust (optional, for veilkey CLI): `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- openssl (for TLS certificate generation)
- VaultCenter running and unlocked

## Install

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=http://<HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

The script handles: source update, build, TLS cert generation, init, start, systemd registration, auto-unlock, daily backup, and veilkey CLI installation.

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | - | VaultCenter URL (required) |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_NAME` | `$(hostname)` | Vault display name |
| `VEILKEY_PASSWORD` | - | Master password (prompted if not set) |
| `VEILKEY_BULK_APPLY_ALLOWED_PATHS` | - | Comma-separated absolute paths for bulk-apply targets |

## What it does

| Step | First run | Re-run (update) |
|------|-----------|-----------------|
| Source update | - | `git pull` |
| Build LocalVault | Go build | Rebuild with latest |
| Build veilkey CLI | Rust build (if available) | Rebuild |
| TLS certificate | Auto-generate (self-signed, 10yr) | Preserved |
| .env config | Created (with `::1`, auto-unlock) | Preserved (missing settings added) |
| Init | Password -> KEK -> salt | Skipped (salt exists) |
| systemd service | Created + enabled | Restarted |
| Backup cron | Daily 04:00, 7-day retention | Skipped if exists |
| Health check | Verify unlocked + connected | Verify |

## Auto-unlock

The installer sets `VEILKEY_UNLOCK_PASSWORD` in `.env`. On reboot, the LocalVault automatically unlocks via VaultCenter without manual intervention.

## Trusted IPs

Default: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1,::1`

`::1` (IPv6 loopback) is included to prevent unlock failures when curl uses IPv6.

## After install

The vault auto-registers with VaultCenter via heartbeat:

```bash
curl -sk <VC_URL>/api/agents
```

Use the veilkey CLI:

```bash
export VEILKEY_LOCALVAULT_URL=https://<LV_IP>:<PORT>
export VEILKEY_TLS_INSECURE=1
veilkey status
veilkey scan /path/to/file
```

## Management

```bash
# Service
systemctl status veilkey-localvault
systemctl restart veilkey-localvault
systemctl stop veilkey-localvault

# Logs
journalctl -u veilkey-localvault -f

# Backups
ls -la .localvault/data/backups/

# Update (re-run -- pulls latest, rebuilds, restarts)
VEILKEY_CENTER_URL=http://<HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

## Uninstall

```bash
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```

Removes: systemd service, backup cron, optionally data directory and CLI binaries.
