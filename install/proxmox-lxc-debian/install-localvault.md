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
VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN="<REGISTRATION_TOKEN>" \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

The script handles: source update, build, TLS cert generation, token registration, start, systemd registration, VaultCenter-managed unlock, daily backup, and veilkey CLI installation.

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | - | VaultCenter URL (required) |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_NAME` | `$(hostname)` | Vault display name |
| `VEILKEY_REG_TOKEN` | - | Registration token (required on first initialization) |
| `SSL_CERT_FILE` | system roots | Verified CA PEM bundle, persisted for the service |
| `VEILKEY_TRUSTED_IPS` | resolved VaultCenter address only | Exact source address; no private-range or loopback defaults |
| `VEILKEY_BULK_APPLY_ALLOWED_PATHS` | - | Comma-separated absolute paths for bulk-apply targets |

## What it does

| Step | First run | Re-run (update) |
|------|-----------|-----------------|
| Source update | - | `git pull` |
| Build LocalVault | Go build | Rebuild with latest |
| Build veilkey CLI | Rust build (if available) | Rebuild |
| TLS certificate | Auto-generate (self-signed, 10yr) | Preserved |
| .env config | Created without passwords, TLS verification enabled | Legacy password entries removed; trust settings narrowed |
| Init | Registration token + auto-generated VC-managed unlock key | Skipped (salt exists) |
| systemd service | Created + enabled | Restarted |
| Backup cron | Daily 04:00, 7-day retention | Skipped if exists |
| Health check | Verify unlocked + connected | Verify |

## VaultCenter-managed unlock

LocalVault generates its unlock key during initialization and registers with the supplied token. No master password is prompted for or written to `.env`, another file, or a RAM disk. Re-running the installer removes legacy password entries without making a backup copy. Remove any older plaintext backups separately. After restart, unlock VaultCenter interactively; LocalVault retrieves its managed key from VaultCenter.

## Trusted IPs and TLS

Only the resolved VaultCenter address is trusted by default. Private-network ranges and loopback are not added. An explicit `VEILKEY_TRUSTED_IPS` override should name only VaultCenter's exact source addresses. Trust does not replace the registration token or agent authentication.

TLS verification is enabled for initialization and service traffic. Provision the [verified CA bundle](../../docs/setup/tls.md) before running the installer. Local health checks pin the service's generated certificate with `--cacert`.

## After install

The vault auto-registers with VaultCenter via heartbeat:

```bash
curl -sS <VC_URL>/api/agents
```

Use the veilkey CLI:

```bash
export VEILKEY_LOCALVAULT_URL=https://<LV_IP>:<PORT>
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem
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
VEILKEY_CENTER_URL=https://<HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN="<REGISTRATION_TOKEN>" \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

## Uninstall

```bash
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```

Removes: systemd service, backup cron, optionally data directory and CLI binaries.
