# Generated Summary

This file is generated from `docs/cue/`. Do not edit it manually.

## Components

| Component | Kind | Path | README |
|---|---|---|---|
| `installer` | `installer` | `installer` | [`installer/README.md`](../installer/README.md) |
| `keycenter` | `service` | `services/keycenter` | [`services/keycenter/README.md`](../services/keycenter/README.md) |
| `localvault` | `service` | `services/localvault` | [`services/localvault/README.md`](../services/localvault/README.md) |
| `proxy` | `service` | `services/proxy` | [`services/proxy/README.md`](../services/proxy/README.md) |
| `cli` | `client` | `client/cli` | [`client/cli/README.md`](../client/cli/README.md) |

## Top-Level Validate Jobs

- `facts-validate`
- `installer-validate`
- `keycenter-validate`
- `localvault-validate`
- `cli-validate`
- `proxy-validate`

## Top-Level E2E Jobs

- `selfhosted-e2e-proxmox-allinone`
- `selfhosted-e2e-proxmox-runtime`
- `selfhosted-e2e-proxmox-account-smoke`

## Identity Terms

### Primary Terms

- `vault_node_uuid`
- `vault_hash`
- `vault_runtime_hash`

### Compatibility Aliases

- `node_id`
- `agent_hash`

## Hardcoding Guardrails

Blocking checks currently fail on these literals:

- `10.50.100.210`
- `10.60.100.210`
- `gitlab.ranode.net`

For the broader audit report, see ./hardcoding-report.md.
