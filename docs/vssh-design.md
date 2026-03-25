# VSSH: SSH Key Management in VeilKey

## 개요

VeilKey에 SSH 키를 저장하고 참조하는 기능. 시크릿과 동일한 암호화 + 마스킹 체계를 사용하되, SSH 키 전용 ref 형식과 워크플로우를 제공한다.

## Ref 형식

```
vssh:local:{hash}       # 본인 소유 SSH 키 (private + public)
vssh:external:{hash}    # 외부/서버 SSH 키 (주로 public만)
```

- `hash`: 8자리 hex (SHA256 of key fingerprint)
- `local`: 개인 키 — 암호화 저장, ssh-agent 연동
- `external`: 외부 키 — 서버 호스트 키, 동료 public key 등

## 저장 구조

```json
{
  "ref": "vssh:local:abc12345",
  "type": "ssh-key",
  "scope": "local",
  "label": "jeonghan@pve-main",
  "key_type": "ed25519",
  "fingerprint": "SHA256:xyzabc...",
  "private_key_enc": "<encrypted>",   // local만
  "public_key": "ssh-ed25519 AAAA...",
  "created_at": "2026-03-25T12:00:00Z",
  "metadata": {
    "hosts": ["192.168.2.60", "github.com"],
    "user": "root",
    "comment": "Proxmox backup server"
  }
}
```

## CLI 명령어

### 키 등록

```bash
# 기존 SSH 키 등록 (private + public)
veilkey ssh add ~/.ssh/id_ed25519 --label "main-key"
# → vssh:local:abc12345

# public key만 등록 (외부 키)
veilkey ssh add ~/.ssh/authorized_keys/coworker.pub --external --label "coworker"
# → vssh:external:def67890

# ssh-keygen으로 새 키 생성 + 자동 등록
veilkey ssh generate --type ed25519 --label "deploy-key"
# → Generates key pair, stores in VeilKey
# → vssh:local:ghi11111
# → Public key printed to stdout (for adding to GitHub etc.)
```

### 키 목록

```bash
veilkey ssh list
# TYPE        REF                   LABEL              KEY_TYPE   HOSTS
# local       vssh:local:abc12345   jeonghan@pve-main  ed25519    192.168.2.60, github.com
# local       vssh:local:ghi11111   deploy-key         ed25519    —
# external    vssh:external:def678  coworker           rsa        —
```

### 키 사용

```bash
# SSH 접속 시 VeilKey에서 키를 꺼내서 사용
veilkey ssh connect root@192.168.2.60
# → 자동으로 해당 host에 매핑된 키를 ssh-agent에 로드
# → ssh -o IdentityAgent=... root@192.168.2.60

# ssh-agent에 키 로드
veilkey ssh agent-add vssh:local:abc12345
# → 키를 복호화하여 ssh-agent에 추가 (TTL 설정 가능)

# 키를 파일로 임시 추출 (TTL 후 자동 삭제)
veilkey ssh export vssh:local:abc12345 --ttl 5m
# → /tmp/veilkey-ssh-abc12345 (5분 후 자동 삭제)
# → 권한 0600
```

### 키 삭제

```bash
veilkey ssh remove vssh:local:abc12345
```

### Host 매핑

```bash
# 키를 특정 호스트에 매핑
veilkey ssh map vssh:local:abc12345 --host 192.168.2.60 --user root
veilkey ssh map vssh:local:abc12345 --host github.com --user git

# 매핑 확인
veilkey ssh hosts vssh:local:abc12345
# → 192.168.2.60 (root)
# → github.com (git)
```

## veil shell 연동

veil shell 안에서 `ssh` 명령이 실행되면:
1. 대상 호스트에 매핑된 키가 있는지 확인
2. 있으면 자동으로 ssh-agent에 로드
3. SSH 접속 시 해당 키 사용

```
(VEIL) $ ssh root@192.168.2.60
[veilkey] using vssh:local:abc12345 for root@192.168.2.60
root@backup-server:~#
```

## PTY 마스킹 연동

- SSH private key가 출력에 나오면 `vssh:local:{hash}` 형태로 마스킹
- `-----BEGIN OPENSSH PRIVATE KEY-----` 패턴 감지
- public key는 마스킹하지 않음 (공개 정보)

## 보안 모델

| 항목 | 정책 |
|------|------|
| Private key 저장 | KEK로 암호화, DB에 저장 |
| Private key 접근 | TTY 필수, admin 인증 |
| Private key 출력 | PTY 마스킹 (절대 평문 노출 안 됨) |
| ssh-agent 로드 | TTY 필수, 세션 종료 시 자동 제거 |
| 임시 파일 추출 | 0600 권한, TTL 후 자동 삭제 |
| Public key | 자유 접근 (공개 정보) |
| External key | public만 저장, private 없음 |

## API 엔드포인트 (VaultCenter)

```
POST   /api/ssh/keys              # 키 등록
GET    /api/ssh/keys              # 키 목록
GET    /api/ssh/keys/{ref}        # 키 상세 (public만)
POST   /api/ssh/keys/{ref}/private # private key 복호화 (TTY + admin)
DELETE /api/ssh/keys/{ref}        # 키 삭제
POST   /api/ssh/keys/{ref}/map    # 호스트 매핑
GET    /api/ssh/hosts/{host}      # 호스트별 키 조회
```

## DB 스키마

```sql
CREATE TABLE ssh_keys (
    ref TEXT PRIMARY KEY,           -- vssh:local:abc12345
    scope TEXT NOT NULL,            -- local | external
    label TEXT,
    key_type TEXT,                  -- ed25519 | rsa | ecdsa
    fingerprint TEXT,
    private_key_enc BLOB,           -- AES-256-GCM encrypted (local만)
    private_key_nonce BLOB,
    public_key TEXT,
    metadata_json TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ssh_host_mappings (
    ref TEXT NOT NULL,
    host TEXT NOT NULL,
    username TEXT DEFAULT '',
    PRIMARY KEY (ref, host),
    FOREIGN KEY (ref) REFERENCES ssh_keys(ref)
);
```

## 구현 순서

### Phase 1: 기본 CRUD
- [ ] DB 스키마 + 마이그레이션
- [ ] API 엔드포인트 (등록, 목록, 상세, 삭제)
- [ ] CLI: `veilkey ssh add/list/remove`
- [ ] 암호화 저장/복호화

### Phase 2: SSH 연동
- [ ] `veilkey ssh agent-add` (ssh-agent 로드)
- [ ] `veilkey ssh connect` (자동 키 선택 + 접속)
- [ ] Host 매핑 (`veilkey ssh map/hosts`)
- [ ] 임시 파일 추출 (`veilkey ssh export --ttl`)

### Phase 3: veil shell 통합
- [ ] SSH 명령 감지 + 자동 키 로드
- [ ] Private key PTY 마스킹
- [ ] 세션 종료 시 ssh-agent cleanup

### Phase 4: TUI
- [ ] KeyCenter 옆에 SSH Keys 탭
- [ ] 키 생성/삭제/매핑 UI
