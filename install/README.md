# Installation Guides

Platform-specific installation guides for VeilKey Self-Hosted.

## Platforms

| Platform | Script | Guide |
|----------|--------|-------|
| [macOS](./macos/) | `install-all.sh` / `install-server.sh` / `install-cli.sh` | [install-all.md](./macos/install-all.md) |
| [Proxmox LXC Debian](./proxmox-lxc-debian/) | `install-veilkey.sh` | [install-veilkey.md](./proxmox-lxc-debian/install-veilkey.md) |

## Common

| Script | Guide | Description |
|--------|-------|-------------|
| `install-localvault.sh` | [install-localvault.md](./common/install-localvault.md) | Add a standalone LocalVault (any platform) |

After installation, follow the [Post-Install Setup](../docs/setup.md) to initialize VaultCenter and register LocalVault.

## Which guide should I follow?

- **macOS (local development)** — You want to run VeilKey on your Mac with `veil` CLI in your terminal.
- **Proxmox LXC Debian (self-hosted server)** — You want to run VeilKey as a service on a Proxmox hypervisor.
- **Other Linux** — Not yet tested. Contributions welcome.
