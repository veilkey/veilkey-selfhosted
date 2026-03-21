# Windows Installation

Run VeilKey Self-Hosted on Windows using Docker Desktop.

> **Tested on:** Windows 10 (21H2+), Windows 11, Docker Desktop 4.x+

## Quick Start (script)

관리자 PowerShell에서 실행:

```powershell
git clone https://github.com/veilkey/veilkey-selfhosted.git C:\veilkey
cd C:\veilkey
powershell -ExecutionPolicy Bypass -File install\windows\install-veilkey.ps1
```

또는 원격 스크립트 직접 실행:

```powershell
irm https://raw.githubusercontent.com/veilkey/veilkey-selfhosted/main/install/windows/install-veilkey.ps1 | iex
```

스크립트는 Git/Docker 설치 여부 확인 → 저장소 클론 → `.env` 구성 → `docker compose up -d` → health check 순서로 진행됩니다.
전체 옵션은 [install-veilkey.ps1](./install-veilkey.ps1) 참고.

> **Note:** 포트 기본값 — VaultCenter `11181`, LocalVault `11180`.
> 변경 시 `-VaultCenterPort` / `-LocalVaultPort` 파라미터 사용 또는 `.env` 직접 편집.

---

## Requirements

| 항목 | 최소 | 권장 |
|------|------|------|
| OS | Windows 10 21H2 | Windows 11 |
| RAM | 4 GB (Docker Desktop 포함) | 8 GB |
| 디스크 | 20 GB | 40 GB |
| Docker Desktop | 4.x+ | 최신 |

## 1. Docker Desktop 설치

이미 설치되어 있으면 건너뜁니다.

```powershell
winget install --id Docker.DockerDesktop -e
```

또는 [공식 설치 페이지](https://docs.docker.com/desktop/install/windows-install/)에서 다운로드.

설치 후 Docker Desktop 을 실행하고 **엔진이 시작될 때까지 대기**합니다 (트레이 아이콘 → 고래 아이콘이 초록색).

> **WSL 2 백엔드** 사용을 권장합니다. 설치 중 WSL 2 설치를 요청하면 허용하세요.

## 2. 저장소 클론 및 환경 설정

```powershell
git clone https://github.com/veilkey/veilkey-selfhosted.git C:\veilkey
cd C:\veilkey
Copy-Item .env.example .env
notepad .env   # 필요 시 포트 등 수정
```

Windows 에서 주의할 설정 (`C:\veilkey\.env`):

```dotenv
# veil 컨테이너가 마운트할 호스트 경로 (Windows 경로를 슬래시로 표기)
VEIL_WORK_DIR=C:/Users

# 포트 (기본값 그대로 사용 권장)
VAULTCENTER_HOST_PORT=11181
LOCALVAULT_HOST_PORT=11180
```

## 3. 서비스 시작

```powershell
cd C:\veilkey
docker compose up -d
```

첫 실행 시 이미지 빌드로 수 분이 소요됩니다.

## 4. 상태 확인

```powershell
docker compose ps

# Health check (PowerShell 7+)
Invoke-RestMethod -Uri https://localhost:11181/health -SkipCertificateCheck
# Expected: @{status=setup}

# 또는 curl.exe 사용
curl.exe -sk https://localhost:11181/health
```

## 5. 초기 설정

### VaultCenter 초기화

```powershell
# 최초 실행 (status: setup)
curl.exe -sk -X POST https://localhost:11181/api/setup/init `
  -H "Content-Type: application/json" `
  -d '{"password":"<MASTER_PASSWORD>","admin_password":"<ADMIN_PASSWORD>"}'

# 이후 재시작 시 unlock (status: locked)
curl.exe -sk -X POST https://localhost:11181/api/unlock `
  -H "Content-Type: application/json" `
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### LocalVault 등록

```powershell
cd C:\veilkey

# Init
docker compose exec -T localvault sh -c `
  'echo "<MASTER_PASSWORD>" | veilkey-localvault init --root --center https://vaultcenter:10181'

# 재시작 후 unlock
docker compose restart localvault
Start-Sleep 3
curl.exe -sk -X POST https://localhost:11180/api/unlock `
  -H "Content-Type: application/json" `
  -d '{"password":"<MASTER_PASSWORD>"}'
```

### 두 서비스 동시 확인

```powershell
curl.exe -sk https://localhost:11181/health
curl.exe -sk https://localhost:11180/health
# Expected: {"status":"ok"}
```

## 관리 명령

```powershell
cd C:\veilkey
docker compose ps           # 상태 확인
docker compose logs -f      # 실시간 로그
docker compose down         # 중지
docker compose pull; docker compose up -d   # 업데이트
```

## Troubleshooting

### `docker: command not found` / Docker 엔진이 응답하지 않음

Docker Desktop 트레이 아이콘이 초록색인지 확인. 재시작 후 재시도.

### 포트 충돌

`.env` 에서 `VAULTCENTER_HOST_PORT` / `LOCALVAULT_HOST_PORT` 를 변경 후:

```powershell
docker compose down; docker compose up -d
```

### `LOCALVAULT_CHAIN_PEERS` 경고

무해한 경고입니다. `.env` 에 아래 줄 추가로 억제:

```dotenv
LOCALVAULT_CHAIN_PEERS=
```

### `execution of scripts is disabled` 오류

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

전체 설정 가이드는 [Post-Install Setup](../../docs/setup/README.md) 참고.
