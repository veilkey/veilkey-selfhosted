# Linux — veil-cli Installation

Install veil CLI on any Linux machine.

## Prerequisites

- Rust / cargo: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- build-essential: `apt install build-essential`
- Network access to VaultCenter

Provision [CA trust](../../docs/setup/tls.md) before connecting. For a private CA, pass `VEILKEY_CA_CERT=/path/to/verified-ca.pem` when installing; the public path is retained in the CLI environment file. TLS verification stays enabled.

## Install

```bash
# Choose the latest published release tag.
RELEASE_TAG="<latest-release-tag>"
git clone --depth 1 --branch "$RELEASE_TAG" https://github.com/veilkey/veilkey-selfhosted.git
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
| `VEILKEY_TLS_INSECURE` | `0` | Verify TLS certificates |
| `VEILKEY_CA_CERT` | public roots | Verified PEM CA bundle for a private CA |
| `VEILKEY_BINARY_URL` | - | Pre-built binary base URL |

## After install

The generated config directory is owner-only (`0700`) and its `env` file is `0600`. Launchers default to `$HOME/.veilkey/env`; they do not auto-load a repository's `.veilkey/env`. With a custom `VEILKEY_CONFIG_DIR`, explicitly select the trusted file using `VEILKEY_ENV=/absolute/private/config/env veil`.

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
