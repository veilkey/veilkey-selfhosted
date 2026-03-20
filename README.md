<div align="center">
  <h1>VeilKey Self-Hosted</h1>
  <p><strong>AI가 절대 볼 수 없는 시크릿 관리. PTY 레벨 양방향 마스킹 + 블록체인 감사.</strong></p>
  <p>
    <a href="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml"><img src="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
    <a href="https://github.com/veilkey/veilkey-selfhosted/releases"><img src="https://img.shields.io/github/v/release/veilkey/veilkey-selfhosted?display_name=tag" alt="GitHub release"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-2563eb.svg" alt="MIT"></a>
  </p>
</div>

## What is VeilKey?

VeilKey is a self-hosted secret manager where **AI coding tools never see your passwords**.

```bash
# veil 셸 안에서
$ cat .env
DB_PASSWORD=VK:LOCAL:ea2bfd16    ← AI가 보는 값

# 실제 앱은 진짜 비밀번호를 받음
$ npm start                       ← DB_PASSWORD=actual-secret
```

PTY 출력에서 비밀번호가 나오면 자동으로 VK ref로 치환. Claude Code, Cursor, Copilot 등 어떤 AI 도구를 써도 평문을 볼 수 없습니다.

## Architecture

```
VaultCenter (열쇠 관리자)          LocalVault (금고)
┌──────────────────────┐          ┌──────────────────┐
│ agentDEK (암호화 키)  │          │ ciphertext (암호문) │
│ KEK → DEK 보호       │          │ 저장만, 복호화 불가  │
│ 블록체인 감사 로그     │          │                    │
└──────────────────────┘          └──────────────────┘
         │                                  │
         └──── 둘 다 있어야 복호화 가능 ────────┘

veil CLI (PTY 마스킹)
┌──────────────────────────────────────────┐
│ 환경변수: VK:LOCAL:xxx → 실제 값 (프로세스) │
│ 출력: 실제 값 → VK:LOCAL:xxx (화면/AI)     │
└──────────────────────────────────────────┘
```

**둘 다 탈취해야 시크릿 접근 가능:**
- VaultCenter만 탈취 → agentDEK 있지만 ciphertext 없음
- LocalVault만 탈취 → ciphertext 있지만 agentDEK 없음

## Installation

### macOS

```bash
curl -sL https://gist.githubusercontent.com/dalsoop/990c3706a62834599b0d9f5316a314ad/raw/install-veilkey.sh | bash
```

이 한 줄로:
- repo 클론 (`~/.veilkey`)
- Docker 서비스 시작 (VaultCenter + LocalVault + veil)
- Rust CLI 빌드 + ad-hoc 코드 서명
- 셸 환경변수 설정 (`~/.zshrc`)

설치 후:
1. `https://localhost:11181` → 마스터 + 관리자 비밀번호 설정
2. 터미널 재시작
3. `veil` 입력 → 보호 셸 진입

삭제:
```bash
curl -sL https://gist.githubusercontent.com/dalsoop/990c3706a62834599b0d9f5316a314ad/raw/uninstall-veilkey.sh | bash
```

### Linux

```bash
# 1. 의존성
sudo apt install -y git docker.io docker-compose-plugin
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env

# 2. 클론 + 서비스 시작
git clone https://github.com/veilkey/veilkey-selfhosted.git ~/.veilkey
cd ~/.veilkey
docker compose up -d

# 3. CLI 빌드 + 설치
cargo build --release
sudo cp target/release/{veil,veilkey,veilkey-cli,veilkey-session-config} /usr/local/bin/

# 4. 셸 설정
cat >> ~/.bashrc << 'EOF'
# VeilKey
export VEILKEY_LOCALVAULT_URL="https://localhost:11181"
export VEILKEY_TLS_INSECURE=1
export VEILKEY_CONFIG="$HOME/.veilkey.yml"
export VEILKEY_BIN=/usr/local/bin/veilkey
export VEILKEY_CLI_BIN=/usr/local/bin/veilkey-cli
export VEILKEY_VK_BIN=/usr/local/bin/veilkey
export VEILKEY_SESSION_CONFIG_BIN=/usr/local/bin/veilkey-session-config
EOF
cp services/veil-cli/examples/.veilkey.yml ~/.veilkey.yml
source ~/.bashrc

# 5. 셋업 + 진입
# https://localhost:11181 → 비밀번호 설정
veil
```

