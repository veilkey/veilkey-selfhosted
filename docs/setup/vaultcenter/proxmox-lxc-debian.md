# VaultCenter — Proxmox LXC (Debian)

All operations from [setup.md](./setup.md) wrapped with `pct exec`:

```bash
# Setup
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\",\"admin_password\":\"<ADMIN_PASSWORD>\"}'"

# Unlock
pct exec <CTID> -- bash -c "curl -sk -X POST https://localhost:<VC_PORT>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{\"password\":\"<MASTER_PASSWORD>\"}'"

# Verify
pct exec <CTID> -- bash -c "curl -sk https://localhost:<VC_PORT>/health"
```
