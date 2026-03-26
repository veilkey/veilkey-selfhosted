# Linux — veil-cli Installation

Install veil CLI on any Linux machine.

## Prerequisites

- Rust / cargo: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- build-essential: `apt install build-essential`
- Network access to VaultCenter

## Install

```bash
cd veilkey-selfhosted
VEILKEY_URL=https://<VC_HOST>:<VC_PORT> bash install/common/install-veil-cli.sh
```

Or with pre-built binaries:

```bash
VEILKEY_URL=https://<VC_HOST>:<VC_PORT> \
VEILKEY_BINARY_URL=https://releases.example.com/veilkey/latest \
bash install/common/install-veil-cli.sh
```

## Options

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `VEILKEY_URL` | **(required)** | VaultCenter address |
| `VEILKEY_BIN_DIR` | `/usr/local/bin` | Binary install path |
| `VEILKEY_CONFIG_DIR` | `~/.veilkey` | Config directory |
| `VEILKEY_TLS_INSECURE` | `1` | Skip TLS verification |
| `VEILKEY_BINARY_URL` | - | Pre-built binary base URL |

## After install

```bash
source ~/.veilkey/env

# Check connection
veilkey-cli status

# Enter protected shell
veil

# Create temp ref
veilkey-cli create myvalue

# Resolve ref
veilkey-cli resolve VK:LOCAL:abc12345

# SSH key management
veilkey-cli ssh add ~/.ssh/id_ed25519 --label "main-key"
veilkey-cli ssh list
```

## Uninstall

```bash
bash install/common/uninstall-veil-cli.sh
```
