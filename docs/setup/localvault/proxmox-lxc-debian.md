# LocalVault — Proxmox LXC (Debian)

## Docker Compose (inside VeilKey LXC)

Obtain a registration token from VaultCenter, including for Docker-internal registration:

```bash
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec -T localvault veilkey-localvault init --root \
    --token '<REGISTRATION_TOKEN>' --center https://vaultcenter:10181"
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose restart localvault"
```

LocalVault uses a generated VaultCenter-managed unlock key. Unlock VaultCenter interactively after restart; never save a master password in `.env`.

## Standalone (on any LXC or host)

Use the install script — handles build, TLS, init, systemd, VaultCenter-managed unlock, backup, and CLI:

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN='<REGISTRATION_TOKEN>' \
VEILKEY_NAME=<VAULT_NAME> \
VEILKEY_BULK_APPLY_ALLOWED_PATHS=<PATHS> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

See [install-localvault.md](../../../install/proxmox-lxc-debian/install-localvault.md) for details.

### What the installer sets up

| Component | Detail |
|-----------|--------|
| **systemd service** | `veilkey-localvault.service`, auto-start on boot |
| **Managed unlock** | Auto-generated key managed by VaultCenter; no master password files. Unlock VaultCenter after restart |
| **Trusted IPs** | Only the resolved VaultCenter source address; no private ranges or loopback |
| **TLS** | Verification enabled; provision [CA trust](../tls.md) before initialization |
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
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem
veilkey status
veilkey scan /path/to/file

# Backups
ls .localvault/data/backups/

# Update (re-run the installer)
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
  bash install/proxmox-lxc-debian/install-localvault.sh

# Uninstall
bash install/proxmox-lxc-debian/uninstall-localvault.sh
```
