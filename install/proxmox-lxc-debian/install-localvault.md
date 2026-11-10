# Standalone LocalVault Installation

Install a standalone LocalVault on the Proxmox host or any LXC, connecting to an existing VaultCenter.

## Prerequisites

- Go: `apt install golang`
- Rust (optional, for veilkey CLI): `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- openssl (for TLS certificate generation)
- VaultCenter running and unlocked
- A valid, unused registration token from the VaultCenter keycenter UI (required even on trusted networks)

## Install

For a new vault, use [the token-aware common installer](../common/install-localvault.md):

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN='<REG_TOKEN>' \
  bash install/common/install-localvault.sh
```

The legacy `install/proxmox-lxc-debian/install-localvault.sh` invokes `init` without `--token`; it cannot initialize a new vault until that call is updated. The options and management notes below describe existing installations created by the legacy script. Do not reinitialize an existing registered vault to add a token.

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
| Init | Requires `init --root --token <REG_TOKEN>` with a valid registration token | Skipped (salt exists) |
| systemd service | Created + enabled | Restarted |
| Backup cron | Daily 04:00, 7-day retention | Skipped if exists |
| Health check | Verify unlocked + connected | Verify |

## Auto-unlock

The installer sets `VEILKEY_UNLOCK_PASSWORD` in `.env`. On reboot, the LocalVault automatically unlocks via VaultCenter without manual intervention.

## Trusted IPs

Default: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1,::1`

`::1` (IPv6 loopback) is included to prevent unlock failures when curl uses IPv6.

## After install

A new vault registers on its first heartbeat only with its saved valid registration token. VaultCenter consumes the token once; later heartbeats from that registered vault do not require another token:

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
