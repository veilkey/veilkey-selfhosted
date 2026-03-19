# Installation

## Requirements

- **Docker** + **Docker Compose** v2+
- Ports: `11181` (VaultCenter), `11180` (LocalVault), `26656` (P2P), `26657` (RPC)

## Quick Start

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
docker compose up -d
```

## Setup Flow

### 1. VaultCenter Setup

Open `https://localhost:11181` in your browser.

- Enter master password (KEK derivation — remember this)
- Enter admin password (web UI login)
- Setup complete → auto-unlock on restart

### 2. LocalVault Registration

In the keycenter UI (`https://localhost:11181/keycenter`):
1. Click "+ 등록 토큰" to issue a registration token
2. Run inside the localvault container:

```bash
docker compose exec localvault sh -c \
  "echo 'your-password' | veilkey-localvault init --root \
    --token vk_reg_xxx \
    --center https://vaultcenter:10181"
docker compose restart localvault
```

3. LocalVault appears in the vault list after heartbeat

### 3. Store Secrets

In the keycenter UI:
1. "+ 임시키" → enter name and value
2. Select the key → "볼트에 저장 (격상)" → select vault
3. Secret is now encrypted with agentDEK and stored in LocalVault

### 4. Use veil CLI

```bash
# Enter protected session
docker compose exec -it \
  -e DB_PASSWORD=VK:LOCAL:xxxx \
  veil veilkey-cli wrap-pty bash

# Inside: echo $DB_PASSWORD shows VK:LOCAL:xxxx (masked)
# But actual processes receive the real value
```

## Environment Variables

See `services/vaultcenter/.env.example` and `services/localvault/.env.example`.

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_ADDR` | `:10181` / `:10180` | Listen address |
| `VEILKEY_DB_PATH` | `/data/veilkey.db` | Database path |
| `VEILKEY_TLS_INSECURE` | `0` | Accept self-signed certs |
| `VEILKEY_CHAIN_HOME` | `/data/chain` | CometBFT data directory |
| `VEILKEY_TEMP_REF_TTL` | `1h` | Temp ref expiry |
| `VEILKEY_ADMIN_SESSION_TTL` | `2h` | Admin session duration |
| `VEILKEY_TRUSTED_IPS` | - | Trusted IP ranges (CIDR) |

## Building from Source

```bash
# Go services
cd services/vaultcenter && go build ./...
cd services/localvault && go build ./...

# Rust CLI
cd services/veilkey-cli && cargo build --release
cd services/veil-cli && cargo build --release
```
