# Post-Install Setup

Installation complete? Follow these steps to initialize VeilKey.

## Conventions

| Placeholder | Default | Description |
|-------------|---------|-------------|
| `<VC_URL>` | `https://localhost:11181` | VaultCenter URL |
| `<LV_URL>` | `https://localhost:11180` | LocalVault URL |
| `<VC_PORT>` | `11181` | VaultCenter host port |
| `<MASTER_PASSWORD>` | - | Master password (KEK derivation) |
| `<ADMIN_PASSWORD>` | - | Admin password (web UI login) |
| `<AGENT_HASH>` | - | Vault agent hash (from `/api/agents`) |

Ports depend on your `.env` settings (`VAULTCENTER_HOST_PORT`, `LOCALVAULT_HOST_PORT`).

## Setup order

Provision [TLS certificates and CA trust](./tls.md) before registration or any host-side HTTPS request containing credentials. Host installers verify TLS by default. Never save a master password in `.env` or a password file; LocalVault uses VaultCenter-managed unlock.

| Step | Service | Guide |
|------|---------|-------|
| 1 | VaultCenter | [vaultcenter/setup.md](./vaultcenter/setup.md) — initial setup + unlock |
| 2 | LocalVault | [localvault/registration.md](./localvault/registration.md) — init + register with VaultCenter |
| 3 | Secrets | [secrets/manage.md](./secrets/manage.md) — create, promote, change |
| 4 | veil CLI | [veil-cli/usage.md](./veil-cli/usage.md) — resolve, exec, PTY masking |
| 5 | Bulk Apply | [secrets/bulk-apply.md](./secrets/bulk-apply.md) — template, workflow, deploy |

## Platform-specific guides

| Platform | LocalVault | Notes |
|----------|-----------|-------|
| macOS | [localvault/macos.md](./localvault/macos.md) | Docker Compose exec |
| Proxmox LXC | [localvault/proxmox-lxc-debian.md](./localvault/proxmox-lxc-debian.md) | pct exec + standalone |

## Environment variables

See [env-vars.md](./env-vars.md).
