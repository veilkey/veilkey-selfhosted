# VeilKey LocalVault

`localvault` is the canonical node-local VeilKey runtime.

It stores local ciphertext, configs, and runtime identity, and it executes node-local lifecycle actions under KeyCenter policy.

## Product Position

VeilKey is split into:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - [`installer`](../../installer)
  - [`keycenter`](../keycenter)
  - `localvault` (this component)
  - [`cli`](../../client/cli)
  - [`proxy`](../proxy)

## Responsibilities

This component owns:

- local ciphertext and config storage
- runtime identity (`vault_node_uuid` / `vault_hash`) and heartbeat
- local resolve and decrypt flows
- bulk-apply execution
- `managed_paths` reporting to KeyCenter
- planned rotation, rebind, and blocked state reflection
- node-local policy enforcement requested by KeyCenter

This component does **not** own:

- plaintext input UX
- operator approval
- central encrypt/decrypt policy decisions

Those responsibilities belong to KeyCenter and the CLI.

## Identity Terms

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of the current LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable vault identifier |
| `vault_runtime_hash` | current KeyCenter runtime binding hash |
| `agent_hash` | internal API compatibility alias for `vault_runtime_hash` |

Identity rules:

- `.veilkey/context.json` uses `vault_node_uuid` as the primary key; `node_id` is read as a compatibility fallback.
- Operator-facing output centers on `vault_hash` / `vault_runtime_hash`.
- `agent_hash` appears only in internal API compatibility contexts.

## Related Components

- [`keycenter`](../keycenter) -- central control plane
- [`installer`](../../installer) -- installs and verifies LocalVault targets
- [`cli`](../../client/cli) -- operator entrypoint, local tooling, and platform integration (Proxmox, Docker, etc.)

## KeyCenter URL Resolution

Heartbeat and tracked-ref sync share a single effective KeyCenter URL, resolved by this precedence:

1. `VEILKEY_KEYCENTER_URL` environment variable
2. `VEILKEY_KEYCENTER_URL` stored in DB config
3. `VEILKEY_HUB_URL` environment variable (legacy alias)
4. `VEILKEY_HUB_URL` stored in DB config (legacy alias)

If multiple sources contain differing values, a drift warning is logged at startup and runtime.

## Security Boundary

- Plaintext encryption/decryption is not performed in LocalVault. LocalVault is a ciphertext store; plaintext handling belongs to KeyCenter.
- The following endpoints are blocked (`403`):
  - `POST /api/secrets`, `GET /api/secrets/{name}`, `GET /api/resolve/{ref}`, `POST /api/encrypt`, `POST /api/rekey`
- Blocked-state read/use paths must be **fail-closed**: `GET /api/cipher/{ref}`, `GET /api/configs/{key}`, and explicit lifecycle transition endpoints return `423` with the canonical scoped ref.
- New secret storage defaults to scoped ref `TEMP/temp`.
- Re-storing a secret that is already `LOCAL/active` or `EXTERNAL/active` preserves the existing lifecycle.
- Companion field storage is only allowed when the parent secret is `VK:LOCAL` or `VK:EXTERNAL` and in `active` state.
- `activate`, `archive`, `block`, and `revoke` attempt a KeyCenter tracked-ref sync after the lifecycle change. If sync fails the lifecycle change is kept, and the API response includes `sync_status=degraded`, `sync_target`, and `sync_error` to surface partial failure.

## Features

- Local ciphertext/metadata storage
- Metadata retrieval via `/api/cipher` and `/api/secrets/meta/{name}`
- Companion field (`OTP`, `LOGIN_ID`, `KEY_PASSWORD`, etc.) metadata/cipher storage for `VK:LOCAL` / `VK:EXTERNAL` secrets
- Local configs CRUD (plaintext key-value)
- Automatic KeyCenter registration and heartbeat
- Vault identity reporting based on `vault_name:vault_hash`
- `managed_paths` reporting from `.veilkey/context.json` or `VEILKEY_MANAGED_PATHS`

### Function Catalog

LocalVault stores vault-local function rows:

- One function = one DB row.
- Scope is restricted to `GLOBAL`, `VAULT`, `LOCAL`, or `TEST`.
- `vars_json` stores per-variable `ref` and `LOCAL` | `EXTERNAL` class.
- `GLOBAL` functions are pull-synced from the KeyCenter SSOT as local materialized copies. The local API does not allow direct creation or deletion of `GLOBAL` rows; they are managed exclusively by KeyCenter sync.
- Rows can be queried with a scope filter, e.g. `GET /api/functions?scope=TEST`.
- `scope=TEST` functions are auto-deleted during `cron tick` once `created_at + 1h` has elapsed.

## Installation

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-localvault .
```

Initialization is performed automatically via the KeyCenter `init --child` command:

```bash
veilkey-keycenter init --child \
  --parent http://KEYCENTER_IP:10180 \
  --password <password> \
  --label my-service \
  --install
