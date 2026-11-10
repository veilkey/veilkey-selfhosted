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

> **Note:** `VEILKEY_DB_KEY`는 unlock 시 마스터 비밀번호(KEK)에서 자동 파생됩니다. 수동 설정이 필요 없습니다.

3. LocalVault appears in the vault list after heartbeat

## Trusted addresses and registration

The first registration requires a VaultCenter-issued registration token, including from trusted addresses. Configure `VEILKEY_TRUSTED_IPS` with only the VaultCenter source address; private-network ranges and localhost are not trusted by default. Trusted-IP access does not replace the agent secret required by post-registration APIs. Existing registered agents continue using their issued agent secret.

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
