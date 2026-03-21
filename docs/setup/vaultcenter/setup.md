# VaultCenter — Setup & Unlock

## Initial setup

### Web UI

Open `https://<HOST>:<VC_PORT>` in your browser.

- Enter **master password** (KEK derivation — remember this)
- Enter **admin password** (web UI login)
- Setup complete — server starts in LOCKED mode on restart

### CLI (headless)

```bash
curl -sk -X POST <VC_URL>/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>","admin_password":"<ADMIN_PASSWORD>"}'
```

### Auto-setup

Set `VAULTCENTER_AUTO_COMPLETE_INSTALL_FLOW=1` in `.env`. The server transitions directly to LOCKED state — unlock with the master password.

## Unlock

On restart, VaultCenter enters LOCKED mode.

### Web UI

Enter master password on the lock screen.

### CLI

```bash
curl -sk -X POST <VC_URL>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### Auto-unlock

Set `VEILKEY_PASSWORD_FILE` in `.env` (securing this file is your responsibility).

## Verify

```bash
curl -sk <VC_URL>/health
# Expected: {"status":"ok"}
```
