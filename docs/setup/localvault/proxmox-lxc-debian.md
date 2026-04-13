# LocalVault — Proxmox LXC (Debian)

## Docker Compose (inside VeilKey LXC)

```bash
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec -T localvault sh -c \
    'echo \"<MASTER_PASSWORD>\" | veilkey-localvault init --root --center https://vaultcenter:10181'"

pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose restart localvault"

pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<LV_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\"}'"
```

## Standalone (on any LXC or host)

Use the install script — handles build, TLS, init, systemd, auto-unlock, backup, and CLI:

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=http://<VC_HOST>:<VC_PORT> \
VEILKEY_PASSWORD='<MASTER_PASSWORD>' \
VEILKEY_NAME=<VAULT_NAME> \
VEILKEY_BULK_APPLY_ALLOWED_PATHS=<PATHS> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

See [install-localvault.md](../../../install/proxmox-lxc-debian/install-localvault.md) for details.

### What the installer sets up

| Component | Detail |
|-----------|--------|
| **systemd service** | `veilkey-localvault.service`, auto-start on boot |
| **Auto-unlock** | `VEILKEY_UNLOCK_PASSWORD` in `.env`, unlocks via VaultCenter on restart |
| **Trusted IPs** | Includes `::1` (IPv6 loopback) to prevent localhost unlock failures |
| **Daily backup** | cron 04:00, db + salt, 7-day retention |
| **veilkey CLI** | Built and installed to `/usr/local/bin/veilkey` (requires Rust) |

### Management

```bash
# Service control
systemctl status veilkey-localvault
systemctl restart veilkey-localvault
journalctl -u veilkey-localvault -f

# CLI
export VEILKEY_LOCALVAULT_URL=https://<LV_IP>:10180
export VEILKEY_TLS_INSECURE=1
veilkey status
veilkey scan /path/to/file

# Backups
ls .localvault/data/backups/

# Update (re-run the installer)
VEILKEY_CENTER_URL=http://<VC_HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh

# Uninstall
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```
