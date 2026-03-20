# Installation Guides

Platform-specific installation guides for VeilKey Self-Hosted.

| Platform | Status | Guide |
|----------|--------|-------|
| [macOS](./macos/) | Tested | npm or source build + Docker |
| [Proxmox LXC Debian](./proxmox-lxc-debian/) | Tested | Privileged LXC + Docker Compose |

After installation, follow the [Post-Install Setup](../docs/setup.md) to initialize VaultCenter and register LocalVault.

## Common

| Guide | Description |
|-------|-------------|
| [Add a LocalVault](./common/localvault.md) | Add a standalone LocalVault to an existing VaultCenter (any platform) |

## Which guide should I follow?

- **macOS (local development)** — You want to run VeilKey on your Mac with `veil` CLI in your terminal.
- **Proxmox LXC Debian (self-hosted server)** — You want to run VeilKey as a service on a Proxmox hypervisor.
- **Other Linux** — Not yet tested. Contributions welcome.
