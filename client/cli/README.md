# VeilKey CLI

`cli` is the operator-facing self-hosted VeilKey component.

It provides the command-line surface, secure terminal wrapping, and the `veilroot` host boundary.

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

- operator CLI commands
- secure wrapping and masking
- session configuration rendering
- `veilroot` host-boundary scripts
- host-side session helpers

## Core Surfaces

- `vk`
  - CLI entrypoint
- `vk_wrap`
  - secure PTY wrapper
- `veilroot`
  - host boundary shell and session tooling

## Related Components

- `localvault`
  - local agent and secret/config runtime
- `keycenter`
  - central control plane
- `proxy`
  - outbound enforcement layer used by wrapped workloads
4. Enter 입력 시: 현재 라인의 watchlist 값을 화면에서 마스킹 치환
5. VK:hash 자동 해석: DEBUG trap으로 명령에 포함된 `VK:xxxx`를 원본값으로 복원 후 실행

```
🔒VK $ cat .env
DATABASE_PASSWORD=VK:a1b2c3d4    ← 실제 비밀번호 대신 VK 토큰 표시
🔒VK $ curl -H "Auth: VK:a1b2c3d4"  ← 자동으로 원본값으로 복원되어 실행
```

### vk — 값 암호화 (watchlist 등록)

`vk` 명령으로 평문 값을 VeilKey 토큰으로 암호화합니다. 암호화된 값은 watchlist에 등록되어 `vk_wrap` 터미널에서 자동 마스킹됩니다.

```bash
# 숨김 입력 (타이핑 보이지 않음)
vk
Enter secret value: ********
Confirm secret value: ********
VK:a1b2c3d4

# stdin으로 전달
echo "my-secret" | vk
```

### exec — VK 토큰 자동 복호화 실행

`veilkey-cli exec`는 명령 인자에 포함된 VK 토큰을 원본값으로 복원한 후 실행합니다. 스크립트에서 시크릿을 평문으로 노출하지 않고 안전하게 사용할 수 있습니다.

```bash
# VK 토큰이 포함된 명령 → 자동 복호화 후 실행
veilkey-cli exec curl -H "Authorization: Bearer VK:a1b2c3d4" https://api.example.com

# 환경변수에 VK 토큰 사용
veilkey-cli exec env DATABASE_URL="postgres://user:VK:b2c3d4e5@db:5432/app" ./migrate
```

## 설치

```bash
# 스크립트 설치 (바이너리 + vk, vk_wrap 스크립트)
bash install/install.sh

# 또는 직접 빌드
make build
cp bin/veilkey-cli /usr/local/bin/
cp scripts/vk scripts/vk_wrap /usr/local/bin/
```

### Veilroot Host Boundary

`veilroot` host-boundary assets now live in this repository.

Canonical surface:

- `deploy/host/install-veilroot-boundary.sh`
- `deploy/host/install-veilroot-codex.sh`
- `deploy/host/veilroot-shell`
- `deploy/host/veilkey-veilroot-session`
- `deploy/host/veilkey-veilroot-observe`
- `deploy/host/veilkey-veilroot-egress-guard`
- `deploy/host/verify-veilroot-session.sh`
- `cmd/veilkey-session-config`

Typical flow:

```bash
go build -o /usr/local/bin/veilkey-session-config ./cmd/veilkey-session-config
./deploy/host/install-veilroot-boundary.sh
./deploy/host/install-veilroot-codex.sh
/usr/local/bin/veilroot-shell status
```

## 환경변수

| 변수 | 필수 | 설명 |
|------|------|------|
| `VEILKEY_LOCALVAULT_URL` | O (권장) | localvault endpoint URL |
| `VEILKEY_API` | - | 레거시 endpoint 변수명 (fallback) |
| `VEILKEY_HUB_URL` | - | hub URL fallback |
| `VEILKEY_STATE_DIR` | - | 상태 디렉토리 (기본: `$TMPDIR/veilkey-cli`) |
| `VEILKEY_FUNCTION_DIR` | - | function catalog 디렉토리 |

## 명령어

