# Windows Installation

Run VeilKey Self-Hosted on Windows using Docker Desktop.

> **Tested on:** Windows 10 21H2+, Windows 11, Docker Desktop 4.x+

## Quick Start (script)

관리자 PowerShell에서 실행:

```powershell
git clone https://github.com/veilkey/veilkey-selfhosted.git C:\veilkey
cd C:\veilkey
powershell -ExecutionPolicy Bypass -File install\windows\install-veilkey.ps1
```

스크립트 실행 흐름:
1. WSL2 활성화 확인 (미활성 시 경고)
2. Git / Docker Desktop 설치 확인 (없으면 winget 자동 설치)
3. 저장소 클론 또는 업데이트
4. `.env` 구성 (`VEIL_WORK_DIR` 자동 설정, Firewall 룰 추가)
5. `docker compose up -d` 실행
6. Health check 대기

전체 파라미터는 [install-veilkey.ps1](./install-veilkey.ps1) 참고.

> **포트 기본값** — VaultCenter `11181`, LocalVault `11180`.
> 변경 시 `-VaultCenterPort` / `-LocalVaultPort` 파라미터 사용 또는 `.env` 직접 편집.

---

## Requirements

| 항목 | 최소 | 권장 |
|------|------|------|
| OS | Windows 10 21H2 | Windows 11 |
| RAM | 4 GB (Docker Desktop 포함) | 8 GB |
| 디스크 | 20 GB | 40 GB |
| Docker Desktop | 4.x+ | 최신 |
| WSL2 | 선택 | **권장** (볼륨 마운트 안정성) |

## 1. WSL2 활성화 (권장)

Docker Desktop은 WSL2 없이도 동작하지만, 볼륨 마운트 성능과 안정성을 위해 WSL2를 권장합니다.

```powershell
# 관리자 PowerShell에서 실행
dism.exe /online /enable-feature /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart
dism.exe /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart
wsl --update
wsl --set-default-version 2
```

재부팅 후 Docker Desktop → Settings → General → **Use the WSL 2 based engine** 활성화.

## 2. Docker Desktop 설치

이미 설치되어 있으면 건너뜁니다.

```powershell
winget install --id Docker.DockerDesktop -e
```

또는 [공식 설치 페이지](https://docs.docker.com/desktop/install/windows-install/).

설치 후 Docker Desktop 실행 → 트레이 아이콘이 초록색이 될 때까지 대기.

## 3. 저장소 클론 및 환경 설정

```powershell
git clone https://github.com/veilkey/veilkey-selfhosted.git C:\veilkey
cd C:\veilkey
Copy-Item .env.example .env
notepad .env   # 필요 시 편집
```

Windows에서 주의할 `.env` 항목:

```dotenv
# veil 컨테이너가 마운트할 호스트 경로 (슬래시 사용, 백슬래시 ×)
VEIL_WORK_DIR=C:/Users/YourName

# 포트
VAULTCENTER_HOST_PORT=11181
LOCALVAULT_HOST_PORT=11180
```

> `install-veilkey.ps1`은 `VEIL_WORK_DIR`을 `$env:USERPROFILE` 값으로 자동 설정합니다.

## 4. 서비스 시작

```powershell
cd C:\veilkey
docker compose up -d
```

첫 실행 시 이미지 빌드로 5~10분이 소요됩니다.

## 5. 상태 확인

```powershell
docker compose ps

# Health check (curl.exe 사용 — PowerShell 의 curl 별칭 아님)
curl.exe -sk https://localhost:11181/health
# Expected: {"status":"setup"}

# PowerShell 7+ 사용 시
Invoke-RestMethod -Uri https://localhost:11181/health -SkipCertificateCheck
```

## 6. 초기 설정

### VaultCenter 초기화

```powershell
# 최초 실행 (status: "setup")
curl.exe -sk -X POST https://localhost:11181/api/setup/init `
  -H "Content-Type: application/json" `
  -d "{`"password`":`"<MASTER_PASSWORD>`",`"admin_password`":`"<ADMIN_PASSWORD>`"}"

# 재시작 후 unlock (status: "locked")
curl.exe -sk -X POST https://localhost:11181/api/unlock `
  -H "Content-Type: application/json" `
  -d "{`"password`":`"<MASTER_PASSWORD>`"}"
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
  -d "{`"password`":`"<MASTER_PASSWORD>`"}"
```

## 7. PTY 마스킹 (wrap-pty) — Windows 제약 및 우회 방법

> **⚠️ wrap-pty는 Windows 호스트에서 직접 실행할 수 없습니다.**
>
> veil-cli의 PTY 마스킹 기능은 Unix libc(`openpty`, `fork`, `setsid`, `ioctl`) 전용으로 구현되어 있습니다.
> Windows 호스트에서 직접 실행 시 "wrap-pty is not supported on this platform" 오류가 발생합니다.

### 우회 방법: veil.ps1 래퍼

`install-veilkey.ps1`이 `C:\veilkey\veil.ps1`을 자동으로 배치합니다.
이 래퍼는 veil 컨테이너 내부(`Linux`)에서 wrap-pty를 실행합니다.

```powershell
cd C:\veilkey

# veil.ps1 래퍼 사용
.\veil.ps1 wrap-pty bash -c 'aws s3 ls'
.\veil.ps1 wrap-pty python script.py

# 또는 docker compose exec 직접 사용
docker compose exec -it veil veilkey-cli wrap-pty bash -c 'aws s3 ls'
```

### 동작 원리

```
Windows PowerShell
    └─ veil.ps1
          └─ docker compose exec -it veil veilkey-cli wrap-pty <cmd>
                └─ veil 컨테이너 (Linux Alpine)
                      └─ PTY 마스킹 적용 ✓
```

## 관리 명령

```powershell
cd C:\veilkey
docker compose ps           # 상태 확인
docker compose logs -f      # 실시간 로그
docker compose down         # 중지 (데이터 유지)
docker compose pull; docker compose up -d   # 업데이트
```

## 언인스톨

```powershell
# 소스/설정만 삭제 (데이터 유지)
powershell -ExecutionPolicy Bypass -File C:\veilkey\install\windows\uninstall-veilkey.ps1

# 데이터 포함 완전 삭제
powershell -ExecutionPolicy Bypass -File C:\veilkey\install\windows\uninstall-veilkey.ps1 -RemoveData -RemoveImages
```

## Troubleshooting

### Docker 엔진이 응답하지 않음

Docker Desktop 트레이 아이콘이 초록색인지 확인. 아니면 재시작:

```powershell
Start-Process "C:\Program Files\Docker\Docker\Docker Desktop.exe"
```

### 포트 충돌

`.env`에서 포트 변경 후 재시작:

```powershell
cd C:\veilkey
# .env 편집 후:
docker compose down; docker compose up -d
```

### VEIL_WORK_DIR 마운트 오류

Docker Desktop → Settings → Resources → File Sharing 에서 해당 드라이브가 공유 허용되어 있는지 확인.

### `LOCALVAULT_CHAIN_PEERS` 경고

무해한 경고입니다. `.env`에 추가하면 억제됩니다:

```dotenv
LOCALVAULT_CHAIN_PEERS=
```

### `execution of scripts is disabled` 오류

```powershell
Set-ExecutionPolicy -Scope CurrentUser RemoteSigned
```

전체 설정 가이드는 [Post-Install Setup](../../docs/setup/README.md) 참고.
