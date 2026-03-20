# macOS — Full Installation (VaultCenter + LocalVault + veil-cli)

## Quick start

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash install/macos/bootstrap/install-all.sh
```

This runs the following sequentially:
1. `install/macos/vaultcenter/install.sh`
2. `install/macos/localvault/install.sh`
3. `install/macos/veil-cli/install.sh`

## Install separately

```bash
bash install/macos/vaultcenter/install.sh   # VaultCenter (Docker)
bash install/macos/localvault/install.sh    # LocalVault (Docker, requires VaultCenter)
bash install/macos/veil-cli/install.sh      # veil CLI (Rust build + npm + codesign)
```

## Bootstrap (no clone needed)

```bash
curl -sL .../install-all-bootstrap.sh | bash
```

Clones the repo automatically, then runs `install-all.sh`.

## Prerequisites

| Tool | Install | Required for |
|------|---------|-------------|
| Docker Desktop | [docker.com](https://docs.docker.com/desktop/install/mac-install/) | VaultCenter, LocalVault |
| Node.js / npm | `brew install node` | veil-cli |
| Rust / cargo | `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs \| sh` | veil-cli |

## After install

1. Open `https://localhost:<VC_PORT>` (default: `11181`) — set master + admin password
2. `cd veilkey-selfhosted && veil` — enter protected shell

> **HTTPS 인증서 경고?** See [vaultcenter/troubleshoot.md](../vaultcenter/troubleshoot.md)

See [Post-Install Setup](../../../docs/setup.md) for full initialization steps.

## Update

```bash
npm update -g veilkey-cli          # CLI update
cd veilkey-selfhosted && git pull  # Server update
docker compose up --build -d       # Docker rebuild
```

## Uninstall

```bash
bash install/macos/bootstrap/uninstall-all.sh
```

## Add a standalone LocalVault

See [install-localvault.md](../../common/install-localvault.md).
