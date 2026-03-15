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
> - `veilkey-localvault`
> - `veilkey-cli`
> - `veilkey-proxy`

현재 기준 역할:

- localvault secret 저장 응답의 실제 `scope` / `status` 를 그대로 추적한다
- 새 secret 생성은 기본적으로 `VK:TEMP:*` / `status=temp` 를 반환하고, 기존 active secret update는 기존 lifecycle을 유지한다
- localvault secret 조회/목록 응답은 localvault가 가진 실제 `scope`/`status`를 scoped token 계약(`VK:{SCOPE}:{REF}`)으로 노출한다
- localvault config 저장/조회/목록 응답은 scoped canonical `VE:{SCOPE}:{KEY}` 계약을 유지한다
- `localvault`는 ciphertext 저장소로만 다룬다
- `vault_name:vault_hash`와 `key_version`을 기준으로 localvault inventory를 관리한다
- 각 LocalVault가 heartbeat에 실어 보낸 `managed_paths`를 inventory metadata로 관리한다
- 실제 ownership 식별자는 `vault_node_uuid`(호환 alias: `node_id`) / `vault_hash`이며, `managed_paths`는 설명용 경로 목록이다
- LocalVault lifecycle 전환은 tracked ref sync 로 mirrored 되며, direct `VK:LOCAL:*` resolve 는 이 tracked ref 상태를 사용한다
- key version mismatch 또는 reset 의심 시 localvault를 `rebind_required`로 승격하고, 반복 재접속 시 `blocked`로 전환한다
- 운영 유지보수용 planned rotation 은 `POST /api/agents/rotate-all` 로 예약하고, LocalVault cron tick 이 이를 자동 적용한다
- `rotation_required` 상태가 오래 남은 LocalVault는 `1분 -> 3분 -> 10분` 단계로 승격 후 `blocked` / `rotation_timeout` 처리되며, 다음 planned rotation 대상에서 자동 제외된다
- plaintext 처리와 lifecycle 결정은 upstream operator boundaries가 주도하고, KeyCenter는 중앙 정책/암복호화/추적을 맡는다
- direct `/api/secrets*` plaintext CRUD 는 제공하지 않는다
- 신규 secret/config 이름은 `^[A-Z_][A-Z0-9_]*$` 규칙만 허용한다

## 왜 필요한가

KeyCenter는 다음 문제를 해결하기 위해 존재합니다.

- 비슷한 서비스가 많은 환경에서 로컬 비밀번호 문맥이 섞이는 문제
- 예전 LocalVault identity가 다시 붙어 옛 비밀번호 문맥을 여는 문제
- TEMP / LOCAL / EXTERNAL lifecycle을 중앙에서 추적해야 하는 문제
- planned rotation, rebind, block 같은 운영 정책을 로컬마다 제각각 두기 어려운 문제

즉 LocalVault가 각자 비밀번호를 들고 있더라도, "누가 누구인지", "어떤 상태인지", "재등록을 허용할지"는 KeyCenter가 결정합니다.

## 구조

```
                  Root Node (host)
                  ├── operator/plaintext ingress
                  ├── scoped canonical ref 추적/해석 (`VK:{SCOPE}:{ref}`, `VE:{SCOPE}:{key}`)
                  └── children/inventory 관리
                 /        |        \
           Child A    Child B    Child C   (각 LXC/컨테이너)
           ├── 로컬 ciphertext store   ├── heartbeat → root
           ├── local secret/config API └── 부모 통해 resolve
           └── heartbeat → root
```

| 명령 | 설명 |
|------|------|
| `veilkey-keycenter` | 서버 실행 (init 후) |
| `veilkey-keycenter init --root` | Root 노드 초기화 |
| `veilkey-keycenter init --child` | Child/localvault 노드 초기화 (부모 자동 등록) |

> 플랫폼별 연동 (LXC env 동기화 등)은 `veilkey-hostvault/plugins/`에서 함께 관리합니다.

## 설치

### 바이너리 빌드

```bash
CGO_ENABLED=1 go build -ldflags="-s -w" -o veilkey-keycenter ./cmd/main.go
```

### Root 노드 초기화

```bash
# Interactive:
veilkey-keycenter init --root
# Or via stdin:
echo "your-password" | veilkey-keycenter init --root
```

출력:
```
Generating salt...
Initializing database...
Generating root KEK...
Generating root DEK...
VeilKey HKM initialized (root node).
```

### Child 노드 초기화 (원 커맨드)

```bash
veilkey-keycenter init --child \
  --parent http://your-keycenter:10180 \
  --label my-service \
  --install
```

`--install` 플래그는 systemd 서비스를 자동 생성하고 시작합니다.

출력:
```
Generating salt...
Initializing database...
Registering with parent...
Storing encrypted DEK...
Saving node info...
Creating systemd service...
VeilKey HKM initialized (child node).
```

