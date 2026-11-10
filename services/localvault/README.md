# VeilKey LocalVault

`localvault` is the canonical node-local VeilKey runtime.

It stores local ciphertext, configs, and runtime identity, and it executes node-local lifecycle actions under VaultCenter policy.

## Product Position

VeilKey self-hosted components:

- [`vaultcenter`](../vaultcenter) — central key management server
- `localvault` (this component) — node-local vault runtime
- [`veil-cli`](../veil-cli) — CLI tools (veil, veilkey-cli)

## Responsibilities

This component owns:

- local ciphertext and config storage
- runtime identity (`vault_node_uuid` / `vault_hash`) and heartbeat
- local resolve and decrypt flows
- bulk-apply execution
- `managed_paths` reporting to VaultCenter
- planned rotation, rebind, and blocked state reflection
- node-local policy enforcement requested by VaultCenter

This component does **not** own:

- plaintext input UX
- operator approval
- central encrypt/decrypt policy decisions

Those responsibilities belong to VaultCenter and the CLI.

## Identity Terms

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of the current LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable vault identifier |
| `vault_runtime_hash` | current VaultCenter runtime binding hash |
| `agent_hash` | internal API compatibility alias for `vault_runtime_hash` |

Identity rules:

- `.veilkey/context.json` uses `vault_node_uuid` as the primary key; `node_id` is read as a compatibility fallback.
- Operator-facing output centers on `vault_hash` / `vault_runtime_hash`.
- `agent_hash` appears only in internal API compatibility contexts.

## Related Components

- [`vaultcenter`](../vaultcenter) -- central control plane
- [`install/`](../../install/README.md) -- platform-specific install guides
- [`veil-cli`](../veil-cli) -- CLI tools (veil, veilkey-cli)

## VaultCenter URL Resolution

Heartbeat and tracked-ref sync share a single effective VaultCenter URL, resolved by this precedence:

1. `VEILKEY_VAULTCENTER_URL` environment variable
2. `VEILKEY_VAULTCENTER_URL` stored in DB config

If multiple sources contain differing values, a drift warning is logged at startup and runtime.

## Security Boundary

- Requests to configs, lifecycle actions, bulk-apply precheck/execute, secret inventory, functions, node-info, lock, diagnostics, TLS info, and post-install settings require `Authorization: Bearer <agent_secret>` issued by VaultCenter. Trusted IPs alone are not credentials; protected routes fail closed before registration.
- `/health`, `/ready`, and `/api/status` remain credential-free liveness/readiness endpoints. `/api/status` returns only the health state; use authenticated `/api/node-info` for vault identity and counts.
- Set `VEILKEY_TRUSTED_IPS` explicitly to the VaultCenter source address, not a private-network range or localhost. An empty list denies IP-restricted requests. Before initialization, the wizard and install APIs are available only to configured trusted addresses; health remains public. If temporary setup access is needed, explicitly configure the operator's address and remove it after setup.
- Generic config create/update/bulk/delete APIs reject reserved `VEILKEY_` keys. Use the authenticated install settings API or deployment configuration to change the VaultCenter URL.
- Bulk-apply has no enabled targets or hooks by default. Opt in using `VEILKEY_BULK_APPLY_ALLOWED_PATHS` (comma-separated absolute paths) and `VEILKEY_BULK_APPLY_ALLOWED_HOOKS` (comma-separated `name:command` entries). Request-supplied `hook_env` and raw `gitlab.rb` writes are rejected. Hooks receive only server-generated workflow metadata in addition to their service environment.
- Ciphertext writes, including companion fields, require a 12-byte AES-GCM nonce and at least the 16-byte authentication tag (lengths are checked after base64 decoding).
- Legacy plaintext databases are migrated only after their stored DEK authenticates the supplied KEK. A failed password leaves the legacy database unchanged; migration backups are removed only after a successful unlock.
- TLS certificate verification is enabled by default. Configure `VEILKEY_TLS_CA` for a private CA; do not use `VEILKEY_TLS_INSECURE=1` for multi-host deployments.

- Plaintext encryption/decryption is not performed in LocalVault. LocalVault is a ciphertext store; plaintext handling belongs to VaultCenter.
- The following endpoints are blocked (`403`):
  - `POST /api/secrets`, `GET /api/secrets/{name}`, `GET /api/resolve/{ref}`, `POST /api/encrypt`, `POST /api/rekey`
- Blocked-state read/use paths must be **fail-closed**: `GET /api/cipher/{ref}`, `GET /api/configs/{key}`, and explicit lifecycle transition endpoints return `423` with the canonical scoped ref.
- New secret storage defaults to scoped ref `TEMP/temp`.
- Re-storing a secret that is already `LOCAL/active` or `EXTERNAL/active` preserves the existing lifecycle.
- Companion field storage is only allowed when the parent secret is `VK:LOCAL` or `VK:EXTERNAL` and in `active` state.
- `activate`, `archive`, `block`, and `revoke` attempt a VaultCenter tracked-ref sync after the lifecycle change. If sync fails the lifecycle change is kept, and the API response includes `sync_status=degraded`, `sync_target`, and `sync_error` to surface partial failure.

