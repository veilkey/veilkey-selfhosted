# VeilKey LocalVault

`localvault` is the canonical node-local VeilKey runtime.

It stores local ciphertext, configs, and runtime identity, and it executes node-local lifecycle actions under KeyCenter policy.

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

- local secret and config storage
- runtime identity and heartbeat
- local resolve and decrypt flows
- bulk-apply execution
- node-local policy enforcement requested by KeyCenter

## Identity Terms

| Term | Meaning |
|------|---------|
| `vault_node_uuid` | UUID of the current LocalVault instance |
| `node_id` | compatibility alias of `vault_node_uuid` |
| `vault_hash` | stable vault identifier |
| `vault_runtime_hash` | current KeyCenter runtime binding hash |
| `agent_hash` | internal compatibility alias for `vault_runtime_hash` |

## Related Components

- `keycenter`
  - central control plane
- `installer`
  - installs and verifies LocalVault targets
- `cli`
  - operator entrypoint and local tooling
원칙:
- `.veilkey/context.json`은 `vault_node_uuid`를 우선 사용하고, 없으면 `node_id`를 호환 alias로 읽습니다.
- operator-facing 출력은 `vault_hash` / `vault_runtime_hash` 중심으로 설명합니다.
- `agent_hash`는 내부 API 호환 설명에만 남깁니다.

`scripts/deploy-lxc.sh` 는 Proxmox host에서만 실행해야 하며, CI deploy job도 `proxmox-host` runner에서 실행되어야 합니다.
`docker` 이미지를 못 밀어도 LXC runtime deploy는 계속 진행되도록 CI를 분리합니다.

