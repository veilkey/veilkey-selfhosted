# LocalVault — macOS

Docker Compose wrapping for [registration.md](./registration.md):

```bash
# Init (Docker internal network = trusted IP, no token needed)
docker compose exec -T localvault sh -c \
  "echo '<MASTER_PASSWORD>' | veilkey-localvault init --root \
    --center https://vaultcenter:10181"

# Restart to apply
docker compose restart localvault

# Unlock
curl -sk -X POST <LV_URL>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```
