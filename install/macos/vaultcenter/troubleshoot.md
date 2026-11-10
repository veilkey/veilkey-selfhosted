# macOS — VaultCenter Setup Troubleshooting

Host clients verify HTTPS certificates by default. Before initial setup or unlock, follow [TLS certificate provisioning](../../../docs/setup/tls.md). Do not bypass a browser warning or disable TLS verification to submit credentials.

## Browser or curl rejects the certificate

Use a CA-issued certificate with SANs matching the VaultCenter URL. Obtain the verified public CA certificate through an authenticated administrative channel, check its fingerprint independently, then add it to Keychain:

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /path/to/verified-ca.pem
```

For an individual curl request:

```bash
curl --cacert /path/to/verified-ca.pem https://localhost:${VAULTCENTER_HOST_PORT:-11181}/health
```

If deliberately trusting a generated self-signed certificate for a local deployment, copy it through the locally authenticated Docker connection and verify its fingerprint before trusting it. Do not fetch it over an unverified network connection. Re-provision trust after certificate rotation.

## CLI rejects the certificate

The Rust CLI uses its own root bundle. Configure the verified PEM CA path, even if Keychain already trusts your private CA:

```bash
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem
veilkey-cli status
```

Pass `VEILKEY_CA_CERT` when running `install/macos/veil-cli/install.sh` to retain that public path in `~/.veilkey/env` (owner-only). A custom `VEILKEY_CONFIG_DIR` requires explicit `VEILKEY_ENV=/absolute/path/env` when launching; project-local `.veilkey/env` is no longer auto-loaded. Host installers no longer enable insecure TLS. The bundled Compose exception applies only to single-host Docker-internal traffic, not host clients.

## Codesign (macOS Sequoia+)

The installer ad-hoc signs the local checkout's native CLI binaries. If a manual install was not signed:

```bash
sudo codesign --force --sign - $(npm prefix -g)/lib/node_modules/veilkey-cli/native/*
```

## Docker Desktop: port binding

If port 11181 is already in use:

```bash
lsof -i :${VAULTCENTER_HOST_PORT:-11181}
VAULTCENTER_HOST_PORT=11182 docker compose up -d
```

Use `https://localhost:11182` and ensure the certificate includes `localhost` in its SANs.

## VaultCenter connection refused

```bash
docker compose ps
docker compose logs vaultcenter
curl --cacert /path/to/verified-ca.pem https://localhost:${VAULTCENTER_HOST_PORT:-11181}/health
```
