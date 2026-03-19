<div align="center">
  <h1>VeilKey Self-Hosted</h1>
  <p><strong>AI가 절대 볼 수 없는 시크릿 관리. PTY 레벨 양방향 마스킹 + 블록체인 감사.</strong></p>
  <p>
    <a href="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml"><img src="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
    <a href="https://github.com/veilkey/veilkey-selfhosted/releases"><img src="https://img.shields.io/github/v/release/veilkey/veilkey-selfhosted?display_name=tag" alt="GitHub release"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-2563eb.svg" alt="AGPL-3.0"></a>
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

## Quick Start

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
docker compose up -d
```

1. **VaultCenter 셋업**: `https://localhost:11181` → 마스터 + 관리자 비밀번호 설정
2. **LocalVault 연결**: keycenter에서 등록 토큰 발급 → LV init
3. **시크릿 저장**: keycenter에서 임시키 생성 → 볼트에 격상
4. **veil 셸**: `docker compose exec -it veil veilkey-cli wrap-pty bash`

### LocalVault 등록 (1줄)

```bash
docker compose exec localvault sh -c \
  "echo 'password' | veilkey-localvault init --root --token vk_reg_xxx --center https://vaultcenter:10181"
docker compose restart localvault
```

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

GNU Affero General Public License v3.0 (AGPL-3.0). See [`LICENSE`](./LICENSE).

## Security Disclaimer

VeilKey is a security-sensitive tool that handles secrets and cryptographic material.
This software is provided WITHOUT WARRANTY. Before using VeilKey in production,
conduct your own security audit and review.

If you discover a security issue, please report it privately via GitHub Security Advisories.
