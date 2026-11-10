# LocalVault — Registration

## Required registration token

Every first-time LocalVault registration requires a valid, unused registration token, including Docker networks, localhost, and other trusted IPs. A trusted IP or `--center` URL never replaces the token. Existing registered agents continue using their saved identity and agent secret; they do not need another registration token for normal heartbeats or restarts.

In the keycenter UI (`https://<HOST>:<VC_PORT>/keycenter`):

1. Click "+ 등록 토큰" to issue a registration token.
2. Initialize LocalVault with the issued token. Replace `<REG_TOKEN>` below with that token; do not commit it to source control.

```bash
docker compose exec -T localvault veilkey-localvault init --root \
  --token '<REG_TOKEN>' \
  --center https://vaultcenter:10181
docker compose restart localvault
```

The token's embedded VaultCenter URL takes precedence. `--center` is required only when the token does not include a URL. For a self-signed development certificate, configure `VEILKEY_TLS_INSECURE=1` in the LocalVault container before initialization.

Initialization validates the token with VaultCenter before creating or replacing local state, including when `--force` is used. The token ID is saved for the first heartbeat, where VaultCenter atomically consumes it. Invalid, expired, revoked, or already used tokens cannot register a new agent.

3. LocalVault appears in the vault list after its first successful heartbeat.

## Web setup

The LocalVault setup endpoint `POST /api/install/init` also requires the encoded token in the `registration_token` JSON field, together with `vaultcenter_url` when the token has no embedded URL. Initialization rejects a missing or invalid token before changing local data. A setup client must submit both fields; setting only the VaultCenter URL is insufficient.

## Trusted addresses and registration

The first registration requires a VaultCenter-issued registration token, including from trusted addresses. Configure `VEILKEY_TRUSTED_IPS` with only the VaultCenter source address; private-network ranges and localhost are not trusted by default. Trusted-IP access does not replace the agent secret required by post-registration APIs. Existing registered agents continue using their issued agent secret.

## Unlock after init

LocalVault generates its unlock key automatically and transfers it to VaultCenter during authenticated registration. After restarting, bootstrap unlock uses the local `vault_key` file; subsequent restarts use the saved agent secret to obtain the VaultCenter-managed unlock key. No password needs to be piped into `init`, and `VEILKEY_DB_KEY` is derived automatically.

## Verify

```bash
curl -sk <VC_URL>/health   # VaultCenter: {"status":"ok"}
curl -sk <LV_URL>/health   # LocalVault: {"status":"ok"}
```
