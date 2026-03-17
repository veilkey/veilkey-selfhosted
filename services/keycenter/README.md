# VeilKey KeyCenter

`keycenter` is the central self-hosted VeilKey control plane.

It tracks LocalVault inventory, policy, lifecycle, and orchestration state. It is not a generic plaintext secret bucket.

## Product Position

VeilKey is split into:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - `installer`
  - `keycenter`
  - `localvault`
  - `cli`
  - `proxy`

## Responsibilities

This component owns:

- LocalVault inventory
- runtime identity tracking
- policy and lifecycle decisions
- orchestration endpoints
- central UI and control-plane APIs

## Update Control

The admin settings page can expose a controlled update action when the server is configured with a server-side update script.

Required environment variables for the KeyCenter process:

- `VEILKEY_UPDATE_SCRIPT`
  - absolute path to the update script to execute
- `VEILKEY_PRODUCT_VERSION`
  - current deployed version reported in the UI

Optional environment variables:

- `VEILKEY_UPDATE_WORKDIR`
- `VEILKEY_UPDATE_TIMEOUT`

This repository includes a reference script:

- [`scripts/update-selfhosted.sh`](./scripts/update-selfhosted.sh)

The UI never accepts an arbitrary command from the browser. It only starts the configured server-side script.

KeyCenter is specifically responsible for:

- Tracking the actual `scope` / `status` of each LocalVault secret storage response
- Returning `VK:TEMP:*` / `status=temp` for new secret creation by default, while preserving existing lifecycle on active secret updates
- Exposing LocalVault secret query/list responses using scoped token contracts (`VK:{SCOPE}:{REF}`)
- Maintaining scoped canonical `VE:{SCOPE}:{KEY}` contracts for LocalVault config save/query/list responses
- Treating `localvault` strictly as a ciphertext store
- Managing LocalVault inventory by `vault_name:vault_hash` and `key_version`
- Managing `managed_paths` sent by each LocalVault in its heartbeat as inventory metadata
- Using `vault_node_uuid` (compatibility alias: `node_id`) / `vault_hash` as the actual ownership identifiers; `managed_paths` is a descriptive path list only
- Mirroring LocalVault lifecycle transitions via tracked ref sync; direct `VK:LOCAL:*` resolve uses this tracked ref state
- Escalating a LocalVault to `rebind_required` on key version mismatch or suspected reset, then transitioning to `blocked` on repeated reconnection
- Scheduling planned rotation via `POST /api/agents/rotate-all`, which LocalVault cron ticks apply automatically
- Escalating `rotation_required` LocalVaults through `1min -> 3min -> 10min` stages, then marking them `blocked` / `rotation_timeout` and auto-excluding them from the next planned rotation
- Central policy, encryption/decryption, and tracking; plaintext handling and lifecycle decisions are driven by upstream operator boundaries
- Not providing direct `/api/secrets*` plaintext CRUD
- Enforcing `^[A-Z_][A-Z0-9_]*$` naming for new secret/config names

## Why KeyCenter Exists

KeyCenter solves the following problems:

- In environments with many similar services, local secret contexts bleed into each other
- Old LocalVault identities can reattach and reopen stale secret contexts
- TEMP / LOCAL / EXTERNAL lifecycle must be tracked centrally
- Operational policies like planned rotation, rebind, and block are impractical to maintain independently on each local node

Even though each LocalVault holds its own secrets, KeyCenter decides "who is who", "what state they are in", and "whether re-registration is allowed".

## Identity Terms

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of a LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable human-readable vault identifier |
| `vault_runtime_hash` | current runtime binding hash |
| `agent_hash` | internal compatibility alias for `vault_runtime_hash` |

## Related Components

- `installer`
  - installs and verifies KeyCenter runtimes
- `localvault`
  - agent runtime reporting into KeyCenter
- `cli`
  - operator-facing entrypoint
- `proxy`
  - outbound enforcement for self-hosted workloads

## Install Architecture

- [`docs/web-install-architecture.md`](./docs/web-install-architecture.md)
  - target architecture for web-first install and centralized runtime management

## Architecture

