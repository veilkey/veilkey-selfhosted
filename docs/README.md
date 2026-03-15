# VeilKey Self-Hosted -- docs/

This directory contains centralized governance documentation for the VeilKey self-hosted product surface.

It is the single location for contracts, validation tooling, and hardcoding policy that apply across all self-hosted components.

## Subdirectories

| Directory | Purpose |
|-----------|---------|
| `cue/` | CUE-based contracts: schema definitions and cross-component validation rules |
| `tools/` | Generation and validation utilities that operate on contracts and component metadata |
| `hardcoding/` | Hardcoding governance: policies and audits for values that must not be inlined in source |

## Canonical Identity Terms

All docs and tooling in this directory use the following canonical terms. Component READMEs follow the same convention.

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of a LocalVault instance |
| `vault_hash` | Stable human-readable vault identifier |
| `vault_runtime_hash` | Current KeyCenter runtime binding hash (compatibility alias: `agent_hash`) |

Compatibility aliases remain in some API surfaces for backward compatibility, but governance docs use the canonical field names only.

## Component READMEs

Each self-hosted component maintains its own README with component-specific details:

| Component  | README | Description |
|------------|--------|-------------|
| Root       | [`README.md`](../README.md) | Monorepo layout and runtime model |
| Installer  | [`installer/README.md`](../installer/README.md) | Installation, packaging, and health verification |
| KeyCenter  | [`services/keycenter/README.md`](../services/keycenter/README.md) | Central control plane |
| LocalVault | [`services/localvault/README.md`](../services/localvault/README.md) | Node-local agent and ciphertext store |
| Proxy      | [`services/proxy/README.md`](../services/proxy/README.md) | Outbound enforcement layer |
| CLI        | [`client/cli/README.md`](../client/cli/README.md) | Operator CLI and secure terminal |

## Installer Documentation

The installer component maintains additional docs not linked from its README:

| Document | Topic |
|----------|-------|
| [`installer/INSTALL.md`](../installer/INSTALL.md) | Operator install flow (referenced from installer README) |
| [`installer/docs/installer-architecture.md`](../installer/docs/installer-architecture.md) | Installer architecture overview |
| [`installer/docs/android-remote-control.md`](../installer/docs/android-remote-control.md) | Android remote control setup |
| [`installer/docs/windows-remote-control.md`](../installer/docs/windows-remote-control.md) | Windows remote control setup |

## Installer Sub-component READMEs

| Path | Topic |
|------|-------|
| [`installer/scripts/proxmox-host-localvault/README.md`](../installer/scripts/proxmox-host-localvault/README.md) | Proxmox host LocalVault wrapper commands |
| [`installer/validation-logs/README.md`](../installer/validation-logs/README.md) | Validation log format and rules |

## LocalVault Examples

| Path | Topic |
|------|-------|
| [`services/localvault/docs/examples/bulk-apply/mattermost/README.md`](../services/localvault/docs/examples/bulk-apply/mattermost/README.md) | Mattermost bulk-apply example |

## Generated Documentation

Run `docs/tools/generate.sh` to produce machine-generated reference files under `docs/generated/`:

- `docs/generated/inventory.md` -- component inventory table
- `docs/generated/terminology.md` -- canonical VeilKey terminology

## Hardcoding Governance

- [`docs/hardcoding/rules.md`](hardcoding/rules.md) -- operator-facing hardcoding policy
- [`docs/cue/hardcoding_contract.cue`](cue/hardcoding_contract.cue) -- machine-checked hardcoding contract
- `bash docs/tools/doctor.sh` -- required validation before proposing governance changes

## Contracts and Facts

- [`docs/cue/docs_contract.cue`](cue/docs_contract.cue) -- CUE schema for docs structure, canonical terminology, and blocked term enforcement
- [`facts/repository.cue`](../facts/repository.cue) -- canonical doc paths and repo layout facts
