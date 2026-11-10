# Environment Variables

## Docker Compose (`.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `IMAGE_TAG` | `dev` | Docker image tag |
| `VAULTCENTER_HOST_PORT` | `11181` | VaultCenter host port |
| `LOCALVAULT_HOST_PORT` | `11180` | LocalVault host port |
| `VAULTCENTER_P2P_PORT` | `26656` | CometBFT P2P port |
| `VAULTCENTER_RPC_PORT` | `26657` | CometBFT RPC port |
| `VEILKEY_TRUSTED_IPS` | empty | No trusted-IP bypass; use only exact VaultCenter source addresses if needed |
| `VAULTCENTER_AUTO_COMPLETE_INSTALL_FLOW` | `0` | Auto-complete initial setup |
| `LOCALVAULT_LABEL` | `localvault-01` | LocalVault display name |
| `LOCALVAULT_CHAIN_PEERS` | - | CometBFT persistent peers |

## Service (internal)

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_ADDR` | `:10181` / `:10180` | Listen address |
| `VEILKEY_DB_PATH` | `/data/veilkey.db` | Database path |
| `VEILKEY_DB_KEY` | **(KEK-derived)** | SQLCipher encryption key. Derived from master password (KEK) during unlock. DB only opens after password entry |
| `VEILKEY_TLS_INSECURE` | `0` | Verify TLS, including `init --token`; provision CA trust for private PKI |
| `VEILKEY_TLS_CERT` | - | TLS certificate path |
| `VEILKEY_TLS_KEY` | - | TLS private key path |
| `VEILKEY_CHAIN_HOME` | `/data/chain` | CometBFT data directory |
| `VEILKEY_TEMP_REF_TTL` | `1h` | Temp ref expiry |
| `VEILKEY_ADMIN_SESSION_TTL` | `2h` | Admin session duration |
| ~~`VEILKEY_PASSWORD_FILE`~~ | - | **Removed (v0.5.0).** KEK is memory-only |
| `VEILKEY_AGENT_SCHEME` | `https` | Scheme for agent connections |

## Standalone LocalVault

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_BULK_APPLY_ALLOWED_PATHS` | - | Comma-separated absolute paths for bulk-apply |
| `VEILKEY_BULK_APPLY_ALLOWED_HOOKS` | - | Comma-separated `name:cmd` pairs for custom hooks |
| `VEILKEY_VAULT_NAME` | `$(hostname)` | Vault display name |
| `VEILKEY_VAULTCENTER_URL` | - | VaultCenter URL for heartbeat |
| `VEILKEY_REG_TOKEN` | - | Required by standalone installers for initial registration; obtain from VaultCenter |
| `SSL_CERT_FILE` | system roots | Verified CA PEM bundle for Go clients; installers persist only the public path |
| `VEILKEY_TRUSTED_IPS` | resolved VaultCenter address only | No private-range or loopback defaults; authenticated routes still require agent/admin credentials |

Master passwords and password-file paths must not be stored in `.env`. LocalVault uses its VaultCenter-managed unlock key; unlock VaultCenter interactively after restart. See [TLS provisioning](./tls.md) for CA setup and the explicitly limited single-host Docker-internal exception.

## Host CLI

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_TLS_INSECURE` | `0` | TLS verification enabled |
| `VEILKEY_CA_CERT` | public roots | Verified PEM CA bundle for private PKI; replaces the CLI root bundle |