```
                  Root Node (host)
                  +-- operator/plaintext ingress
                  +-- scoped canonical ref tracking/resolution (VK:{SCOPE}:{ref}, VE:{SCOPE}:{key})
                  +-- children/inventory management
                 /        |        \
           Child A    Child B    Child C   (each LXC/container)
           +-- local ciphertext store    +-- heartbeat -> root
           +-- local secret/config API   +-- resolve via parent
           +-- heartbeat -> root
```

| Command | Description |
|---------|-------------|
| `veilkey-keycenter` | Start server (after init) |
| `veilkey-keycenter init --root` | Initialize root node |
| `veilkey-keycenter init --child` | Initialize child/localvault node (auto-registers with parent) |

> Platform-specific integration (LXC env sync, etc.) is managed alongside veilroot boundary assets in `cli`.

## Installation

### Binary Build

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-keycenter ./cmd/main.go
```

### Root Node Initialization

```bash
# Interactive:
veilkey-keycenter init --root
# Or via stdin:
echo "your-password" | veilkey-keycenter init --root
```

Output:
```
Generating salt...
Initializing database...
Generating root KEK...
Generating root DEK...
VeilKey HKM initialized (root node).
```

### Child Node Initialization (one command)

```bash
veilkey-keycenter init --child \
  --parent http://your-keycenter:10180 \
--label my-service \
  --install
```

The `--install` flag automatically creates and starts a systemd service.

Output:
```
Generating salt...
Initializing database...
Registering with parent...
Storing encrypted DEK...
Saving node info...
Creating systemd service...
VeilKey HKM initialized (child node).
```

### Docker Container

```bash
# Root node
docker run -d --name veilkey \
  -p 10180:10180 \
  -v veilkey-data:/data \
-v /opt/veilkey/password:/run/secrets/veilkey_password:ro \
  -e VEILKEY_MODE=root \
  veilkey-keycenter:latest

# Child node
docker run -d --name veilkey \
  -p 10180:10180 \
  -v veilkey-data:/data \
-v /opt/veilkey/password:/run/secrets/veilkey_password:ro \
  -e VEILKEY_MODE=child \
  -e VEILKEY_PARENT_URL=http://your-keycenter:10180 \
  -e VEILKEY_LABEL=my-service \
  veilkey-keycenter:latest
```

### Docker Compose

```yaml
services:
  veilkey:
    image: veilkey-keycenter:latest
    ports:
      - "10180:10180"
    volumes:
      - veilkey-data:/data
    environment:
VEILKEY_PASSWORD_FILE: /run/secrets/veilkey_password
      VEILKEY_MODE: root          # 또는 child
      # VEILKEY_PARENT_URL: ...   # child 모드 시 필수
      # VEILKEY_LABEL: ...        # child 모드 시 권장
    restart: unless-stopped

volumes:
  veilkey-data:
```

## API

### Secrets / Agent Plaintext Flow

KeyCenter is not a central plaintext secret store.
Plaintext input is accepted only from explicit agent routes; direct `/api/secrets*` CRUD is not supported.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST /api/agents/{agent}/secrets` | `{"name":"key","value":"val"}` | Store plaintext input; returns canonical `token`, `scope`, `status`, and vault metadata |
| `GET /api/agents/{agent}/secrets` | | List secret inventory based on LocalVault metadata |
| `GET /api/agents/{agent}/secrets/{name}` | | Retrieve plaintext by vault hash |
| `GET /api/resolve-agent/{token}` | | Direct resolve by vault hash + ref token (internal/ops use) |
| `GET /api/resolve/{ref}` | | Resolve tracked ref / vault storage path |
| `POST /api/agents/rotate-all` | `{}` | Schedule planned rotation for eligible LocalVaults, incrementing `key_version` by 1 |

Ownership boundaries:

- **Operator boundary**: plaintext input and host operational context
- **LocalVault**: per-node ciphertext storage and actual secret/config API
- **KeyCenter**: central policy, tracked refs, inventory, lifecycle, decrypt/resolve orchestration

