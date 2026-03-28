# Add a Standalone LocalVault

Add a LocalVault to an existing VaultCenter.

## Prerequisites

- Go 1.25+ or pre-built binary
- A running VaultCenter instance
- Registration token from VaultCenter admin

## Install

```bash
VEILKEY_CENTER_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_REG_TOKEN=vk_reg_... \
bash install/common/install-localvault.sh
```

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | **(required)** | VaultCenter address |
| `VEILKEY_REG_TOKEN` | **(required)** | Registration token |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_LABEL` | `$(hostname)` | Vault display name |
| `VEILKEY_DATA_DIR` | `/data/localvault` | Data directory |
| `VEILKEY_BIN_DIR` | `/usr/local/bin` | Binary install path |
| `VEILKEY_TLS_INSECURE` | `1` | Skip TLS verification |
| `VEILKEY_TRUSTED_IPS` | private ranges | Allowed IP CIDRs |
| `VEILKEY_SYSTEMD` | `1` | Create systemd service |
| `VEILKEY_BINARY_URL` | - | Pre-built binary URL (skip build) |

## How it works

The installer:

1. Builds binary (or downloads from `VEILKEY_BINARY_URL`)
2. Creates data directory and self-signed TLS cert
3. Runs `init --root --center <url> --token <token>` (auto-generated password, VC-managed unlock)
4. Writes `.env` with TLS and runtime config
5. Creates + enables systemd service (or nohup fallback) and waits for `health=ok`

## Management

```bash
# systemd
systemctl status veilkey-localvault
systemctl restart veilkey-localvault
journalctl -u veilkey-localvault -f

# Or via veil CLI
veil localvault status
veil localvault log
veil localvault stop
```

## Getting a Registration Token

```bash
# From VaultCenter admin
veilkey-cli function list   # (admin commands)

# Or via API
curl -X POST https://<vc>/api/admin/registration-tokens \
  -H 'Content-Type: application/json' \
  -b <session-cookie> \
  -d '{"label":"my-vault","max_uses":1}'
```

## Re-install / Update

Re-running the script with the same `VEILKEY_DATA_DIR` will:
- Rebuild/re-download binary
- Skip init (DB exists)
- Update .env
- Restart service
