# VeilKey KeyCenter

`keycenter` is the self-hosted VeilKey control plane.

Use the root repository README as the primary entry point:

- [`../../README.md`](../../README.md)

This document stays intentionally short and service-focused.

## Role

KeyCenter owns:

- LocalVault inventory
- runtime identity tracking
- policy and lifecycle decisions
- orchestration endpoints
- operator-facing control-plane APIs

KeyCenter does not act as a generic plaintext secret bucket.

## Runtime Identity

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of a LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable human-readable vault identifier |
| `vault_runtime_hash` | current runtime binding hash |
| `agent_hash` | internal compatibility alias for `vault_runtime_hash` |

## Repository Context

Canonical repository:

- `veilkey-selfhosted`

Relevant neighboring paths:

- `../../installer/`
- `../localvault/`
- `../../client/cli/`
- `../proxy/`

## Local Development

Build:

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-keycenter ./cmd/main.go
```

Primary local test entry point:

```bash
go test ./cmd/... ./internal/api/... ./internal/db/...
```

Focused validation commands:

```bash
bash tests/test_mr_guard.sh
bash tests/test_deploy_lxc.sh
```

## CI

Canonical top-level pipeline job:

- `keycenter-validate`

Service-local CI intent:

- Go unit/integration coverage for `cmd`, `internal/api`, and `internal/db`
- deploy guard coverage for LXC packaging paths
- MR guard enforcement for tests/docs on behavior changes

## Operator Notes

- Plaintext ingress is accepted only through explicit operator/agent boundaries.
- Direct `/api/secrets*` plaintext CRUD is not part of the supported surface.
- Planned rotation is scheduled through `POST /api/agents/rotate-all`.
- Rebind and heartbeat state are the authoritative source for runtime identity health.

## Deployment

`scripts/deploy-lxc.sh` must run on a Proxmox host.

Expected deploy flow:

1. Build a fresh local binary.
2. Resolve the target LXC service/unit path.
3. Push the binary and restart the service.
4. Verify deployed SHA256 matches the local artifact.

If verification fails, treat the deploy as failed.

## More Detail

Use the source tree directly for deeper work:

- `internal/api/`
- `internal/db/`
- `cmd/`
- `scripts/`
- `tests/`

For product-level context, CI entry points, and repository-wide layout, return to [`../../README.md`](../../README.md).
