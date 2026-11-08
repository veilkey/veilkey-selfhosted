# veil-cli

Rust CLI tools for VeilKey. Provides PTY-level bidirectional masking so AI coding tools never see your secrets.

## Binaries

| Binary | Purpose |
|--------|---------|
| `veil` | Enter protected PTY session with masking |
| `veilkey` | State control, crypto, policy |
| `veilkey-cli` | scan, filter, wrap-pty, exec, resolve, status |
| `veilkey-session-config` | Load TOML session configuration |

## Build

```bash
cargo build --release
```

## Usage

```bash
# Enter protected shell
veil

# Resolve a secret
veilkey-cli resolve VK:LOCAL:xxxxx

# Execute with ref replacement
veilkey-cli exec echo VK:LOCAL:xxxxx

# Scan for secrets
veilkey-cli scan file.env

# Check connection
veilkey-cli status
```

## Install

- macOS: [`install/macos/veil-cli/`](../../install/macos/veil-cli/install.md)
- Linux: [`install/common/install-veil-cli.sh`](../../install/common/install-veil-cli.md)

## Documentation

See [docs/setup/veil-cli/usage.md](../../docs/setup/veil-cli/usage.md).

## Security

Please report vulnerabilities privately through the repository's [security policy](https://github.com/veilkey/veilkey-selfhosted/security/policy), not through public issues.
