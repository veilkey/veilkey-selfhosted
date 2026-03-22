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

## Key responsibilities

- Master password → KEK derivation → DEK management
- Admin web UI (Vue.js SPA)
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
