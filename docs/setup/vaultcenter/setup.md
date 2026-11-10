# VaultCenter — Setup & Unlock

## Initial setup

### Web UI

Open `https://<HOST>:<VC_PORT>` in your browser.

- Enter **master password** (KEK derivation — remember this)
- Enter **admin password** (web UI login)
- Setup complete — server starts in LOCKED mode on restart

### CLI (headless)

```bash
curl -sS -X POST <VC_URL>/api/setup/init \
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
curl -sS -X POST <VC_URL>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### Restart behavior

Unlock VaultCenter interactively through the web UI or TUI after restart. Master passwords must remain in memory: do not put them in `.env`, launchd/systemd configuration, password files, or RAM-disk files. The old password-file helper is disabled. LocalVault uses VaultCenter-managed unlock after VaultCenter itself is unlocked.

Provision [CA trust](../tls.md) before using the HTTPS examples; never bypass verification when submitting passwords.

## Verify

```bash
curl -sS <VC_URL>/health
# Expected: {"status":"ok"}
```
