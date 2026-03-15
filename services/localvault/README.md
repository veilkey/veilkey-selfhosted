# VeilKey LocalVault

`localvault` is the node-local VeilKey runtime.

Use the root repository README as the primary entry point:

- [`../../README.md`](../../README.md)

This document stays intentionally short and service-focused.

## Role

LocalVault owns:

- local ciphertext and config storage
- runtime identity and heartbeat
- local resolve/decrypt boundaries
- bulk-apply execution
- node-local policy enforcement requested by KeyCenter

LocalVault does not own:

- operator approval
- central lifecycle policy decisions
- general plaintext ingress UX

## Runtime Identity

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of the current LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable vault identifier |
| `vault_runtime_hash` | current KeyCenter runtime binding hash |
| `agent_hash` | internal compatibility alias for `vault_runtime_hash` |

## Repository Context

Canonical repository:

- `veilkey-selfhosted`

Relevant neighboring paths:

- `../keycenter/`
- `../../installer/`
- `../../client/cli/`

## Runtime Notes

- `.veilkey/context.json` should prefer `vault_node_uuid` and only fall back to `node_id`.
- Operator-facing output should center on `vault_hash` and `vault_runtime_hash`.
- New secret writes default to `TEMP / temp`.
- `activate` promotes a TEMP secret into `LOCAL` or `EXTERNAL`.
- Tracked-ref sync and heartbeat share the same effective KeyCenter URL resolution.

## Security Boundary

- LocalVault is a ciphertext store, not a central plaintext store.
- `POST /api/secrets`, `GET /api/secrets/{name}`, `GET /api/resolve/{ref}`, `POST /api/encrypt`, and `POST /api/rekey` are blocked.
- Lifecycle-changing routes may succeed locally while returning degraded sync metadata if KeyCenter sync fails.
- Blocked read/use paths must fail closed.

## Local Development

Build:

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-localvault .
```

Primary local test entry point:

```bash
go test ./cmd/... ./internal/api/... ./internal/db/...
```

Focused validation commands:

```bash
bash tests/test_mr_guard.sh
bash tests/policy/project_registry_policy.sh --self-test
bash tests/test_gitlab_project_settings.sh --self-test
bash tests/test_deploy_lxc.sh
bash tests/test_deploy_lxc_env_migration.sh
```

## CI

Canonical top-level pipeline job:

- `localvault-validate`

Service-local CI intent:

- Go unit/integration coverage for `cmd`, `internal/api`, and `internal/db`
- policy validation for project registry and GitLab settings
- deploy-path validation for LXC runtime packaging
- MR guard enforcement for tests/docs on behavior changes

## Operations

Useful commands:

```bash
VEILKEY_CONTEXT_FILE=/path/to/.veilkey/context.json veilkey-localvault cron tick
veilkey-localvault rebind --key-version 9
```

`cron tick` reports heartbeat, syncs GLOBAL functions, and applies planned rotation when present.

`rebind --key-version` aligns local node version after a human-approved rebind and should be followed by service restart plus heartbeat re-registration.

## Deployment

`scripts/deploy-lxc.sh` must run on a Proxmox host.

CI deploy jobs must use a `proxmox-host` runner.

## More Detail

Use the source tree directly for deeper work:

- `internal/api/`
- `internal/db/`
- `cmd/`
- `scripts/`
- `tests/`
- `docs/examples/bulk-apply/`

For product-level context, CI entry points, and repository-wide layout, return to [`../../README.md`](../../README.md).
