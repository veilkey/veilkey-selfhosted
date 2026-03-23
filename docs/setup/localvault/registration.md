# LocalVault — Registration

## With registration token (Web UI)

In the keycenter UI (`https://<HOST>:<VC_PORT>/keycenter`):

1. Click "+ 등록 토큰" to issue a registration token
2. Run inside the localvault container:

```bash
# Self-signed cert 환경에서는 TLS_INSECURE 설정 필수
export VEILKEY_TLS_INSECURE=1

docker compose exec -T localvault sh -c \
  "echo '<MASTER_PASSWORD>' | veilkey-localvault init --root \
    --token <REG_TOKEN> \
    --center https://vaultcenter:10181"
docker compose restart localvault
```

> **Note:** `VEILKEY_DB_KEY`가 `.env`에 설정되어 있어야 합니다. 없으면 서버가 시작을 거부합니다.

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
