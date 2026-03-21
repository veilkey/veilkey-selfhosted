#Requires -Version 5.1
<#
.SYNOPSIS
    VeilKey Self-Hosted installer for Windows

.DESCRIPTION
    Docker Desktop 및 Git 설치 여부를 확인하고,
    veilkey-selfhosted 저장소를 클론한 뒤 Docker Compose 로 서비스를 시작합니다.

.PARAMETER InstallDir
    설치 경로 (기본: C:\veilkey)

.PARAMETER VaultCenterPort
    VaultCenter 호스트 포트 (기본: 11181)

.PARAMETER LocalVaultPort
    LocalVault 호스트 포트 (기본: 11180)

.EXAMPLE
    # 관리자 PowerShell에서 실행
    irm https://raw.githubusercontent.com/veilkey/veilkey-selfhosted/main/install/windows/install-veilkey.ps1 | iex

    # 또는 클론 후 직접 실행
    powershell -ExecutionPolicy Bypass -File install\windows\install-veilkey.ps1

    # 경로/포트 커스터마이즈
    powershell -ExecutionPolicy Bypass -File install\windows\install-veilkey.ps1 `
        -InstallDir D:\veilkey -VaultCenterPort 8181

.NOTES
    ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
        귀책사유는 실행자 본인에게 있습니다.
#>

[CmdletBinding()]
param(
    [string]$InstallDir    = "C:\veilkey",
    [int]   $VaultCenterPort = 11181,
    [int]   $LocalVaultPort  = 11180
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# ── 색상 헬퍼 ────────────────────────────────────────────────────────────────
function Write-Step  { param($n, $msg) Write-Host "[$n] $msg" -ForegroundColor Cyan  }
function Write-Ok    { param($msg)     Write-Host "  $msg"    -ForegroundColor Green }
function Write-Warn  { param($msg)     Write-Host "  ⚠  $msg" -ForegroundColor Yellow }
function Write-Err   { param($msg)     Write-Host "  ✗  $msg" -ForegroundColor Red; exit 1 }

# ── 관리자 권한 확인 ──────────────────────────────────────────────────────────
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
        ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Err "관리자 권한으로 실행하세요.`n  오른쪽 클릭 → '관리자 권한으로 실행' 또는:`n  Start-Process powershell -Verb RunAs -ArgumentList '-File install\windows\install-veilkey.ps1'"
}

Write-Host ""
Write-Host "=== VeilKey Installer (Windows) ===" -ForegroundColor Magenta
Write-Host ""
Write-Host "  설치 경로:        $InstallDir"
Write-Host "  VaultCenter 포트: $VaultCenterPort"
Write-Host "  LocalVault 포트:  $LocalVaultPort"
Write-Host ""

# ── [1/5] Git ────────────────────────────────────────────────────────────────
Write-Step "1/5" "Git 확인..."
if (Get-Command git -ErrorAction SilentlyContinue) {
    Write-Ok "Git $(git --version) OK"
} else {
    Write-Host "  Git not found. winget 으로 설치합니다..." -ForegroundColor Yellow
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id Git.Git -e --source winget --silent
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" +
                    [System.Environment]::GetEnvironmentVariable("Path","User")
        if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
            Write-Err "Git 설치 후에도 찾을 수 없습니다. 새 터미널을 열고 다시 실행하세요."
        }
        Write-Ok "Git 설치 완료."
    } else {
        Write-Err "Git 을 찾을 수 없습니다.`n  https://git-scm.com/download/win 에서 수동 설치 후 재실행하세요."
    }
}

# ── [2/5] Docker Desktop ─────────────────────────────────────────────────────
Write-Step "2/5" "Docker 확인..."
if (Get-Command docker -ErrorAction SilentlyContinue) {
    $dockerVer = docker version --format '{{.Server.Version}}' 2>$null
    Write-Ok "Docker $dockerVer OK"

    # docker compose v2 플러그인 확인
    $composeOk = $false
    try { docker compose version 2>$null | Out-Null; $composeOk = $true } catch {}
    if (-not $composeOk) {
        Write-Err "Docker Compose 플러그인을 찾을 수 없습니다.`n  Docker Desktop 을 최신 버전으로 업데이트하세요."
    }
    Write-Ok "Docker Compose $(docker compose version --short) OK"
} else {
    Write-Host "  Docker not found. winget 으로 Docker Desktop 을 설치합니다..." -ForegroundColor Yellow
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id Docker.DockerDesktop -e --source winget --silent
        Write-Warn "Docker Desktop 이 설치되었습니다."
        Write-Warn "Docker Desktop 을 실행하고 엔진이 시작되면 이 스크립트를 다시 실행하세요."
        exit 0
    } else {
        Write-Err "Docker 를 찾을 수 없습니다.`n  https://docs.docker.com/desktop/install/windows-install/ 에서 수동 설치 후 재실행하세요."
    }
}

# Docker 엔진 실행 중인지 확인
try {
    docker info 2>$null | Out-Null
} catch {
    Write-Err "Docker 엔진이 실행 중이지 않습니다.`n  Docker Desktop 을 시작한 뒤 다시 실행하세요."
}

# ── [3/5] 저장소 클론 ─────────────────────────────────────────────────────────
Write-Step "3/5" "저장소 클론 중..."
$repoUrl = "https://github.com/veilkey/veilkey-selfhosted.git"

