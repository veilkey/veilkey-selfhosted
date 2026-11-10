# LocalVault — macOS

Docker Compose wrapping for [registration.md](./registration.md). Issue a registration token in the VaultCenter keycenter UI first; Docker networking does not waive the token requirement.

```bash
# Init with the issued registration token
docker compose exec -T localvault veilkey-localvault init --root \
  --token '<REG_TOKEN>' \
  --center https://vaultcenter:10181

# Restart to apply and bootstrap auto-unlock
docker compose restart localvault
```

The unlock key is generated automatically. Existing registered LocalVault instances do not need to be reinitialized or receive a new token on restart.
