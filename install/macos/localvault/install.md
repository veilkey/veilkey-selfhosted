# macOS — LocalVault Installation

## Prerequisites

- Docker Desktop
- VaultCenter running (`bash install/macos/vaultcenter/install.sh`)

## Install

```bash
bash install/macos/localvault/install.sh
```

Builds and starts LocalVault via Docker Compose. Requires VaultCenter to be healthy.

## After install

Register LocalVault with VaultCenter. See [Post-Install Setup](../../../docs/setup/README.md).

## Standalone LocalVault (without Docker)

To add a LocalVault outside Docker Compose, see [common/install-localvault.md](../../common/install-localvault.md).

## Uninstall

```bash
bash install/macos/localvault/uninstall.sh
```
