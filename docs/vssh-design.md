# VK:SSH — SSH Key Management in VeilKey

## 개요

VeilKey의 기존 `VK:` ref 체계 안에서 SSH 키를 관리한다. 별도 체계가 아니라 scope 확장이므로 마스킹, resolve, detector가 그대로 동작한다.

## Ref 형식

```
VK:LOCAL:abc12345    ← 시크릿 (기존)
VK:TEMP:abc12345     ← 임시 시크릿 (기존)
VK:SSH:abc12345      ← SSH 키 (신규)
```

- `hash`: 8자리 hex (key fingerprint 기반)
- 본인/외부 구분: 메타데이터 `ownership` 필드 (`own` / `external`)
- 기존 regex에 `SSH` scope 추가만 하면 전체 파이프라인 호환

### 기존 체계 호환

| 기능 | SSH 키 동작 |
|------|-------------|
| PTY 마스킹 | private key → `VK:SSH:{hash}` 치환 |
| resolve | `veilkey-cli resolve VK:SSH:xxx` → private key (TTY 필수) |
| mask_map | private key → VK:SSH 매핑 자동 등록 |
| padded_colorize_ref | `VK:SSH:` → GREEN 색상 |
| detector | SSH private key 패턴 자동 감지 |

## CLI

```bash
veilkey-cli ssh add ~/.ssh/id_ed25519 --label "main-key"      # → VK:SSH:abc12345
veilkey-cli ssh add coworker.pub --external --label "coworker" # → VK:SSH:def67890 (public만)
veilkey-cli ssh generate --type ed25519 --label "deploy-key"   # → VK:SSH:ghi11111
veilkey-cli ssh list                                            # 전체 목록
veilkey-cli ssh pubkey VK:SSH:abc12345                          # public key 출력
veilkey-cli ssh connect root@<HOST_IP>                           # 자동 키 선택 + 접속
veilkey-cli ssh agent-add VK:SSH:abc12345                       # ssh-agent 로드
veilkey-cli ssh map VK:SSH:abc12345 --host github.com --user git
veilkey-cli ssh remove VK:SSH:abc12345
```

## DB 스키마

```sql
CREATE TABLE ssh_keys (
    ref TEXT PRIMARY KEY,
    ownership TEXT NOT NULL DEFAULT 'own',
    label TEXT,
    key_type TEXT,
    fingerprint TEXT UNIQUE,
    private_key_enc BLOB,
    private_key_nonce BLOB,
    public_key TEXT NOT NULL,
    hosts_json TEXT DEFAULT '[]',
    metadata_json TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

## API

```
POST   /api/ssh/keys                    # 등록
GET    /api/ssh/keys                    # 목록
GET    /api/ssh/keys/{ref}              # 상세 (public만)
POST   /api/ssh/keys/{ref}/decrypt      # private key 복호화 (TTY+admin)
DELETE /api/ssh/keys/{ref}              # 삭제
PUT    /api/ssh/keys/{ref}/hosts        # 호스트 매핑
GET    /api/ssh/hosts/{host}            # 호스트별 키
```

## 코드 변경

```rust
// VEILKEY_RE_STR: SSH scope 추가
r"VK:(?:(?:TEMP|LOCAL|EXTERNAL|SSH):[0-9A-Fa-f]{4,64}|[0-9a-f]{8})"
```

## 구현 순서

- Phase 1: DB + API + CLI CRUD
- Phase 2: 마스킹 + resolve
- Phase 3: SSH 연동 (agent, connect, host mapping)
- Phase 4: TUI + generate