> **이 레포는 LocalVault (로컬 노드)입니다.** 관련 레포:
> - [veilkey-keycenter](https://github.com/veilkey/veilkey-keycenter) — 중앙 관리 서버 (KeyCenter)
> - **veilkey-localvault** (이 레포) — 각 노드의 로컬 secrets + configs 저장
> - `veilkey-hostvault/plugins/` — 플랫폼별 연동 (Proxmox, Docker 등). CLI/host 운영 경로와 함께 관리
> - [veilkey-cli](https://github.com/veilkey/veilkey-cli) — CLI 도구
> - [veilkey-installer](https://github.com/veilkey/veilkey-installer) — 설치 스크립트

## 기능

- 로컬 ciphertext/metadata 저장
- `/api/cipher`, `/api/secrets/meta/{name}` 기준 metadata 제공
- `VK:LOCAL`/`VK:EXTERNAL` secret에 companion field (`OTP`, `LOGIN_ID`, `KEY_PASSWORD` 등) metadata/cipher 저장
- 로컬 configs CRUD (평문 key-value)
- KeyCenter(부모) 자동 등록 + heartbeat
- `vault_name:vault_hash` 기반 vault identity 보고
- heartbeat / tracked-ref sync 대상은 아래 precedence로 하나의 effective KeyCenter URL을 결정합니다.
  - `VEILKEY_KEYCENTER_URL` (env)
  - `VEILKEY_KEYCENTER_URL` (DB config)
  - `VEILKEY_HUB_URL` (env, legacy alias)
  - `VEILKEY_HUB_URL` (DB config, legacy alias)
- 서로 다른 값이 동시에 존재하면 startup/runtime 로그에 drift warning 을 남깁니다.
- `.veilkey/context.json` 또는 `VEILKEY_MANAGED_PATHS` 기준으로 `managed_paths`를 KeyCenter에 보고합니다.
- `veilkey-localvault cron tick` 으로 1회 heartbeat/retry를 실행할 수 있습니다.
- KeyCenter가 planned rotation 을 예약하면 `veilkey-localvault cron tick` 이 `rotation_required` 응답을 보고 로컬 `key_version`을 자동 갱신한 뒤 heartbeat를 재시도합니다.
- LocalVault DB는 vault-local function row를 저장할 수 있습니다.
  - 함수 1개 = DB row 1개
  - scope 는 `GLOBAL|VAULT|LOCAL|TEST` 네 가지로만 고정됩니다.
  - `vars_json` 안에 변수별 `ref`와 `LOCAL|EXTERNAL` class를 함께 저장합니다.
  - `GLOBAL` 함수는 KeyCenter SSOT를 local materialized copy 로 pull sync 합니다.
  - 로컬 API는 `GLOBAL` 함수의 직접 생성/삭제를 허용하지 않습니다. `GLOBAL` row는 KeyCenter sync 전용입니다.
  - `GET /api/functions?scope=TEST` 처럼 scope filter 로 row를 조회할 수 있습니다.
  - `scope=TEST` 함수는 `veilkey-localvault cron tick` 실행 시 `created_at + 1h` 기준으로 자동 삭제됩니다.
- heartbeat는 매번 DB의 최신 `node_info.version`을 다시 읽어 전송하므로, planned rotation 직후 stale in-memory version으로 다시 mismatch를 만들지 않습니다.
- human-approved rebind 시 `veilkey-localvault rebind --key-version <n>` 으로 로컬 version을 갱신합니다.

## 역할

LocalVault는 다음 역할에 집중합니다.

- 로컬 문맥의 ciphertext 저장
- `vault_node_uuid` / `vault_hash` 기반 identity 유지
- `managed_paths` metadata를 KeyCenter에 보고
- planned rotation, rebind, blocked 상태를 로컬에서 반영

반대로 LocalVault는 다음을 직접 주도하지 않습니다.

- plaintext 입력 UX
- operator approval
- 중앙 암복호화 정책 결정

이 역할은 HostVault와 KeyCenter가 나눠서 담당합니다.

## 현재 보안 경계

- plaintext 암호화/복호화는 localvault에서 직접 수행하지 않습니다.
- `POST /api/secrets`, `GET /api/secrets/{name}`, `GET /api/resolve/{ref}`, `POST /api/encrypt`, `POST /api/rekey` 는 차단됩니다.
- localvault는 ciphertext 저장소이며, plaintext 처리는 keycenter에서 수행해야 합니다.
- 새 secret 저장 결과는 scoped `TEMP/temp` ref를 기준으로 봅니다.
- 이미 `LOCAL/active` 또는 `EXTERNAL/active` 인 secret을 같은 이름으로 다시 저장하면 기존 lifecycle을 유지합니다.
- companion field 저장은 parent secret이 `VK:LOCAL` 또는 `VK:EXTERNAL` 이고 `active` 인 경우에만 허용합니다.
- `activate`, `archive`, `block`, `revoke` 는 lifecycle 변경 후 KeyCenter tracked ref sync 를 시도합니다.
- sync 가 실패해도 lifecycle 변경 자체는 유지되며, API 응답에 `sync_status=degraded`, `sync_target`, `sync_error` 를 함께 반환해 partial failure 를 드러냅니다.
- blocked 상태의 read/use 경로는 fail-closed 여야 하며, `GET /api/cipher/{ref}`, `GET /api/configs/{key}`, explicit lifecycle transition 경로는 `423`과 canonical scoped ref를 반환해야 합니다.

## 설치

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-localvault .
```

초기화는 KeyCenter의 `init --child` 명령으로 자동 수행됩니다:

```bash
veilkey-keycenter init --child \
  --parent http://KEYCENTER_IP:10180 \
  --label my-service \
  --install
```

프로젝트/서비스 루트에는 `.veilkey/context.json` 을 두고, cron은 이 context를 기준으로 heartbeat를 보냅니다.

예시:

```json
{
  "version": 1,
  "managed_path": "/var/www/services/demo",
  "context_id": "auto-generated-uuid",
  "vault_node_uuid": "",
  "node_id": ""
}
```

- `context_id` 는 로컬 context 파일 식별용 값입니다.
- 실제 LocalVault owner 식별은 `vault_node_uuid` / `vault_hash` 기준입니다.
- `vault_node_uuid` 는 live 등록 전에는 비어 있을 수 있고, 등록 뒤 현재 LocalVault identity와 맞춰 사용합니다.
- `node_id` 는 기존 호환을 위한 alias 입니다.

## API

| Method | Endpoint | 설명 |
|--------|----------|------|
| `GET /api/secrets` | | 시크릿 목록 |
| `GET /api/secrets/meta/{name}` | | plaintext 없이 ref/meta 조회 |
| `POST /api/secrets` | `{"name":"key","value":"val"}` | 차단됨 (`403`) |
| `GET /api/secrets/{name}` | | 차단됨 (`403`) |
| `GET /api/resolve/{ref}` | | 차단됨 (`403`) |
| `POST /api/encrypt` | `{"plaintext":"val"}` | 차단됨 (`403`) |
| `GET /api/cipher/{ref}` | | ref 기준 ciphertext/nonce 조회 |
| `GET /api/cipher/{ref}/fields/{field}` | | LOCAL/EXTERNAL secret companion field ciphertext/nonce 조회 |
| `POST /api/cipher` | `{"name":"key","ref":"...","ciphertext":"...","nonce":"..."}` | KeyCenter가 암호화한 ciphertext 저장, 새 secret 기본 lifecycle은 `VK:TEMP:*` + `temp`, 기존 secret update는 기존 lifecycle 유지 |
| `POST /api/secrets/fields` | `{"name":"GITHUB_KEY","fields":[{"key":"OTP","type":"otp","ciphertext":"...","nonce":"..."}]}` | `VK:LOCAL`/`VK:EXTERNAL` + `active` secret에 companion field 저장/수정 |
| `DELETE /api/secrets/{name}/fields/{field}` | | active `VK:LOCAL`/`VK:EXTERNAL` secret companion field 삭제 |
| `POST /api/reencrypt` | `{"ciphertext":"VK:TEMP:deadbeef"}` | explicit transition 경로의 scoped ref 검증 + canonical ref 반환 |
| `POST /api/activate` | `{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}` | explicit TEMP ref를 `VK:LOCAL:ref` 또는 `VK:EXTERNAL:ref`로 활성화 |
| `POST /api/active` | `{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}` | `activate`의 호환 alias |
| `POST /api/archive` | `{"ciphertext":"VK:LOCAL:deadbeef"}` | scoped ref를 archive 상태로 전환 |
| `POST /api/revoke` | `{"ciphertext":"VK:LOCAL:deadbeef"}` | scoped ref를 revoke 상태로 전환 |
| `GET /api/configs` | | configs 목록 |
| `PUT /api/configs` | `{"key":"k","value":"v"}` | config 저장, 기본 lifecycle은 `VE:LOCAL:*` + `active` |
| `DELETE /api/configs/{key}` | | config 삭제 |
| `POST /api/rekey` | | 차단됨 (`403`) |
| `GET /api/status` | | vault identity, `key_version`, `mode=vault`, 상태 조회 |
| `GET /health` | | 헬스 체크 |

### List vs Meta Read Model

LocalVault의 read model 책임은 intentionally 두 층으로 나눕니다.

- `GET /api/secrets`
  - lightweight local inventory only
  - name / ref / scope / status / version / updated timestamp 중심
  - operator-wide search, cross-vault list, binding count 같은 중앙 catalog 역할은 맡지 않음
- `GET /api/secrets/meta/{name}`
  - 단일 secret detail 전용
  - `display_name`, `description`, `tags_json`, `origin`, `class`
  - `last_rotated_at`, `last_revealed_at`
  - companion field metadata (`field_role`, `display_name`, `masked_by_default`, `required`, `sort_order`)

즉 operator inventory의 canonical list/search source는 KeyCenter operator catalog이고, LocalVault는 per-secret detail / ciphertext owner metadata source로 동작합니다.

## Cron / Rebind

```bash
VEILKEY_CONTEXT_FILE=/path/to/.veilkey/context.json veilkey-localvault cron tick
veilkey-localvault rebind --key-version 9
```

- `cron tick`
  - 현재 LocalVault identity와 `managed_paths`를 KeyCenter heartbeat로 보고합니다.
  - 먼저 KeyCenter `GLOBAL` 함수 레지스트리를 local `functions` 테이블로 pull sync 합니다.
  - planned rotation 이 예약된 경우 새 `key_version`을 자동 적용하고 같은 tick 안에서 heartbeat를 다시 보냅니다.
  - heartbeat 직전마다 최신 `node_info.version`을 다시 읽어 planned rotation 반영값을 우선 사용합니다.
  - KeyCenter가 `rebind_required` 또는 `blocked`를 반환하면 non-zero로 실패합니다.
- `rebind --key-version`
  - human-approved rebind 뒤 로컬 `node_info.version`을 새 key version으로 맞춥니다.
  - 이후 service restart + heartbeat 재등록이 뒤따라야 합니다.

Tracked ref sync 와 heartbeat 는 같은 effective KeyCenter URL resolution 을 공유하며, payload에는 `vault_node_uuid`를 우선 넣고 `node_id`를 호환 alias로 함께 보냅니다.

현재 운영 기본값은 새 secret 저장 시 `TEMP / temp` 입니다.
`activate` 는 이 TEMP secret을 `LOCAL` 또는 `EXTERNAL` 로 승격할 때 사용합니다.

## 테스트

```bash
go test ./...
bash tests/test_ci_deploy_rules.sh
```

## License

MIT License
## MR Rule

- runtime, deploy, install, CLI, API behavior changes must add focused regression tests in the same MR
- user-facing or operator-facing behavior changes must update README/docs in the same MR
- the repo CI runs `tests/test_mr_guard.sh` and `scripts/check-mr-guard.sh` to block weak MRs
- platform-common policy lives in `tests/policy/project_registry_policy.sh`: public projects must keep package/container publishing private unless a platform adapter says otherwise
- the current GitLab adapter is `tests/test_gitlab_project_settings.sh`; it enforces `container_registry_access_level=private` when a maintainer token is available
