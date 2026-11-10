# LocalVault — Proxmox LXC (Debian)

## Docker Compose (inside VeilKey LXC)

Issue a registration token in VaultCenter first. It is required even on the trusted Docker network.

```bash
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec -T localvault veilkey-localvault init --root \
    --token '<REG_TOKEN>' --center https://vaultcenter:10181"

pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose restart localvault"
```

The unlock key is generated automatically; bootstrap and subsequent restarts use VC-managed unlock. See [registration.md](./registration.md).

## Standalone (on any LXC or host)

For first-time installation, use the token-aware common installer:

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN='<REG_TOKEN>' \
VEILKEY_LABEL=<VAULT_NAME> \
  bash install/common/install-localvault.sh
```

See [the common installer guide](../../../install/common/install-localvault.md). The legacy Proxmox installer does not pass a registration token and cannot initialize a new vault until its init invocation supplies `--token`. The management notes below describe existing installations using that legacy installer.

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
