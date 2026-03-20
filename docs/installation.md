# Installation

> **This page is a legacy reference.** For platform-specific installation guides, see [`install/`](../install/).
> For post-install setup, see [`setup.md`](./setup.md).

## Platform Guides

| Platform | Guide |
|----------|-------|
| macOS | [`install/macos/`](../install/macos/) |
| Proxmox LXC (Debian) | [`install/proxmox-lxc-debian/`](../install/proxmox-lxc-debian/) |

## Requirements

- **Docker** + **Docker Compose** v2+
- Ports: `11181` (VaultCenter), `11180` (LocalVault), `26656` (P2P), `26657` (RPC)

## Building from Source

```bash
# Go services
cd services/vaultcenter && go build ./...
cd services/localvault && go build ./...

# Rust CLI
cd services/veil-cli && cargo build --release
```