```

Place a `.veilkey/context.json` at the project/service root. The cron heartbeat uses this context file.

Example:

```json
{
  "version": 1,
  "managed_path": "/var/www/services/demo",
  "context_id": "auto-generated-uuid",
  "vault_node_uuid": "",
  "node_id": ""
}
```

- `context_id` identifies the local context file.
- Actual LocalVault owner identification uses `vault_node_uuid` / `vault_hash`.
- `vault_node_uuid` may be empty before live registration; after registration it is aligned with the current LocalVault identity.
- `node_id` is a compatibility alias.

## Deploy

`scripts/deploy-lxc.sh` must run on a Proxmox host only. The CI deploy job must use the `proxmox-host` runner.

Docker image push and LXC runtime deploy are separate CI stages so that an image-push failure does not block the LXC deploy.

## API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET /api/secrets` | | List secrets |
| `GET /api/secrets/meta/{name}` | | Retrieve ref/meta without plaintext |
| `POST /api/secrets` | `{"name":"key","value":"val"}` | Blocked (`403`) |
| `GET /api/secrets/{name}` | | Blocked (`403`) |
| `GET /api/resolve/{ref}` | | Blocked (`403`) |
| `POST /api/encrypt` | `{"plaintext":"val"}` | Blocked (`403`) |
| `GET /api/cipher/{ref}` | | Retrieve ciphertext/nonce by ref |
| `GET /api/cipher/{ref}/fields/{field}` | | Retrieve companion field ciphertext/nonce for LOCAL/EXTERNAL secret |
| `POST /api/cipher` | `{"name":"key","ref":"...","ciphertext":"...","nonce":"..."}` | Store KeyCenter-encrypted ciphertext. New secret defaults to `VK:TEMP:*` + `temp`; updating an existing secret preserves its lifecycle |
| `POST /api/secrets/fields` | `{"name":"GITHUB_KEY","fields":[{"key":"OTP","type":"otp","ciphertext":"...","nonce":"..."}]}` | Store/update companion fields on `VK:LOCAL`/`VK:EXTERNAL` + `active` secrets |
| `DELETE /api/secrets/{name}/fields/{field}` | | Delete companion field on active `VK:LOCAL`/`VK:EXTERNAL` secret |
| `POST /api/reencrypt` | `{"ciphertext":"VK:TEMP:deadbeef"}` | Verify scoped ref on explicit transition path and return canonical ref |
| `POST /api/activate` | `{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}` | Promote TEMP ref to `VK:LOCAL:ref` or `VK:EXTERNAL:ref` |
| `POST /api/active` | `{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}` | Compatibility alias for `activate` |
| `POST /api/archive` | `{"ciphertext":"VK:LOCAL:deadbeef"}` | Transition scoped ref to archive state |
| `POST /api/revoke` | `{"ciphertext":"VK:LOCAL:deadbeef"}` | Transition scoped ref to revoke state |
| `GET /api/configs` | | List configs |
| `PUT /api/configs` | `{"key":"k","value":"v"}` | Store config; default lifecycle is `VE:LOCAL:*` + `active` |
| `DELETE /api/configs/{key}` | | Delete config |
| `POST /api/rekey` | | Blocked (`403`) |
| `GET /api/status` | | Vault identity, `key_version`, `mode=vault` status |
| `GET /health` | | Health check |

### List vs Meta Read Model

LocalVault intentionally separates its read model into two tiers:

- `GET /api/secrets`
  - Lightweight local inventory only.
  - Returns name / ref / scope / status / version / updated timestamp.
  - Does not serve operator-wide search, cross-vault list, or binding count (those are KeyCenter catalog responsibilities).
- `GET /api/secrets/meta/{name}`
  - Single-secret detail view.
  - Includes `display_name`, `description`, `tags_json`, `origin`, `class`.
  - Includes `last_rotated_at`, `last_revealed_at`.
  - Includes companion field metadata (`field_role`, `display_name`, `masked_by_default`, `required`, `sort_order`).

The canonical list/search source for operator inventory is the KeyCenter operator catalog. LocalVault serves as the per-secret detail and ciphertext-owner metadata source.

## Cron / Rebind

```bash
VEILKEY_CONTEXT_FILE=/path/to/.veilkey/context.json veilkey-localvault cron tick
veilkey-localvault rebind --key-version 9
```

- `cron tick`
  - Reports the current LocalVault identity and `managed_paths` to KeyCenter via heartbeat.
  - Pulls the KeyCenter `GLOBAL` function registry into the local `functions` table before heartbeat.
  - If a planned rotation is scheduled, automatically applies the new `key_version` and retries the heartbeat within the same tick.
  - Re-reads `node_info.version` from DB immediately before each heartbeat so that a just-applied rotation value is never stale in memory.
  - Exits non-zero if KeyCenter returns `rebind_required` or `blocked`.
- `rebind --key-version`
  - After a human-approved rebind, updates the local `node_info.version` to the new key version.
  - A service restart and heartbeat re-registration must follow.

Tracked-ref sync and heartbeat share the same effective KeyCenter URL resolution. Payloads include `vault_node_uuid` as the primary identifier with `node_id` sent alongside as a compatibility alias.

The current operational default for new secret storage is `TEMP / temp`. Use `activate` to promote a TEMP secret to `LOCAL` or `EXTERNAL`.

## Testing

```bash
go test ./...
bash tests/test_ci_deploy_rules.sh
```

## MR Rule

- Runtime, deploy, install, CLI, and API behavior changes must add focused regression tests in the same MR.
- User-facing or operator-facing behavior changes must update README/docs in the same MR.
- The repo CI runs `tests/test_mr_guard.sh` and `scripts/check-mr-guard.sh` to block weak MRs.
- Platform-common policy lives in `tests/policy/project_registry_policy.sh`: public projects must keep package/container publishing private unless a platform adapter says otherwise.
- The current GitLab adapter is `tests/test_gitlab_project_settings.sh`; it enforces `container_registry_access_level=private` when a maintainer token is available.

## License

MIT License