`name` and `key` accept uppercase identifiers only:
- Allowed: `GITLAB_PAT_PUBLIC`, `VEILKEY_KEYCENTER_URL`
- Rejected: `gitlab_pat`, `Bad_Key`, `BAD@KEY`

### Node Management

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET /api/node-info` | | Current HKM node info (includes `tracked_refs_count`) |
| `GET /api/children` | | List registered child nodes |
| `POST /api/register` | | Register child node (called automatically by `init --child`) |
| `DELETE /api/children/{node_id}` | | Delete child node (`vault_node_uuid` path alias) |
| `DELETE /api/agents/by-node/{node_id}` | | Deregister LocalVault agent; the installer purge calls this route before removing the host LocalVault to clean up inventory |
| `GET /api/configs` | | List VE (configs) held by self/all-in-one LocalVault |
| `GET /api/configs/{key}` | | Retrieve individual VE (config) from self/all-in-one LocalVault |
| `POST /api/configs` | | Store VE (config) in self/all-in-one LocalVault |
| `PUT /api/configs/bulk` | | Bulk store VE (configs) in self/all-in-one LocalVault |
| `DELETE /api/configs/{key}` | | Delete VE (config) from self/all-in-one LocalVault |

### Heartbeat

Child nodes send a heartbeat to the parent every 5 minutes by default (adjustable via `VEILKEY_HEARTBEAT_INTERVAL`).
The parent tracks each node's status (online/offline) in the children list.
A heartbeat must include at minimum:

- `vault_hash`
- `vault_name`
- `key_version`

KeyCenter maintains inventory based on these values and rejects a LocalVault whose version does not match.

Heartbeats also carry `managed_paths` metadata and manage rebind state:

- **`managed_paths`**
  - List of actual service paths the LocalVault is responsible for
  - Duplicate / overlap is allowed
  - Actual owner identification is based on `vault_node_uuid` (compatibility alias: `node_id`) / `vault_hash`
- **Tracked ref sync**
  - When a LocalVault performs a `TEMP -> LOCAL/EXTERNAL` promotion or other lifecycle change, the KeyCenter tracked ref is updated accordingly
  - Sync requests locate the current `vault_runtime_hash` owner by `vault_node_uuid`
  - This path must be alive for direct `VK:LOCAL:*` resolve to work without returning `404`
- **`key_version_mismatch` escalation**
  - First mismatch: `rebind_required`
  - Subsequent retries: `1min -> 3min -> 10min -> blocked`
  - However, if the LocalVault later sends a heartbeat with the correct `key_version`, temporary `rebind_required` / `blocked(key_version_mismatch)` states are automatically cleared
- **Planned rotation timeout**
  - If `rotation_required` is not resolved by the next heartbeat/tick, it escalates through `1min -> 3min -> 10min` stages
  - This escalation proceeds not only from `rotate-all` but also during regular inventory queries (`/api/agents`) and heartbeat boundaries
  - If never resolved: `blocked` + `block_reason=rotation_timeout`
  - A LocalVault blocked this way is auto-excluded from the next `POST /api/agents/rotate-all` target set
- **`blocked` state**
  - All resolve/save/get/config/migrate routes are blocked
  - The old identity cannot reopen normal routes until a human-approved rebind is performed

### Human-Approved Rebind

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST /api/agents/{agent}/approve-rebind` | | Approve rebind; issues a new `vault_hash` and incremented `key_version` |

After rebind approval, the LocalVault must update its local `node_info.version` with the new `key_version` and send another heartbeat.

### Operations

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST /api/unlock` | `{"password":"..."}` | Unlock a locked server |
| `POST /api/heartbeat` | `{"vault_node_uuid":"...","vault_hash":"...","vault_name":"...","key_version":1}` | Child-to-parent heartbeat (automatic; `node_id` alias accepted) |
| `GET /api/health` | | Health check |
| `GET /api/status` | | HKM status info (includes `tracked_refs_count`) |
| `POST /api/rekey` | `{"dek":"..."}` | Re-encrypt everything with a new DEK issued by the parent |
| `POST /api/set-parent` | `{"parent_url":"..."}` | Change parent URL |

### Tracked Ref Audit

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET /api/tracked-refs/audit` | | Aggregate `blocked`/`stale` status and reasons for tracked refs |

