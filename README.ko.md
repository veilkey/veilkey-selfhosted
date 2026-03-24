<div align="center">
  <img src=".github/banner.png" alt="VeilKey" width="720">
  <h1>VeilKey Self-Hosted</h1>
  <p><strong>터미널에 도달하기 전에 시크릿을 숨깁니다.</strong></p>
  <p>
    <a href="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml"><img src="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
    <a href="https://github.com/veilkey/veilkey-selfhosted/releases"><img src="https://img.shields.io/github/v/release/veilkey/veilkey-selfhosted?display_name=tag" alt="GitHub release"></a>
    <a href="https://www.npmjs.com/package/veilkey-cli"><img src="https://img.shields.io/npm/v/veilkey-cli?color=cb3837" alt="npm"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-2563eb.svg" alt="MIT"></a>
  </p>
</div>

[English](./README.md) | 한국어

## 문제

AI 코딩 도구(Claude Code, Cursor, Copilot)는 터미널 출력, 환경 변수, 파일을 읽습니다.

비밀번호가 어디든 한 번이라도 나타나면 — **AI가 봅니다.**

## 해결

VeilKey가 터미널을 감쌉니다. 시크릿은 화면에 나타나지 않습니다.

```bash
# VeilKey 없이 — AI가 비밀번호를 봅니다
$ echo $DB_PASSWORD
actual-password-here          ← AI가 읽음

# VeilKey 사용 — AI는 참조만 봅니다
$ veil                        ← 보호 셸 진입
$ echo $DB_PASSWORD
VK:LOCAL:ea2bfd16             ← AI가 읽는 것 (암호화된 참조)

# 하지만 앱은 실제 비밀번호를 받습니다
$ npm start                   ← DB_PASSWORD = actual-password-here
```

## 동작 원리

```
명령어 입력
    ↓
VeilKey가 출력을 가로챔
    ↓
비밀번호가 VK:LOCAL:xxx 참조로 치환
    ↓
AI는 참조만 봄 — 실제 값은 절대 못 봄
    ↓
앱은 실제 비밀번호를 받음
```

VeilKey는 **222가지 시크릿 패턴**을 자동 감지합니다 — AWS 키, GitHub 토큰, API 키, 비밀번호 등.

## 빠른 시작

### macOS

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash install/macos/bootstrap/install-all.sh
```

### Proxmox LXC (Debian)

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
CT_IP=<IP>/<MASK> CT_GW=<GATEWAY> bash install/proxmox-lxc-debian/install-veilkey.sh
```

설치 후: [초기 설정 가이드](./docs/setup/README.md) 참조

## 명령어

```bash
veil                          # 보호 셸 진입
veil status                   # 연결 상태 확인
veil resolve VK:LOCAL:xxx     # 참조 복호화
veil exec echo VK:LOCAL:xxx   # 실제 값으로 명령 실행
veil scan file.env            # 파일에서 시크릿 찾기 (222 패턴)
```

## 시크릿 저장 및 사용

### 1. 시크릿 저장

```bash
# VaultCenter 로그인
curl -sk -X POST https://<VC>:11181/api/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"password":"<ADMIN_PASSWORD>"}' -c /tmp/vk-cookies.txt

# 임시 ref 생성
curl -sk -X POST https://<VC>:11181/api/keycenter/temp-refs \
  -H 'Content-Type: application/json' -b /tmp/vk-cookies.txt \
  -d '{"name":"MY_API_KEY","value":"sk-actual-key-value"}'
# → {"ref":"VK:TEMP:xxxxxxxx"}

# vault로 승격
curl -sk -X POST https://<VC>:11181/api/keycenter/promote \
  -H 'Content-Type: application/json' -b /tmp/vk-cookies.txt \
  -d '{"ref":"VK:TEMP:xxxxxxxx","name":"MY_API_KEY","vault_hash":"a0a761c6"}'
# → {"token":"VK:LOCAL:yyyyyyyy"}
```

