# macOS — Full Installation (Server + CLI)

## Quick start

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash install/macos/install-all.sh
```

This runs `install-server.sh` + `install-cli.sh` sequentially.

To install separately:

```bash
bash install/macos/install-server.sh   # Docker Compose (VaultCenter + LocalVault)
bash install/macos/install-cli.sh      # veil CLI (Rust build + npm + codesign)
```

## Bootstrap (no clone needed)

```bash
curl -sL .../install-all-bootstrap.sh | bash
```

Clones the repo automatically, then runs `install-all.sh`.

## Prerequisites

| Tool | Install | Required for |
|------|---------|-------------|
| Docker Desktop | [docker.com](https://docs.docker.com/desktop/install/mac-install/) | `install-server.sh` |
| Node.js / npm | `brew install node` | `install-cli.sh` |
| Rust / cargo | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` | `install-cli.sh` |

## After install

1. Open `https://localhost:11181` — set master + admin password
2. `cd veilkey-selfhosted && veil` — enter protected shell

> **HTTPS 인증서 경고?** See [troubleshoot-vaultcenter.md](./troubleshoot-vaultcenter.md)

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

See [install-localvault.md](../common/install-localvault.md).
