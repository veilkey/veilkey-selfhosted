# LocalVault — Proxmox LXC (Debian)

## Docker Compose (inside VeilKey LXC)

```bash
pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && \
  docker compose exec -T localvault sh -c \
    'VEILKEY_DB_PATH=/data/veilkey.db veilkey-localvault init --root --center https://vaultcenter:10181'"

pct exec <CTID> -- bash -c "cd /root/veilkey-selfhosted && docker compose restart localvault"

pct exec <CTID> -- bash -c "curl -sk https://localhost:<LV_PORT>/health"
```

## Standalone (on any LXC or host)

Use the install script — handles build, TLS, init, start, bootstrap auto-unlock, and health check:

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_LABEL=<VAULT_NAME> \
VEILKEY_BULK_APPLY_ALLOWED_PATHS=<PATHS> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

See [install-localvault.md](../../../install/proxmox-lxc-debian/install-localvault.md) for details.
