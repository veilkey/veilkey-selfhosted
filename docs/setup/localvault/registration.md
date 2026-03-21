# LocalVault — Registration

## With registration token (Web UI)

In the keycenter UI (`https://<HOST>:<VC_PORT>/keycenter`):

1. Click "+ 등록 토큰" to issue a registration token
2. Run inside the localvault container:

```bash
docker compose exec -T localvault sh -c \
  "echo '<MASTER_PASSWORD>' | veilkey-localvault init --root \
    --token <REG_TOKEN> \
    --center https://vaultcenter:10181"
docker compose restart localvault
```

3. LocalVault appears in the vault list after heartbeat

## Without token (trusted IP)

If the LocalVault is on a trusted IP network (default: `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`), registration tokens are not required:

```bash
echo '<MASTER_PASSWORD>' | veilkey-localvault init --root \
  --center <VC_URL>
```

## Unlock after init

```bash
curl -sk -X POST <LV_URL>/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<MASTER_PASSWORD>"}'
```

## Verify

```bash
curl -sk <VC_URL>/health   # VaultCenter: {"status":"ok"}
curl -sk <LV_URL>/health   # LocalVault: {"status":"ok"}
```
