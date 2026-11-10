# macOS — veil-cli Installation

## Prerequisites

- Node.js / npm: `brew install node`
- Rust / cargo: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`

## Install

Choose the latest published release tag, clone it, and run the installer:

```bash
RELEASE_TAG="<latest-release-tag>"
git clone --depth 1 --branch "$RELEASE_TAG" https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash install/macos/veil-cli/install.sh
```

Builds veil CLI from the release checkout, links the local `packages/veil-cli` package with npm, and codesigns for macOS Gatekeeper. It does not install the CLI from the npm registry. For updates, check out the latest release tag and rerun this script.

TLS verification is enabled by default. For a private CA, pass `VEILKEY_CA_CERT=/path/to/verified-ca.pem` when installing. See [CA provisioning](../../../docs/setup/tls.md).

Installed binaries: `veil`, `veilkey`, `veilkey-cli`, `veilkey-session-config`

## Configuration and after install

The installer writes `~/.veilkey/env` by default (directory mode `0700`, environment file mode `0600`). Launchers automatically read only this user-owned configuration, not a repository's `.veilkey/env`.

```bash
veil
```

To choose another location, pass `VEILKEY_CONFIG_DIR=/absolute/private/config` to the installer, then explicitly opt in when launching:

```bash
VEILKEY_ENV=/absolute/private/config/env veil
```

Only select an environment file you own and trust. Legacy project-local files are not automatically loaded. The uninstaller uses the same `VEILKEY_CONFIG_DIR` and removes only an installer-marked environment file; other user settings remain.

## Uninstall

```bash
bash install/macos/veil-cli/uninstall.sh
```
