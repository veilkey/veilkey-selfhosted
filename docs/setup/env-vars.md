# Environment Variables

## Docker Compose (`.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `IMAGE_TAG` | `dev` | Docker image tag |
| `VAULTCENTER_HOST_PORT` | `11181` | VaultCenter host port |
| `LOCALVAULT_HOST_PORT` | `11180` | LocalVault host port |
| `VAULTCENTER_P2P_PORT` | `26656` | CometBFT P2P port |
| `VAULTCENTER_RPC_PORT` | `26657` | CometBFT RPC port |
| `VEILKEY_TRUSTED_IPS` | `10.0.0.0/8,...` | Trusted IP ranges (CIDR) |
| `VAULTCENTER_AUTO_COMPLETE_INSTALL_FLOW` | `0` | Auto-complete initial setup |
| `LOCALVAULT_LABEL` | `localvault-01` | LocalVault display name |
| `LOCALVAULT_CHAIN_PEERS` | - | CometBFT persistent peers |

## Service (internal)

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_ADDR` | `:10181` / `:10180` | Listen address |
| `VEILKEY_DB_PATH` | `/data/veilkey.db` | Database path |
| `VEILKEY_DB_KEY` | **(KEK-derived)** | SQLCipher encryption key. Derived from master password (KEK) during unlock. DB only opens after password entry |
| `VEILKEY_TLS_INSECURE` | `0` | Accept self-signed certs (also applies to `init --token` validation) |
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
| `VEILKEY_UNLOCK_PASSWORD` | - | Master password for auto-unlock on startup. If set, LocalVault unlocks via VaultCenter automatically after reboot |
| `VEILKEY_TRUSTED_IPS` | `10.0.0.0/8,...,::1` | Always include `::1` (IPv6 loopback) to prevent localhost unlock failures |