### Docker 컨테이너

```bash
# Root 노드
docker run -d --name veilkey \
  -p 10180:10180 \
  -v veilkey-data:/data \
  -v /opt/veilkey/password:/run/secrets/veilkey_password:ro \
  -e VEILKEY_MODE=root \
  veilkey-keycenter:latest

# Child 노드
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

### Secrets / Agent plaintext flow

KeyCenter는 중앙 plaintext secret 저장소가 아닙니다.
plaintext 입력은 hostvault 또는 명시적 agent 경로에서만 받으며, direct `/api/secrets*` CRUD 는 지원하지 않습니다.

| Method | Endpoint | 설명 |
|--------|----------|------|
| `POST /api/agents/{agent}/secrets` | `{"name":"key","value":"val"}` | hostvault/plaintext 입력 저장, canonical `token`, `scope`, `status`와 vault metadata를 함께 반환 |
| `GET /api/agents/{agent}/secrets` | | localvault metadata 기준 secret inventory 목록 |
| `GET /api/agents/{agent}/secrets/{name}` | | vault hash 기준 plaintext 조회 |
| `GET /api/resolve-agent/{token}` | | vault hash + ref token 기반 direct resolve (내부/운영용) |
| `GET /api/resolve/{ref}` | | tracked ref / vault 저장 경로 resolve |
| `POST /api/agents/rotate-all` | `{}` | eligible LocalVault에 planned rotation 을 예약하고 `key_version` 을 1씩 올린다 |

ownership 경계:
- operator boundary: plaintext 입력과 host 운영 문맥
- localvault: 노드별 ciphertext 저장과 실제 secret/config API
- keycenter: 중앙 정책, tracked ref, inventory, lifecycle, decrypt/resolve orchestration

`name`과 `key`는 모두 대문자 식별자만 허용합니다.
- 허용: `GITLAB_PAT_PUBLIC`, `VEILKEY_KEYCENTER_URL`
- 거부: `gitlab_pat`, `Bad_Key`, `BAD@KEY`

### 노드 관리

| Method | Endpoint | 설명 |
|--------|----------|------|
| `GET /api/node-info` | | 현재 HKM 노드 정보 (`tracked_refs_count` 포함) |
| `GET /api/children` | | 등록된 자식 노드 목록 |
| `POST /api/register` | | 자식 노드 등록 (init --child가 자동 호출) |
| `DELETE /api/children/{node_id}` | | 자식 노드 삭제 (`vault_node_uuid`의 path alias) |
| `DELETE /api/agents/by-node/{node_id}` | | LocalVault agent 등록 해제. installer purge가 host LocalVault 제거 전에 이 경로를 호출해 inventory를 정리한다 |
| `GET /api/configs` | | self/all-in-one LocalVault가 직접 보유한 VE(configs) 목록 조회 |
| `GET /api/configs/{key}` | | self/all-in-one LocalVault의 개별 VE(config) 조회 |
| `POST /api/configs` | | self/all-in-one LocalVault에 VE(config) 저장 |
| `PUT /api/configs/bulk` | | self/all-in-one LocalVault에 VE(config) 일괄 저장 |
| `DELETE /api/configs/{key}` | | self/all-in-one LocalVault의 VE(config) 삭제 |

### Heartbeat

자식 노드는 기본 5분마다 부모에 heartbeat를 전송합니다 (`VEILKEY_HEARTBEAT_INTERVAL`로 조정 가능).
부모는 자식 목록에서 각 노드의 상태(online/offline)를 추적합니다.
heartbeat에는 최소 다음 필드가 포함되어야 합니다.

- `vault_hash`
- `vault_name`
- `key_version`

KeyCenter는 이 값을 기준으로 inventory를 유지하고, 버전이 맞지 않으면 해당 localvault를 거부합니다.

추가로 heartbeat는 `managed_paths` metadata와 rebind 상태를 함께 관리합니다.

- `managed_paths`
  - LocalVault가 담당하는 실제 서비스 경로 목록
  - duplicate / overlap 자체는 허용
  - 실제 owner 식별은 `vault_node_uuid`(호환 alias: `node_id`) / `vault_hash` 기준
- `tracked ref sync`
  - LocalVault가 `TEMP -> LOCAL/EXTERNAL` 승격 또는 lifecycle 변경을 수행하면 KeyCenter tracked ref도 같이 갱신합니다.
  - sync 요청은 `vault_node_uuid`를 기준으로 현재 `vault_runtime_hash` owner를 찾습니다.
  - 이 경로가 살아 있어야 direct `VK:LOCAL:*` resolve 가 `404` 없이 동작합니다.
- `key_version_mismatch`
  - 첫 mismatch 시 `rebind_required`
  - 이후 재시도는 `1분 -> 3분 -> 10분 -> blocked`
  - 다만 LocalVault가 나중에 올바른 `key_version`으로 heartbeat 하면 temporary `rebind_required` / `blocked(key_version_mismatch)` 상태는 자동 해제됩니다.
- `planned rotation timeout`
  - `rotation_required` 상태가 다음 heartbeat/tick에서 해소되지 않으면 `1분 -> 3분 -> 10분` 단계로 진전
  - 이 진전은 `rotate-all`뿐 아니라 일반 inventory 조회(`/api/agents`)와 heartbeat 경계에서도 함께 진행됩니다.
  - 끝까지 해소되지 않으면 `blocked` + `block_reason=rotation_timeout`
  - 이렇게 막힌 LocalVault는 다음 `POST /api/agents/rotate-all` 대상에서 자동 제외
- `blocked`
  - resolve/save/get/config/migrate 경로를 모두 차단
  - human-approved rebind 전까지 old identity는 정상 경로를 다시 열 수 없음

### Human-Approved Rebind

| Method | Endpoint | 설명 |
|--------|----------|------|
| `POST /api/agents/{agent}/approve-rebind` | | rebind 승인, 새 `vault_hash`/증가된 `key_version` 발급 |

rebind 승인 후 LocalVault는 새 `key_version`으로 로컬 `node_info.version`을 갱신한 뒤 다시 heartbeat 해야 합니다.

### 운영

| Method | Endpoint | 설명 |
|--------|----------|------|
| `POST /api/unlock` | `{"password":"..."}` | 잠긴 서버 언락 |
| `POST /api/heartbeat` | `{"vault_node_uuid":"...","vault_hash":"...","vault_name":"...","key_version":1}` | 자식→부모 heartbeat (자동, `node_id` alias 허용) |
| `GET /api/health` | | 헬스 체크 |
| `GET /api/status` | | HKM 상태 정보 (`tracked_refs_count` 포함) |
| `POST /api/rekey` | `{"dek":"..."}` | 부모가 발급한 새 DEK로 전체 재암호화 |
| `POST /api/set-parent` | `{"parent_url":"..."}` | 부모 URL 변경 |

### Tracked Ref Audit

| Method | Endpoint | 설명 |
|--------|----------|------|
| `GET /api/tracked-refs/audit` | | tracked ref의 `blocked`/`stale` 상태와 사유를 집계 |

Tracked ref audit는 top-level class를 두 개만 유지합니다.

- `blocked`
  - 즉시 사용 차단 또는 격리가 필요한 ref
- `stale`
  - 중앙 정리 또는 ownership 복구가 필요한 ref

현재 `stale.reason` 값은 다음을 사용합니다.

- `missing_owner`
  - tracked ref row 자체에 owner vault hash가 비어 있음
- `missing_agent`
  - owner vault hash는 있으나 현재 inventory에 존재하지 않음
- `duplicate_ref_id`
  - 같은 `vault_hash + family + id` 조합에 ref가 둘 이상 남아 있음
- `agent_mismatch`
  - 같은 `family + id`가 둘 이상의 vault hash에 걸쳐 남아 있음

새 top-level class는 운영 조치가 실제로 달라질 때만 추가합니다. 그 전에는 `blocked` 또는 `stale.reason` 확장으로 처리합니다.

## LXC 배포

`scripts/deploy-lxc.sh` 는 다음 순서로 동작합니다.

1. 로컬에서 새 바이너리를 빌드합니다.
2. `vibe_lxc_ops` 로 target LXC의 systemd unit 경로를 읽습니다.
3. 바이너리를 LXC에 push 하고 서비스를 다시 시작합니다.
4. LXC 안의 배포된 바이너리 SHA256을 다시 읽어 로컬 빌드 산출물과 일치하는지 확인합니다.

이 검증이 실패하면 배포를 성공으로 간주하지 않습니다.
이 스크립트는 Proxmox host에서만 실행해야 하며, CI deploy job도 `proxmox-host` runner에서 실행되어야 합니다.

## 환경변수

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

## 테스트

```bash
go test ./...
```

- `tests/integration/removed_endpoints_test.go`
  - KeyCenter가 로컬 `/api/secrets*` 및 federation secret 경로를 제공하지 않음을 검증
- `internal/api/hkm_agent_secret_routes_test.go`
  - direct `/api/secrets*` 는 비워 두고, 지원되는 plaintext ingress 가 `/api/agents/{agent}/secrets` 경로에만 남아 있음을 검증
- `internal/api/hkm_resolve_no_local_secret_test.go`
  - `secrets` 테이블에 row가 있어도 `GET /api/resolve/VK:LOCAL:{ref}` 가 로컬 secret fallback 없이 `404` 여야 함을 검증
- `internal/api/hkm_agent_secrets_test.go`
  - localvault secret 공개 응답이 scoped canonical token(`VK:{SCOPE}:{REF}`)만 사용하고 legacy `VK:{ref}` 형식을 노출하지 않음을 검증

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
