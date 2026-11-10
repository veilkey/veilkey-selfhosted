# LocalVault — Registration

Every first registration requires a VaultCenter-issued registration token, including Docker-internal, trusted-IP, and localhost callers. Already registered agents continue using their issued agent credentials. IP trust does not replace authentication.

## Obtain a registration token

In the keycenter UI (`https://<HOST>:<VC_PORT>/keycenter`), authenticate as admin and click "+ 등록 토큰". Use the token only for the intended LocalVault. Keep VaultCenter unlocked while registering.

## Bundled single-host Docker deployment

Run inside the release checkout:

```bash
docker compose exec -T localvault veilkey-localvault init --root \
  --token '<REGISTRATION_TOKEN>' \
  --center https://vaultcenter:10181
docker compose restart localvault
```

The bundled Compose TLS exception applies only to this single-host Docker-internal connection; see [TLS provisioning](../tls.md). Do not copy it to a remote installer or host CLI environment.

## Standalone LocalVault

Provision the verified CA bundle before initialization. Go uses the system trust store or `SSL_CERT_FILE`:

```bash
export VEILKEY_TLS_INSECURE=0
export SSL_CERT_FILE=/path/to/verified-ca.pem
veilkey-localvault init --root \
  --token '<REGISTRATION_TOKEN>' \
  --center https://<VC_HOST>:<VC_PORT>
```

Alternatively, use the [standalone installer](../../../install/common/install-localvault.md) with `VEILKEY_CENTER_URL` and `VEILKEY_REG_TOKEN`.

## Unlock and verification

LocalVault generates its own unlock key and registers it with VaultCenter. Do not supply or save a master password for LocalVault initialization. Unlock VaultCenter interactively after restart; registered LocalVaults retrieve their managed keys. No plaintext password belongs in `.env` or a password file.

LocalVault appears in the vault list after heartbeat. After provisioning CA trust, check health without disabling verification:

```bash
curl --cacert /path/to/verified-ca.pem https://<VC_HOST>:<VC_PORT>/health
curl --cacert /path/to/verified-ca.pem https://<LV_HOST>:<LV_PORT>/health
```

Management and secret routes still require an admin session or the appropriate agent Bearer token; health/status access is not proof of authorization.
