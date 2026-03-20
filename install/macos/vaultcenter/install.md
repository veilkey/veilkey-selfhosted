# macOS — VaultCenter Installation

## Install

```bash
bash install/macos/vaultcenter/install.sh
```

Builds and starts VaultCenter via Docker Compose.

## Prerequisites

- Docker Desktop: [docker.com](https://docs.docker.com/desktop/install/mac-install/)

## After install

Open `https://localhost:11181` — set master + admin password.

> **HTTPS 인증서 경고?** See [troubleshoot.md](./troubleshoot.md)

## Uninstall

```bash
bash install/macos/vaultcenter/uninstall.sh
```
