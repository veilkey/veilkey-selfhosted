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

## Operator configuration and local state

`veil` reads `$HOME/.veilkey/env`, or the file explicitly selected by
`VEILKEY_ENV`. It does not discover `.veilkey/env` in the working directory,
parent projects, or a checkout under the home directory. Keep endpoint URLs,
CLI binary selection, authentication settings, and TLS policy in the shell or
these operator-controlled files. Use `VEILKEY_CA_CERT=/path/to/ca-bundle.pem`
for a private server CA; the PEM bundle becomes the CLI's trust store and both
certificate signatures and hostnames remain verified. An unreadable, empty, or
invalid bundle stops the CLI rather than disabling verification.

State defaults to `$XDG_STATE_HOME/veilkey-cli` when that variable is absolute,
or `$HOME/.local/state/veilkey-cli`. `VEILKEY_STATE_DIR` can select another
operator-controlled directory. The CLI requires ownership by the current user,
uses mode `0700` for the directory and `0600` for state files, and rejects state
paths that are symlinks. Existing state in a temporary directory is not imported;
remove old session logs and recreate any required watchlist in the private state
directory. New session logs contain only reference and detection metadata, never
secret previews. Protected shells use a unique owner-only rcfile, removed when
the shell exits, and do not persist shell history.

`veil localvault init` and `update` require a local release installer. If none is
found, clone the latest release tag and run its install script; the CLI does not
download and execute a remote fallback installer.

## Security

Please report vulnerabilities privately through the repository's [security policy](https://github.com/veilkey/veilkey-selfhosted/security/policy), not through public issues.
