# Installation Guides

## Platforms

### macOS

| Service | Install | Uninstall | Guide |
|---------|---------|-----------|-------|
| VaultCenter | `macos/vaultcenter/install.sh` | `macos/vaultcenter/uninstall.sh` | [install.md](./macos/vaultcenter/install.md) |
| LocalVault | `macos/localvault/install.sh` | `macos/localvault/uninstall.sh` | [install.md](./macos/localvault/install.md) |
| veil-cli | `macos/veil-cli/install.sh` | `macos/veil-cli/uninstall.sh` | [install.md](./macos/veil-cli/install.md) |
| **All** | `macos/bootstrap/install-all.sh` | `macos/bootstrap/uninstall-all.sh` | [install-all.md](./macos/bootstrap/install-all.md) |

Troubleshooting: [vaultcenter/troubleshoot.md](./macos/vaultcenter/troubleshoot.md)

### Proxmox LXC (Debian)

| Component | Install | Uninstall | Guide |
|-----------|---------|-----------|-------|
| VeilKey (LXC) | `install-veilkey.sh` | - | [install-veilkey.md](./proxmox-lxc-debian/install-veilkey.md) |
| LocalVault (host) | `install-localvault.sh` | `uninstall-localvault.sh` | [install-localvault.md](./proxmox-lxc-debian/install-localvault.md) |

### Common

| Script | Guide |
|--------|-------|
| `common/install-localvault.sh` | [install-localvault.md](./common/install-localvault.md) |
| `common/install-veil-cli.sh` | [install-veil-cli.md](./common/install-veil-cli.md) |

## After installation

See [Post-Install Setup](../docs/setup.md).