| 명령어 | 설명 |
|--------|------|
| `scan [file\|-]` | 파일/stdin에서 시크릿 탐지 (감지만, API 불필요) |
| `filter [file\|-]` | 시크릿을 VK 토큰으로 치환하여 stdout 출력 |
| `wrap <command...>` | 명령 실행 + stdout 자동 치환 |
| `wrap-pty [command]` | PTY 대화형 셸 + 입출력 자동 치환 |
| `exec <command...>` | VK 토큰을 원본값으로 복원 후 명령 실행 |
| `resolve <VK:token>` | VK 토큰을 원본값으로 복원 |
| `function <subcommand...>` | repo-tracked TOML function wrapper 관리 |
| `list` | 탐지된 VeilKey 목록 |
| `clear` | 세션 로그 초기화 |
| `status` | 상태 확인 |
| `version` | 버전 출력 |

## 옵션

| 옵션 | 설명 |
|------|------|
| `--format <text\|json\|sarif>` | 출력 형식 (기본: text) |
| `--config <path>` | 프로젝트 설정 파일 (기본: .veilkey.yml) |
| `--exit-code` | 시크릿 발견 시 exit 1 (CI용) |
| `--patterns <path>` | 커스텀 패턴 파일 |

## 사용 예시

```bash
# 파일 스캔 (API 없이 로컬에서 동작)
veilkey-cli scan .env
veilkey-cli scan --format json --exit-code .env

# 명령 출력 필터링
export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180
kubectl get secret -o yaml | veilkey-cli filter

# CI pre-commit hook (SARIF 출력)
veilkey-cli scan --exit-code --format sarif src/

# 보안 터미널 진입
vk_wrap

# 값 암호화
echo "my-api-key" | vk

# function 목록
VEILKEY_FUNCTION_DIR=/opt/veilkey/veilkey-cli/functions veilkey-cli function list
```

## Function Catalog

`function` 서브커맨드는 repo-tracked `functions/*.toml` 카탈로그를 읽습니다.

- 1파일 1함수
- placeholder는 `{%{NAME}%}`
- 변수값은 반드시 scoped ref (`VK:*:*`, `VE:*:*`)
- 실행 가능한 명령은 allowlist (`curl`, `git`, `gh`, `glab`) 로 제한
- 실행 문법은 모두 허용
  - `veilkey-cli function run <name> [vault_hash]`
  - `veilkey-cli function <name> [vault_hash]`
  - `veilkey-cli function run <domain> <name> [vault_hash]`
  - `veilkey-cli function <domain> <name> [vault_hash]`
- `vault_hash`를 넘기면 실행 child env에 `VEILKEY_CONTEXT_VAULT_HASH`로 전달됩니다.

도메인별 함수는 `functions/<domain>/<name>.toml` 경로를 우선 사용합니다.

예시:

```toml
name = "gitlab-project-get"
description = "Call GitLab API with VeilKey-managed token"
command = """curl -sS -H "PRIVATE-TOKEN: {%{GITLAB_TOKEN}%}" "https://gitlab.example.com/api/v4/projects/{%{PROJECT_ID}%}" """

[vars]
GITLAB_TOKEN = "VK:EXTERNAL:abcd1234"
PROJECT_ID = "VE:LOCAL:GITLAB_PROJECT_ID"
```

## 아키텍처

```
┌──────────────────────────────────────────────────────────┐
│  vk_wrap (보안 터미널)                                    │
│  ┌─────────┐     ┌──────────────┐     ┌──────────────┐   │
│  │  stdin   │────▶│ 입력 필터    │────▶│  PTY (bash)  │   │
│  │ (키보드) │     │ 5ms 페이스트 │     │              │   │
│  └─────────┘     │ 감지+마스킹  │     │              │   │
│                  └──────────────┘     └──────┬───────┘   │
│                                             │            │
│  ┌─────────┐     ┌──────────────┐           │            │
│  │  stdout  │◀───│ 출력 필터    │◀──────────┘            │
│  │ (화면)   │     │ 30ms 버퍼    │                        │
│  └─────────┘     │ 패턴+워치리스트│                        │
│                  └──────────────┘                        │
├──────────────────────────────────────────────────────────┤
│  SecretDetector: 222 regex 패턴 + Shannon 엔트로피       │
│  VeilKey API: encrypt(평문→VK토큰) / decrypt(VK토큰→평문) │
│  Watchlist: vk 명령으로 등록된 값 실시간 마스킹            │
└──────────────────────────────────────────────────────────┘
```

## 빌드

```bash
make build        # 로컬 빌드
make build-all    # 전체 플랫폼 빌드
make test         # 테스트
make lint         # 린트
make bench        # 벤치마크
make coverage     # 커버리지 리포트
make package      # 배포 패키지 생성
```
