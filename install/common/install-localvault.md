# Add a Standalone LocalVault

Add a LocalVault to an existing VaultCenter without Docker.

## Prerequisites

- Go 1.25+: `brew install go`
- A running VaultCenter instance

## Install

```bash
curl -sL "https://gist.githubusercontent.com/dalsoop/11e00346263678340189cdfdc79644b5/raw/install-localvault.sh?$(date +%s)" | \
  VEILKEY_CENTER_URL=https://<vaultcenter-host>:<port> bash
```

Or via `veil` CLI:

```bash
veil localvault init      # Install + start
veil localvault stop      # Stop
veil localvault log       # Tail logs
veil localvault status    # Health check
```

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_CENTER_URL` | - | VaultCenter address |
| `VEILKEY_PORT` | `10180` | LocalVault listen port |
| `VEILKEY_NAME` | `$(hostname)` | Vault name |

## How it works

The installer (`install/common/install-localvault.sh`):

1. Checks Go installation
2. Clones/updates source repo
3. Builds Go binary
4. Creates `.localvault/` data directory + `.env`
5. Starts process with nohup

Re-running the same command updates and restarts.
