# VaultCenter

Central management server for VeilKey. Handles encryption keys (agentDEK), admin UI, CometBFT blockchain audit, and secret promotion.

## Build

```bash
CGO_ENABLED=1 go build -o veilkey-vaultcenter ./cmd
```

## Docker

```bash
docker compose up --build -d vaultcenter
```

## Admin UI

VaultCenter는 두 가지 관리 인터페이스를 제공한다:

### Web UI

Vue.js SPA. 브라우저로 접속.

```
https://<vaultcenter-host>:10181/
```

- 경로: `services/vaultcenter/frontend/admin/`
- 빌드: `cd frontend/admin && npm install && npx vite build`
- 빌드 결과는 `internal/api/admin/ui_dist/`에 embed되어 Go 바이너리에 포함

### TUI (Terminal UI)

bubbletea 기반 터미널 관리 패널. 웹 브라우저 없이 tmux에서 직접 사용.

```bash
VEILKEY_KEYCENTER_URL=https://<host>:<port> vaultcenter keycenter
```

필수 환경변수:

| 변수 | 설명 |
|------|------|
| `VEILKEY_KEYCENTER_URL` | VaultCenter API 주소 (없으면 `VEILKEY_ADDR` 사용) |
| `VEILKEY_TLS_INSECURE` | `1`로 설정 시 self-signed 인증서 허용 |

로그인 플로우:

1. 서버 locked → 마스터키(KEK) 입력 → unlock
2. 인증: TOTP 코드 또는 관리자 패스워드 (`tab`으로 전환)
3. admin 패널 진입

5개 탭 (숫자 1~5 또는 마우스 클릭):

| 탭 | 기능 |
|----|------|
| 1 Keycenter | temp ref 목록/상세/생성/reveal |
| 2 Vaults | vault 목록, agent 목록, secret catalog, secret 상세 + 복호화 |
| 3 Functions | global function 목록/상세/실행, 바인딩 |
| 4 Audit | admin 감사 로그 |
| 5 Settings | 서버 상태, 보안 설정, registration token 관리, configs |

비밀번호 표시:

- `VK:` ref → 마스킹(`••••••••`), `r`로 reveal (authorize + decrypt)
- `VE:` ref → 값 그대로 표시

키 바인딩:

| 키 | 동작 |
|----|------|
| `j/k` | 위/아래 이동 |
| `enter` | 선택/상세 |
| `esc` | 뒤로 |
| `tab` | 서브탭 전환 |
| `r` | 새로고침 또는 reveal |
| `h` | reveal 숨기기 |
| `n` | 새로 만들기 |
| `1-5` | 탭 전환 |
| `q` | 종료 |

## Key responsibilities

- Master password → KEK derivation → DEK management
- Admin web UI (Vue.js SPA) + TUI (bubbletea)
- Keycenter: temp ref CRUD, promote to vault
- Registration token management for LocalVault onboarding
- CometBFT ABCI chain layer for audit trail
- Bulk-apply: template rendering + workflow proxy to LocalVault

## Agent lifecycle

Agent 상태 전이는 모두 체인 TX로 기록된다. DEK(암호화 키)는 보안상 체인에 포함하지 않으며 DB에만 저장.

```
[heartbeat] → UpsertAgent TX → agent 생성/갱신 (DEK 발급)
[archive]   → DB 직접         → archived_at 설정 (UI에서 숨김)
[unarchive] → DB 직접         → archived_at 해제
[delete]    → DeleteAgent TX  → deleted_at 설정 (soft-delete, DEK 보존)
```

### Soft-delete 설계 원칙

`DELETE /api/agents/by-node/{node_id}`는 **soft-delete**:
- `deleted_at` 타임스탬프만 설정, 실제 레코드/DEK는 보존
- 체인에 DeleteAgent TX가 기록되어 감사 이력 유지
- 삭제된 agent의 LocalVault가 재연결(heartbeat) 시 → `deleted_at` 해제 → 기존 DEK로 복호화 가능

**이유:** VaultCenter는 체인 블록(v1~vn)으로 전체 상태를 재구성할 수 있어야 한다. DEK가 DB에서 삭제되면 해당 agent의 시크릿을 영구적으로 복호화할 수 없다. hard-delete는 지원하지 않는다.

### Archive vs Delete

| | Archive | Delete |
|---|---------|--------|
| 체인 TX | 없음 | DeleteAgent TX |
| DEK 보존 | O | O |
| 감사 이력 | 없음 | 체인에 기록 |
| 자동 복원 | unarchive API | heartbeat 시 자동 |
| 용도 | UI 정리 | agent 등록 해제 |

## API

See [docs/setup/](../../docs/setup/README.md) for usage guides.

## Environment

See [docs/setup/env-vars.md](../../docs/setup/env-vars.md).