### LocalVault 별도 설치

이미 VaultCenter가 돌고 있는 환경에서 LocalVault만 추가:

```bash
curl -sL "https://gist.githubusercontent.com/dalsoop/11e00346263678340189cdfdc79644b5/raw/install-localvault.sh?$(date +%s)" | \
  VEILKEY_CENTER_URL=https://your-vaultcenter:11181 bash
```

현재 디렉토리에 `.localvault/`를 만들고 Docker로 실행합니다. heartbeat로 VaultCenter에 자동 등록됩니다.

```bash
# 커스텀 이름/포트
VEILKEY_CENTER_URL=https://10.0.0.1:11181 VEILKEY_HOST_PORT=11182 VEILKEY_NAME=dev-vault \
  curl -sL "...?$(date +%s)" | bash

# 업데이트 (같은 명령 재실행)
curl -sL "...?$(date +%s)" | VEILKEY_CENTER_URL=... bash
```

`veil` CLI에서도 가능:
```bash
veil localvault init      # 설치 + 시작
veil localvault stop      # 중지
veil localvault log       # 로그
veil localvault status    # health check
```

### Setup (공통)

설치 후 VaultCenter 셋업:

1. **`https://localhost:11181`** 접속 → 마스터 + 관리자 비밀번호 설정
2. **LocalVault 등록** — keycenter에서 등록 토큰 발급:
```bash
docker compose exec localvault sh -c \
  "echo 'password' | veilkey-localvault init --root \
    --token vk_reg_xxx --center https://vaultcenter:10181"
docker compose restart localvault
```
3. **시크릿 저장** — keycenter에서 임시키 생성 → 볼트에 격상
4. **`veil`** 입력 → 보호 셸. 모든 등록된 시크릿이 자동 마스킹. AI가 출력을 봐도 `VK:LOCAL:xxx`만 보임.

## Key Features

### PTY 양방향 마스킹
```bash
# veil 셸 안에서 — AI가 이 출력을 봐도 안전
$ echo $DB_PASSWORD
VK:LOCAL:ea2bfd16              ← 마스킹됨 (실제 값: actual-password)

$ cat config.env
DB_PASSWORD=VK:LOCAL:ea2bfd16  ← 파일 읽기도 마스킹
API_KEY=VK:LOCAL:ea2bfd16      ← 등록된 값만 치환

# 실제 프로세스는 진짜 값을 받음
$ node app.js                  ← process.env.DB_PASSWORD = "actual-password"
```

### CometBFT 블록체인 감사
- 모든 키 생성/회전/삭제가 불변 체인에 기록
- LocalVault가 full node로 블록 검증 → VaultCenter 단독 조작 불가
- DB 해킹해도 블록 해시 체인이 깨져서 위변조 탐지

### 분리 보관
- VaultCenter: agentDEK (암호화 키) 보관
- LocalVault: ciphertext (암호문) 저장만, 복호화 불가
- 한쪽만 탈취해도 시크릿 접근 불가

### 관리자 웹 UI
- keycenter: 임시키 CRUD, 볼트 격상, 등록 토큰 발급
- 볼트 관리: 시크릿 조회, 함수 바인딩, 설정
- 감사 로그: 전체 키 운영 이력

## Repository Structure

```
services/
  vaultcenter/     ← 중앙 관리 서버 (Go)
  localvault/      ← 로컬 금고 (Go)
  veil-cli/        ← veil, veilkey, veilkey-cli, veilkey-session-config (Rust)
docker-compose.yml ← 전체 스택 (VC + LV + veil)
```

## CLI Tools

