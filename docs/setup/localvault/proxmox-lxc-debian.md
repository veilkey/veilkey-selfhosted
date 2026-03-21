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

Use the install script — handles build, TLS, init, start, unlock:

```bash
cd veilkey-selfhosted
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_PASSWORD='<MASTER_PASSWORD>' \
VEILKEY_NAME=<VAULT_NAME> \
VEILKEY_BULK_APPLY_ALLOWED_PATHS=<PATHS> \
  bash install/proxmox-lxc-debian/install-localvault.sh
```

See [install-localvault.md](../../install/proxmox-lxc-debian/install-localvault.md) for details.