if (Test-Path (Join-Path $InstallDir ".git")) {
    Write-Host "  이미 설치됨 — 업데이트 중..." -ForegroundColor Yellow
    git -C $InstallDir pull --quiet
} else {
    git clone --quiet $repoUrl $InstallDir
}
Write-Ok "경로: $InstallDir"

# ── [4/5] 환경 설정 ───────────────────────────────────────────────────────────
Write-Step "4/5" "환경 설정..."
$envFile    = Join-Path $InstallDir ".env"
$envExample = Join-Path $InstallDir ".env.example"

if (-not (Test-Path $envFile)) {
    Copy-Item $envExample $envFile

    # Windows 경로를 Docker 가 마운트할 수 있는 형식으로 교체
    # VEIL_WORK_DIR: /Users (macOS 기본값) → C:/Users
    $envContent = Get-Content $envFile -Raw
    $envContent = $envContent -replace '(?m)^(VEIL_WORK_DIR=).*$', "VEIL_WORK_DIR=C:/Users"
    $envContent = $envContent -replace '(?m)^(VAULTCENTER_HOST_PORT=).*$', "VAULTCENTER_HOST_PORT=$VaultCenterPort"
    $envContent = $envContent -replace '(?m)^(LOCALVAULT_HOST_PORT=).*$',  "LOCALVAULT_HOST_PORT=$LocalVaultPort"
    Set-Content $envFile $envContent -NoNewline
    Write-Ok ".env 생성 완료 (필요 시 $envFile 을 수정하세요)."
} else {
    Write-Ok "기존 .env 유지."
}

# ── [5/5] 서비스 시작 ─────────────────────────────────────────────────────────
Write-Step "5/5" "서비스 시작 중 (첫 빌드는 수 분이 걸릴 수 있습니다)..."
Push-Location $InstallDir
try {
    docker compose up -d 2>&1 | Select-Object -Last 8 | ForEach-Object { Write-Host "  $_" }
} finally {
    Pop-Location
}

# ── Health Check ──────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "Health check 대기 중..." -ForegroundColor Cyan
$health = ""
for ($i = 1; $i -le 30; $i++) {
    try {
        # TLS 인증서 검증 무시 (자체 서명 인증서)
        if ($PSVersionTable.PSVersion.Major -ge 6) {
            $resp = Invoke-RestMethod -Uri "https://localhost:$VaultCenterPort/health" `
                        -SkipCertificateCheck -TimeoutSec 3 -ErrorAction Stop
        } else {
            Add-Type @"
using System.Net; using System.Security.Cryptography.X509Certificates;
public class TrustAll : ICertificatePolicy {
    public bool CheckValidationResult(ServicePoint sp, X509Certificate cert, WebRequest req, int err) { return true; }
}
"@ -ErrorAction SilentlyContinue
            [System.Net.ServicePointManager]::CertificatePolicy = New-Object TrustAll
            $resp = Invoke-RestMethod -Uri "https://localhost:$VaultCenterPort/health" `
                        -TimeoutSec 3 -ErrorAction Stop
        }
        if ($resp.status) { $health = $resp | ConvertTo-Json -Compress; break }
    } catch {}
    Start-Sleep -Seconds 5
}

$localIp = (Get-NetIPAddress -AddressFamily IPv4 |
            Where-Object { $_.InterfaceAlias -notmatch 'Loopback' -and $_.IPAddress -notmatch '^169' } |
            Select-Object -First 1).IPAddress

if ($health) {
    Write-Host ""
    Write-Host "=== 설치 완료 ===" -ForegroundColor Green
    Write-Host ""
    Write-Host "  VaultCenter: https://${localIp}:${VaultCenterPort}"
    Write-Host "  LocalVault:  https://${localIp}:${LocalVaultPort}"
    Write-Host "  상태:        $health"
    Write-Host ""
    Write-Host "다음 단계:"
    Write-Host "  1. 초기 설정 (최초 1회):"
    Write-Host "     curl -sk -X POST https://localhost:${VaultCenterPort}/api/setup/init ``"
    Write-Host "       -H 'Content-Type: application/json' ``"
    Write-Host "       -d '{""password"":""<MASTER_PASSWORD>"",""admin_password"":""<ADMIN_PASSWORD>""}'"
    Write-Host ""
    Write-Host "  2. 이미 초기화된 경우 unlock:"
    Write-Host "     curl -sk -X POST https://localhost:${VaultCenterPort}/api/unlock ``"
    Write-Host "       -H 'Content-Type: application/json' ``"
    Write-Host "       -d '{""password"":""<MASTER_PASSWORD>""}'"
    Write-Host ""
    Write-Host "  3. LocalVault 등록: docs\setup.md 참고"
    Write-Host ""
    Write-Host "관리 명령 (설치 경로: $InstallDir):"
    Write-Host "  docker compose ps          # 상태 확인"
    Write-Host "  docker compose logs -f     # 로그 보기"
    Write-Host "  docker compose down        # 중지"
    Write-Host ""
} else {
    Write-Host ""
    Write-Warn "Health check 가 시간 내에 응답하지 않았습니다."
    Write-Host "  서비스가 아직 빌드 중일 수 있습니다. 아래 명령으로 확인하세요:"
    Write-Host "    cd $InstallDir"
    Write-Host "    docker compose ps"
    Write-Host "    docker compose logs"
}
