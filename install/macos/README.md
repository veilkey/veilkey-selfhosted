# macOS Installation

## Option 1: npm (recommended)

```bash
npm install -g veilkey-cli
sudo codesign --force --sign - $(npm prefix -g)/lib/node_modules/veilkey-cli/native/*
```

Then start the server:

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
cp .env.example .env
docker compose up -d
```

## Option 2: Source build (one-liner)

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash install/macos/install.sh
```

This script handles everything: prerequisite checks, Rust CLI build, npm install, codesign, Docker Compose startup.

## Prerequisites

| Tool | Install | Required for |
|------|---------|-------------|
| Docker Desktop | [docker.com](https://docs.docker.com/desktop/install/mac-install/) | VaultCenter, LocalVault |
| Node.js / npm | `brew install node` | CLI install |
| Rust / cargo | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` | Source build only |

## After install

1. Open `https://localhost:11181` — set master + admin password
2. `cd veilkey-selfhosted && veil` — enter protected shell

> **HTTPS 인증서 경고가 뜨나요?** See [vaultcenter.md](./vaultcenter.md) for browser/CLI trust setup.

See [Post-Install Setup](../../docs/setup.md) for full initialization steps.

## Update

```bash
npm update -g veilkey-cli          # CLI update
cd veilkey-selfhosted && git pull  # Server update
docker compose up --build -d       # Docker rebuild
```

## Uninstall

```bash
cd veilkey-selfhosted
bash install/macos/uninstall.sh
```

## Add a standalone LocalVault

See [localvault.md](../common/localvault.md).
