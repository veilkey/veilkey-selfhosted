# Post-Install Setup

Installation complete? Follow these steps to initialize VeilKey.

This guide is common to all platforms. For platform-specific installation, see [install/](../install/).

## 1. VaultCenter Setup

Open `https://<your-host>:11181` in your browser.

- Enter **master password** (KEK derivation — remember this)
- Enter **admin password** (web UI login)
- Setup complete — server starts in LOCKED mode on restart

> On restart, you must re-enter the master password.
> Optional: set `VEILKEY_PASSWORD_FILE` for auto-unlock (securing this file is your responsibility).

## 2. LocalVault Registration

In the keycenter UI (`https://<your-host>:11181/keycenter`):

1. Click "+ 등록 토큰" to issue a registration token
2. Run inside the localvault container:

```bash
docker compose exec localvault sh -c \
  "echo 'your-password' | veilkey-localvault init --root \
    --token vk_reg_xxx \
    --center https://vaultcenter:10181"
docker compose restart localvault
```

3. LocalVault appears in the vault list after heartbeat

## 3. Store Secrets

In the keycenter UI:

1. "+ 임시키" — enter name and value
2. Select the key — "볼트에 저장 (격상)" — select vault
3. Secret is now encrypted with agentDEK and stored in LocalVault

## 4. Use veil CLI

```bash
# Docker Compose environment
docker compose exec -it \
  -e DB_PASSWORD=VK:LOCAL:xxxx \
  veil veilkey-cli wrap-pty bash

# Native CLI (macOS)
cd veilkey-selfhosted && veil
```

Inside the veil shell, `echo $DB_PASSWORD` shows `VK:LOCAL:xxxx` (masked), but actual processes receive the real value.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_ADDR` | `:10181` / `:10180` | Listen address |
| `VEILKEY_DB_PATH` | `/data/veilkey.db` | Database path |
| `VEILKEY_TLS_INSECURE` | `0` | Accept self-signed certs |
| `VEILKEY_CHAIN_HOME` | `/data/chain` | CometBFT data directory |
| `VEILKEY_TEMP_REF_TTL` | `1h` | Temp ref expiry |
| `VEILKEY_ADMIN_SESSION_TTL` | `2h` | Admin session duration |
| `VEILKEY_TRUSTED_IPS` | - | Trusted IP ranges (CIDR) |
| `VEILKEY_PASSWORD_FILE` | - | Auto-unlock password file path |
