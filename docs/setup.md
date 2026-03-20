# Post-Install Setup

Installation complete? Follow these steps to initialize VeilKey.

This guide is common to all platforms. For platform-specific installation, see [install/](../install/).

## Conventions

This guide uses the following placeholders:

| Placeholder | Default | Description |
|-------------|---------|-------------|
| `<vc_url>` | `<vc_url>` | VaultCenter URL |
| `<lv_url>` | `<lv_url>` | LocalVault URL |
| `<master_password>` | - | Master password (KEK derivation) |
| `<admin_password>` | - | Admin password (web UI login) |
| `<agent_hash>` | - | Vault agent hash (from `/api/agents`) |

Ports depend on your `.env` settings (`VAULTCENTER_HOST_PORT`, `LOCALVAULT_HOST_PORT`).

## 1. VaultCenter Setup

### Web UI

Open `https://<your-host>:<vc_port>` in your browser.

- Enter **master password** (KEK derivation — remember this)
- Enter **admin password** (web UI login)
- Setup complete — server starts in LOCKED mode on restart

### CLI (headless)

If you don't have browser access (e.g. SSH-only server):

```bash
# Initial setup (first run only)
curl -sk -X POST <vc_url>/api/setup/init \
  -H 'Content-Type: application/json' \
  -d '{"password":"<master_password>","admin_password":"<admin_password>"}'

# Unlock after restart
curl -sk -X POST <vc_url>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<master_password>"}'
```

### Auto-setup

The docker-entrypoint may auto-complete the initial setup if `VAULTCENTER_AUTO_COMPLETE_INSTALL_FLOW=1` is set in `.env`. In this case, the server transitions directly to LOCKED state — unlock with the master password.

### After restart

On restart, VaultCenter enters LOCKED mode. You must unlock it:
- Web UI: enter master password on the lock screen
- CLI: `POST /api/unlock` (see above)
- Auto-unlock: set `VEILKEY_PASSWORD_FILE` in `.env` (securing this file is your responsibility)

## 2. LocalVault Registration

### Web UI

In the keycenter UI (`https://<your-host>:<vc_port>/keycenter`):

1. Click "+ 등록 토큰" to issue a registration token
2. Run inside the localvault container:

```bash
docker compose exec localvault sh -c \
  "echo '<master_password>' | veilkey-localvault init --root \
    --token vk_reg_xxx \
    --center https://vaultcenter:10181"
docker compose restart localvault
```

3. LocalVault appears in the vault list after heartbeat

### CLI (headless / trusted IP)

If the LocalVault container is on the same Docker network as VaultCenter (default setup), registration tokens are not required — trusted IPs can register directly:

```bash
# Init LocalVault (uses master password from stdin)
docker compose exec localvault sh -c \
  "echo '<master_password>' | veilkey-localvault init --root \
    --center https://vaultcenter:10181"

# Restart to apply
docker compose restart localvault

# Wait a few seconds, then unlock
curl -sk -X POST <lv_url>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<master_password>"}'
```

### Verify

```bash
# Both should return {"status":"ok"}
curl -sk <vc_url>/health   # VaultCenter
curl -sk <lv_url>/health   # LocalVault
```

## 3. Store Secrets

### Web UI

In the keycenter UI:

1. "+ 임시키" — enter name and value
2. Select the key — "볼트에 저장 (격상)" — select vault
3. Secret is now encrypted with agentDEK and stored in LocalVault

### CLI (headless)

```bash
# Login as admin (get session cookie)
curl -sk -X POST <vc_url>/api/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"password":"<admin_password>"}' \
  -c /tmp/vk-cookies.txt

# Create temp ref
curl -sk -X POST <vc_url>/api/keycenter/temp-refs \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"name":"MY_SECRET","value":"actual-secret-value"}'
# Returns: {"ref":"VK:TEMP:xxxxxxxx", ...}

# List agents to get vault_hash
curl -sk <vc_url>/api/agents
# Returns: {"agents":[{"agent_hash":"xxxxxxxx", ...}]}

# Promote to vault (use full VK:TEMP:xxx ref and agent_hash as vault_hash)
curl -sk -X POST <vc_url>/api/keycenter/promote \
  -H 'Content-Type: application/json' \
  -b /tmp/vk-cookies.txt \
  -d '{"ref":"VK:TEMP:xxxxxxxx","name":"MY_SECRET","vault_hash":"<agent_hash>"}'
# Returns: {"token":"VK:LOCAL:yyyyyyyy", "status":"active", ...}
```

## 4. Use veil CLI

### Resolve a secret

```bash
docker compose exec veil veilkey-cli resolve VK:LOCAL:yyyyyyyy
# Output: actual-secret-value
```

### Execute with ref replacement

```bash
docker compose exec veil veilkey-cli exec echo VK:LOCAL:yyyyyyyy
# Output: actual-secret-value (ref replaced with real value in args)
```

### PTY masking session

```bash
# Docker Compose
docker compose exec veil veilkey-cli wrap-pty bash

# Native CLI (macOS)
cd veilkey-selfhosted && veil
```

Inside the veil shell:
- Real secrets in output are replaced with `VK:LOCAL:xxx` — AI cannot see them
- Processes receive the actual value — your app works normally
- `echo $MY_SECRET` shows `VK:LOCAL:yyyyyyyy` on screen, but the process sees `actual-secret-value`

### Verify masking

```bash
docker compose exec veil veilkey-cli wrap-pty sh -c "echo actual-secret-value"
# Screen output: VK:LOCAL:yyyyyyyy  (masked!)
```

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