## Features

- Local ciphertext/metadata storage
- Metadata retrieval via `/api/cipher` and `/api/secrets/meta/{name}`
- Companion field (`OTP`, `LOGIN_ID`, `KEY_PASSWORD`, etc.) metadata/cipher storage for `VK:LOCAL` / `VK:EXTERNAL` secrets
- Local configs CRUD (plaintext key-value)
- Automatic VaultCenter registration and heartbeat
- Vault identity reporting based on `vault_name:vault_hash`
- `managed_paths` reporting from `.veilkey/context.json` or `VEILKEY_MANAGED_PATHS`

### Function Catalog

LocalVault stores vault-local function rows:

- One function = one DB row.
- Scope is restricted to `GLOBAL`, `VAULT`, `LOCAL`, or `TEST`.
- `vars_json` stores per-variable `ref` and `LOCAL` | `EXTERNAL` class.
- `GLOBAL` functions are pull-synced from the VaultCenter SSOT as local materialized copies. The local API does not allow direct creation or deletion of `GLOBAL` rows; they are managed exclusively by VaultCenter sync.
- Rows can be queried with a scope filter, e.g. `GET /api/functions?scope=TEST`.
- `scope=TEST` functions are auto-deleted during `cron tick` once `created_at + 1h` has elapsed.

## Installation

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-localvault .
```

First-time initialization requires a valid registration token issued from the VaultCenter keycenter UI, even on trusted networks. `--center` supplies the URL only; it does not replace the token. Existing registrations keep working without a new token.

```bash
veilkey-localvault init --root \
  --token '<REG_TOKEN>' \
  --center https://vaultcenter:10181
```

The unlock key is auto-generated; no password is read from stdin. Web setup (`POST /api/install/init`) must submit the encoded token in `registration_token` as well. See [registration setup](../../docs/setup/localvault/registration.md).

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

Deploy via Docker container. See the `Dockerfile` in this directory.


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
| `POST /api/cipher` | `{"name":"key","ref":"...","ciphertext":"...","nonce":"..."}` | Store VaultCenter-encrypted ciphertext. New secret defaults to `VK:TEMP:*` + `temp`; updating an existing secret preserves its lifecycle |
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
| `GET /api/status` | | Credential-free health state only |
| `GET /api/node-info` | | Agent-authenticated vault identity and inventory counts |
| `GET /health` | | Health check |

### List vs Meta Read Model

LocalVault intentionally separates its read model into two tiers:

- `GET /api/secrets`
  - Lightweight local inventory only.
  - Returns name / ref / scope / status / version / updated timestamp.
  - Does not serve operator-wide search, cross-vault list, or binding count (those are VaultCenter catalog responsibilities).
- `GET /api/secrets/meta/{name}`
  - Single-secret detail view.
  - Includes `display_name`, `description`, `tags_json`, `origin`, `class`.
  - Includes `last_rotated_at`, `last_revealed_at`.
  - Includes companion field metadata (`field_role`, `display_name`, `masked_by_default`, `required`, `sort_order`).

The canonical list/search source for operator inventory is the VaultCenter operator catalog. LocalVault serves as the per-secret detail and ciphertext-owner metadata source.

## Cron / Rebind

```bash
VEILKEY_CONTEXT_FILE=/path/to/.veilkey/context.json veilkey-localvault cron tick
veilkey-localvault rebind --key-version 9
```

- `cron tick`
  - Reports the current LocalVault identity and `managed_paths` to VaultCenter via heartbeat.
  - Pulls the VaultCenter `GLOBAL` function registry into the local `functions` table before heartbeat.
  - If a planned rotation is scheduled, automatically applies the new `key_version` and retries the heartbeat within the same tick.
  - Re-reads `node_info.version` from DB immediately before each heartbeat so that a just-applied rotation value is never stale in memory.
  - Exits non-zero if VaultCenter returns `rebind_required` or `blocked`.
- `rebind --key-version`
  - After a human-approved rebind, updates the local `node_info.version` to the new key version.
  - A service restart and heartbeat re-registration must follow.

Tracked-ref sync and heartbeat share the same effective VaultCenter URL resolution. Payloads include `vault_node_uuid` as the primary identifier with `node_id` sent alongside as a compatibility alias.

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

AGPL-3.0 License

## Security

Please report vulnerabilities privately through the repository's [security policy](https://github.com/veilkey/veilkey-selfhosted/security/policy), not through public issues.