Tracked ref audit maintains only two top-level classes:

- **`blocked`**
  - Refs requiring immediate use-blocking or quarantine
- **`stale`**
  - Refs requiring central cleanup or ownership recovery

Current `stale.reason` values:

- `missing_owner` -- the tracked ref row itself has an empty owner vault hash
- `missing_agent` -- owner vault hash exists but is not present in the current inventory
- `duplicate_ref_id` -- more than one ref exists for the same `vault_hash + family + id` combination
- `agent_mismatch` -- the same `family + id` spans more than one vault hash

New top-level classes are added only when the required operational action genuinely differs. Until then, extend `blocked` or `stale.reason` instead.

## LXC Deploy

`scripts/deploy-lxc.sh` operates in the following sequence:

1. Builds a new binary locally.
2. Reads the target LXC's systemd unit path via `vibe_lxc_ops`.
3. Pushes the binary to the LXC and restarts the service.
4. Reads back the deployed binary's SHA256 from inside the LXC and verifies it matches the local build artifact.

The deploy is not considered successful if this verification fails.
This script must be run on a Proxmox host, and the CI deploy job must run on a `proxmox-host` runner.

## Environment Variables

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `VEILKEY_DB_PATH` | `/opt/veilkey/data/veilkey.db` | SQLite DB 경로 |
| `VEILKEY_ADDR` | `:10180` | 서버 바인드 주소 |
| `VEILKEY_PASSWORD_FILE` | (없음) | 마스터 패스워드 파일 경로 (mode 0600, 자동 언락용) |
| `VEILKEY_TRUSTED_IPS` | (없음) | 신뢰 IP 대역 (쉼표 구분 CIDR) |
| `VEILKEY_MODE` | (없음) | Docker 모드: `root` 또는 `child` |
| `VEILKEY_PARENT_URL` | (없음) | 부모 노드 URL (child 모드 시) |
| `VEILKEY_LABEL` | hostname | 노드 라벨 (child 모드 시) |
| `VEILKEY_EXTERNAL_URL` | 자동감지 | heartbeat에 보고할 자신의 URL |
| `VEILKEY_HEARTBEAT_INTERVAL` | `5m` | heartbeat 전송 주기 |
| `VEILKEY_HEARTBEAT_TIMEOUT` | `5s` | heartbeat HTTP 타임아웃 |
| `VEILKEY_TIMEOUT_CASCADE` | (기본) | cascade resolve 타임아웃 |
| `VEILKEY_TIMEOUT_PARENT` | (기본) | 부모 forward 타임아웃 |
| `VEILKEY_TIMEOUT_DEPLOY` | (기본) | 배포 관련 타임아웃 |

## Tests

```bash
go test ./...
```

- `tests/integration/removed_endpoints_test.go`
  - Verifies that KeyCenter does not serve local `/api/secrets*` or federation secret routes
- `internal/api/hkm_agent_secret_routes_test.go`
  - Verifies that direct `/api/secrets*` is empty and the only supported plaintext ingress is the `/api/agents/{agent}/secrets` route
- `internal/api/hkm_resolve_no_local_secret_test.go`
  - Verifies that `GET /api/resolve/VK:LOCAL:{ref}` returns `404` without local secret fallback, even when rows exist in the `secrets` table
- `internal/api/hkm_agent_secrets_test.go`
  - Verifies that LocalVault secret public responses use only scoped canonical tokens (`VK:{SCOPE}:{REF}`) and do not expose the legacy `VK:{ref}` format

## Identity Migration Checkpoint

- canonical operator-facing/runtime fields: `vault_node_uuid`, `vault_runtime_hash`
- compatibility aliases: `node_id`, `agent_hash`
- current DB columns and route surface still keep legacy names for compatibility and staged migration safety

## License

MIT License

## MR Rule

- runtime, deploy, install, CLI, API behavior changes must add focused regression tests in the same MR
- user-facing or operator-facing behavior changes must update README/docs in the same MR
- the repo CI runs `tests/test_mr_guard.sh` and `scripts/check-mr-guard.sh` to block weak MRs