| 명령 | 용도 |
|------|------|
| `veilkey-cli resolve VK:LOCAL:xxx` | ref → 실제 값 |
| `veilkey-cli exec echo VK:LOCAL:xxx` | 인자 치환 후 실행 |
| `veilkey-cli wrap-pty bash` | PTY 마스킹 셸 진입 |
| `veilkey-cli scan file.env` | 시크릿 감지 (222 패턴) |
| `veilkey-cli filter file.env` | 시크릿 → VK ref 치환 |
| `veilkey-cli status` | 연결 상태 확인 |

## Comparison

| 기능 | 1Password CLI | Doppler | HashiCorp Vault | **VeilKey** |
|------|---------------|---------|-----------------|-------------|
| 시크릿 저장 | ✅ | ✅ | ✅ | ✅ |
| 참조 시스템 | ✅ `op://` | ❌ | ❌ | ✅ `VK:LOCAL:` |
| 환경변수 주입 | ✅ | ✅ | ✅ | ✅ |
| **PTY 출력 마스킹** | ❌ | ❌ | ❌ | **✅** |
| **양방향 치환** | ❌ | ❌ | ❌ | **✅** |
| **파일 읽기 마스킹** | ❌ | ❌ | ❌ | **✅** |
| 블록체인 감사 | ❌ | ❌ | ❌ | **✅** |
| 분리 보관 (VC/LV) | ❌ | ❌ | ❌ | **✅** |
| 셀프호스팅 | ❌ | ❌ | ✅ | **✅** |

## Environment Variables

설정 가능한 값들은 `.env.example` 참조:
- `services/vaultcenter/.env.example`
- `services/localvault/.env.example`

주요 설정:
| 변수 | 기본값 | 설명 |
|------|--------|------|
| `VEILKEY_TEMP_REF_TTL` | `1h` | 임시키 만료 시간 |
| `VEILKEY_ADMIN_SESSION_TTL` | `2h` | 관리자 세션 유지 |
| `VEILKEY_CHAIN_HOME` | `/data/chain` | CometBFT 데이터 경로 |
| `VEILKEY_TLS_INSECURE` | `0` | 자체서명 인증서 허용 |

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## License

MIT License. See [`LICENSE`](./LICENSE).

## How It Works (서버 재시작 시)

VeilKey 서버가 재시작되면 **마스터 비밀번호를 다시 입력해야** 합니다.

```
서버 시작 → LOCKED 상태 (DEK 메모리에 없음)
  → 웹 UI에서 마스터 비밀번호 입력
  → KEK 유도 → DEK 복호화 → 메모리에 로드
  → UNLOCKED (정상 동작)
```

**비밀번호는 어디에도 저장되지 않습니다.** KEK는 비밀번호 + salt로 매번 유도되고, DEK는 KEK로 암호화된 상태로만 DB에 존재. 서버가 꺼지면 KEK와 DEK 모두 메모리에서 사라집니다.

`VEILKEY_PASSWORD_FILE` 환경변수로 자동 unlock을 설정할 수 있지만, 이 파일의 보안은 운영자 책임입니다.

## Security

**AI를 root 권한으로 절대 실행하지 마세요.**

VeilKey는 AI가 시크릿에 접근하지 못하게 설계되었지만, root 권한이 있으면:
- 프로세스 메모리 덤프 → DEK 추출 가능
- `/data/` 디렉토리 직접 접근 → DB 파일 조작 가능
- PTY 마스킹 우회 → `/proc/{pid}/fd/` 등으로 raw 출력 접근 가능

**권장:**
- AI 코딩 도구는 일반 사용자 권한으로 실행
- `veil` 셸 안에서만 작업 → PTY 마스킹 보장
- `sudo`가 필요한 작업은 veil 밖에서 직접 수행

## Security Disclaimer

VeilKey is a security-sensitive tool that handles secrets and cryptographic material.
This software is provided WITHOUT WARRANTY. Before using VeilKey in production,
conduct your own security audit and review.

If you discover a security issue, please report it privately via GitHub Security Advisories.
