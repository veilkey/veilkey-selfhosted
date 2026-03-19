# VeilKey Self-Hosted Documentation

## Components

| Component | Path | Language | Role |
|-----------|------|----------|------|
| VaultCenter | `services/vaultcenter/` | Go | Central management, admin UI, CometBFT chain, agentDEK encryption |
| LocalVault | `services/localvault/` | Go | Storage-only vault (no decrypt capability), heartbeat |
| veil-cli | `services/veil-cli/` | Rust | veil, veilkey, veilkey-cli, veilkey-session-config |

## Documentation Index

| Document | Description |
|----------|-------------|
| [architecture.md](./architecture.md) | System architecture, VC/LV split, agentDEK, blockchain |
| [installation.md](./installation.md) | Docker Compose quick start, setup flow |
| [cli.md](./cli.md) | CLI reference (veil, veilkey, veilkey-cli) |
| [security-model.md](./security-model.md) | Security model, PTY masking, threat boundaries |
| [OPERATING-MODEL.md](./OPERATING-MODEL.md) | Operational model, heartbeat, key rotation |
| [contributing.md](./contributing.md) | Contribution guidelines |

## Canonical Identity Terms

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of a LocalVault instance |
| `vault_hash` / `vault_runtime_hash` | Stable human-readable vault identifier |
| `agent_hash` | Alias for vault_runtime_hash |
| `agentDEK` | Per-vault encryption key held by VaultCenter |