### 2. `.env` 파일에서 사용

`.env`에는 VK 참조만 — 평문 금지:

```env
ANTHROPIC_API_KEY=VK:LOCAL:ce2aac9a
OPENAI_API_KEY=VK:LOCAL:7accddf2
DB_PASSWORD=VK:LOCAL:bdd9d472
```

### 3. 런타임에 resolve

```bash
# 명령 인자에서 resolve
veil exec echo VK:LOCAL:ce2aac9a

# .env 파일을 resolve해서 앱 실행
./veil-run.sh node app.js
```

### 4. 인프라에서도 VK ref 사용

```bash
# 비밀번호를 모른 채 LXC 생성
veil exec pct create 105 local:vztmpl/debian-13.tar.zst \
  --hostname myapp --password VK:LOCAL:bdd9d472 ...

# VK 관리 키로 SSH
veil exec ssh -i VK:LOCAL:ssh-private-key user@host
```

## 아키텍처

VeilKey는 시크릿을 두 서버에 분리합니다. **양쪽 모두 탈취해야** 시크릿에 접근 가능합니다.

```
VaultCenter                    LocalVault
┌────────────────────┐        ┌────────────────────┐
│ 암호화 키           │        │ 암호화된 데이터      │
│ (데이터 없이는      │        │ (키 없이는          │
│  읽을 수 없음)      │        │  복호화 불가)        │
└────────────────────┘        └────────────────────┘
         │                              │
         └──── 양쪽 모두 필요 ──────────┘
```

| 컴포넌트 | 저장 내용 | 단독으로 할 수 있는 것 |
|----------|----------|---------------------|
| **VaultCenter** | 암호화 키 | 없음 (데이터 없음) |
| **LocalVault** | 암호화된 시크릿 | 없음 (키 없음) |
| **veil CLI** | 없음 | 터미널 출력 마스킹 |

모든 키 작업은 **블록체인 감사 추적**에 기록됩니다.

## 보안

### 데이터베이스 암호화

모든 데이터베이스는 SQLCipher로 암호화됩니다. 암호화 키는 마스터 비밀번호(KEK)에서 파생 — unlock 후에만 DB 접근 가능.

`sqlite3` 직접 접근은 차단됩니다. admin 비밀번호는 소유자 비밀번호(owner password)로만 변경 가능합니다.

### Vault 격리

각 LocalVault는 `agent_secret` (Bearer 토큰)으로 VaultCenter에 인증합니다. 자기 vault의 시크릿만 접근 가능하며, cross-vault 조회는 거부됩니다.

### 메모리 전용 KEK

마스터 비밀번호(KEK)는 메모리에만 존재합니다. 파일 저장 없음, 환경 변수 없음. 재시작 시 `POST /api/unlock`으로 입력해야 합니다.

## 서버 재시작

서버가 재시작되면 시크릿이 잠깁니다. 마스터 비밀번호로 unlock해야 합니다.

```bash
curl -sk -X POST https://<server>:10180/api/unlock \
  -H 'Content-Type: application/json' \
  -d '{"password":"<master-password>"}'
```

## 관리자 패널

**웹 UI** — 브라우저에서 `https://<서버>:10181/` 접속

**TUI** — 터미널에서 `vaultcenter keycenter` 실행

## 레포지토리 구조

```
services/
  vaultcenter/     # 키 관리 서버 (Go)
  localvault/      # 암호화 저장소 (Go)
  veil-cli/        # 터미널 래퍼 (Rust)
packages/
  veil-cli/        # npm 패키지
docker-compose.yml # 전체 스택
```

## 기여

[`CONTRIBUTING.md`](./CONTRIBUTING.md) 참조

## 라이선스

MIT License. [`LICENSE`](./LICENSE) 참조

---

<sub>본 README의 이미지는 AI로 생성되었습니다.</sub>
