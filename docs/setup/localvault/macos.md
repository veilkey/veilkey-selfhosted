# LocalVault — macOS

Use the release checkout and obtain a token as described in [registration.md](./registration.md). First registration requires a token even on the Docker-internal network.

```bash
docker compose exec -T localvault veilkey-localvault init --root \
  --token '<REGISTRATION_TOKEN>' \
  --center https://vaultcenter:10181
docker compose restart localvault
```

LocalVault generates a VaultCenter-managed unlock key; no master password is supplied or saved by this flow. Unlock VaultCenter interactively after a restart.

The bundled TLS exception is single-host Docker-internal only. Provision [CA trust](../tls.md) for host clients and remote connections; keep TLS verification enabled.
