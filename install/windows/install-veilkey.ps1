#Requires -Version 5.1
<#
.SYNOPSIS
    VeilKey Self-Hosted installer for Windows

.DESCRIPTION
    WSL2 / Docker Desktop 설치 여부를 확인하고,
    veilkey-selfhosted 저장소를 클론한 뒤 Docker Compose 로 서비스를 시작합니다.
    Windows Firewall 인바운드 룰을 추가하고, PTY 마스킹용 veil.ps1 래퍼를 배치합니다.

.PARAMETER InstallDir
    설치 경로 (기본: C:\veilkey)

.PARAMETER VaultCenterPort
    VaultCenter 호스트 포트 (기본: 11181)

.PARAMETER LocalVaultPort
    LocalVault 호스트 포트 (기본: 11180)

.EXAMPLE
    # 관리자 PowerShell에서 실행
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
    [string]$InstallDir      = "C:\veilkey",
    [int]   $VaultCenterPort = 11181,
    [int]   $LocalVaultPort  = 11180
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

# UTF-8 출력 설정
$OutputEncoding                    = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding          = [System.Text.Encoding]::UTF8
[Console]::InputEncoding           = [System.Text.Encoding]::UTF8

# ── 색상 헬퍼 ────────────────────────────────────────────────────────────────
function Write-Step { param($n, $msg) Write-Host "[$n] $msg" -ForegroundColor Cyan   }
function Write-Ok   { param($msg)     Write-Host "  ✓ $msg"  -ForegroundColor Green  }
function Write-Warn { param($msg)     Write-Host "  ⚠  $msg" -ForegroundColor Yellow }
function Write-Fail { param($msg)     Write-Host "  ✗ $msg"  -ForegroundColor Red; exit 1 }

# ── 관리자 권한 확인 ──────────────────────────────────────────────────────────
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
        ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Fail "관리자 권한으로 실행하세요.`n  오른쪽 클릭 → '관리자 권한으로 실행' 또는:`n  Start-Process powershell -Verb RunAs -ArgumentList '-ExecutionPolicy Bypass -File install\windows\install-veilkey.ps1'"
}

Write-Host ""
Write-Host "=== VeilKey Installer (Windows) ===" -ForegroundColor Magenta
Write-Host ""
Write-Host "  설치 경로:        $InstallDir"
Write-Host "  VaultCenter 포트: $VaultCenterPort"
Write-Host "  LocalVault 포트:  $LocalVaultPort"
Write-Host ""

# ── [1/6] WSL2 확인 ───────────────────────────────────────────────────────────
Write-Step "1/6" "WSL2 확인..."
$wslOk = $false
try {
    $wslOut = wsl --status 2>&1
    if ($LASTEXITCODE -eq 0) { $wslOk = $true }
} catch {}

if ($wslOk) {
    Write-Ok "WSL2 활성화됨."
} else {
    Write-Warn "WSL2가 비활성화되어 있습니다. Docker Desktop은 Hyper-V 모드로 실행됩니다."
    Write-Warn "WSL2 활성화를 권장합니다 (성능 및 볼륨 마운트 안정성):"
    Write-Host "    dism.exe /online /enable-feature /featurename:Microsoft-Windows-Subsystem-Linux /all /norestart" -ForegroundColor DarkGray
    Write-Host "    dism.exe /online /enable-feature /featurename:VirtualMachinePlatform /all /norestart" -ForegroundColor DarkGray
    Write-Host "    wsl --set-default-version 2" -ForegroundColor DarkGray
    Write-Host ""
}

# ── [2/6] Git ────────────────────────────────────────────────────────────────
Write-Step "2/6" "Git 확인..."
if (Get-Command git -ErrorAction SilentlyContinue) {
    Write-Ok "$(git --version)"
} else {
    Write-Host "  Git을 찾을 수 없습니다. winget으로 설치합니다..." -ForegroundColor Yellow
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id Git.Git -e --source winget --silent
        # PATH 갱신
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" +
                    [System.Environment]::GetEnvironmentVariable("Path","User")
        if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
            Write-Fail "Git 설치 후에도 인식되지 않습니다. 새 터미널을 열고 다시 실행하세요."
        }
        Write-Ok "Git 설치 완료."
    } else {
        Write-Fail "Git을 찾을 수 없습니다.`n  https://git-scm.com/download/win 에서 설치 후 재실행하세요."
    }
}

# ── [3/6] Docker Desktop ─────────────────────────────────────────────────────
Write-Step "3/6" "Docker 확인..."
$dockerOk = $false
if (Get-Command docker -ErrorAction SilentlyContinue) {
    try {
        docker info 2>$null | Out-Null
        $dockerOk = ($LASTEXITCODE -eq 0)
    } catch {}
}

if ($dockerOk) {
    $dockerVer = docker version --format '{{.Server.Version}}' 2>$null
    Write-Ok "Docker Engine $dockerVer"

    # docker compose v2 플러그인 확인
    $composeOk = $false
    try { docker compose version 2>$null | Out-Null; $composeOk = ($LASTEXITCODE -eq 0) } catch {}
    if (-not $composeOk) {
        Write-Fail "Docker Compose 플러그인을 찾을 수 없습니다.`n  Docker Desktop을 최신 버전으로 업데이트하세요."
    }
    Write-Ok "Docker Compose $(docker compose version --short)"
} else {
    if (Get-Command docker -ErrorAction SilentlyContinue) {
        Write-Fail "Docker 엔진이 응답하지 않습니다. Docker Desktop을 시작한 뒤 다시 실행하세요."
    }
    Write-Host "  Docker Desktop을 찾을 수 없습니다. winget으로 설치합니다..." -ForegroundColor Yellow
    if (Get-Command winget -ErrorAction SilentlyContinue) {
        winget install --id Docker.DockerDesktop -e --source winget --silent
        Write-Warn "Docker Desktop이 설치되었습니다."
        Write-Warn "Docker Desktop을 실행하고 엔진이 시작(트레이 아이콘 초록)되면 이 스크립트를 다시 실행하세요."
        exit 0
    } else {
        Write-Fail "Docker를 찾을 수 없습니다.`n  https://docs.docker.com/desktop/install/windows-install/ 에서 설치 후 재실행하세요."
    }
}

# ── [4/6] 저장소 클론 ─────────────────────────────────────────────────────────
Write-Step "4/6" "저장소 클론 중..."
$repoUrl = "https://github.com/veilkey/veilkey-selfhosted.git"

if (Test-Path (Join-Path $InstallDir ".git")) {
    Write-Host "  이미 설치됨 — 업데이트 중..." -ForegroundColor Yellow
    git -C $InstallDir pull --quiet
} else {
    git clone --quiet $repoUrl $InstallDir
}
Write-Ok "경로: $InstallDir"

# ── [5/6] 환경 설정 ───────────────────────────────────────────────────────────
Write-Step "5/6" "환경 설정..."
$envFile    = Join-Path $InstallDir ".env"
$envExample = Join-Path $InstallDir ".env.example"

# USERPROFILE을 Docker 경로 형식으로 변환 (백슬래시 → 슬래시)
$workDir = $env:USERPROFILE -replace '\\', '/'

if (-not (Test-Path $envFile)) {
    Copy-Item $envExample $envFile

    $envContent = Get-Content $envFile -Raw
    $envContent = $envContent -replace '(?m)^(VEIL_WORK_DIR=).*$',            "VEIL_WORK_DIR=$workDir"
    $envContent = $envContent -replace '(?m)^(VAULTCENTER_HOST_PORT=).*$',    "VAULTCENTER_HOST_PORT=$VaultCenterPort"
    $envContent = $envContent -replace '(?m)^(LOCALVAULT_HOST_PORT=).*$',     "LOCALVAULT_HOST_PORT=$LocalVaultPort"
    Set-Content $envFile $envContent -NoNewline
    Write-Ok ".env 생성 완료 — VEIL_WORK_DIR=$workDir"
    Write-Host "  (포트나 경로 변경이 필요하면 $envFile 을 편집하세요.)" -ForegroundColor DarkGray
} else {
    Write-Ok "기존 .env 유지."
}

# veil.ps1 래퍼를 설치 디렉터리에 복사
$veilWrapperSrc = Join-Path $PSScriptRoot "veil.ps1"
$veilWrapperDst = Join-Path $InstallDir    "veil.ps1"
if (Test-Path $veilWrapperSrc) {
    Copy-Item $veilWrapperSrc $veilWrapperDst -Force
    Write-Ok "veil.ps1 래퍼 배치: $veilWrapperDst"
}

# ── Windows Firewall ──────────────────────────────────────────────────────────
Write-Host "  Windows Firewall 인바운드 룰 설정..." -ForegroundColor Cyan
foreach ($port in @($VaultCenterPort, $LocalVaultPort)) {
    $ruleName = "VeilKey port $port"
    if (-not (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue)) {
        New-NetFirewallRule -DisplayName $ruleName -Direction Inbound `
            -Protocol TCP -LocalPort $port -Action Allow | Out-Null
        Write-Ok "Firewall 룰 추가: TCP $port"
    } else {
        Write-Ok "Firewall 룰 이미 존재: TCP $port"
    }
}

# ── [6/6] 서비스 시작 ─────────────────────────────────────────────────────────
Write-Step "6/6" "서비스 시작 중 (첫 빌드는 5~10분이 걸릴 수 있습니다)..."
Push-Location $InstallDir
try {
    docker compose up -d 2>&1 | Select-Object -Last 8 | ForEach-Object { Write-Host "  $_" }
} finally {
    Pop-Location
}

# ── Health Check ──────────────────────────────────────────────────────────────
Write-Host ""
Write-Host "  VaultCenter health check 대기 중 (최대 5분)..." -ForegroundColor Cyan
$health = $null
for ($i = 1; $i -le 60; $i++) {
    try {
        if ($PSVersionTable.PSVersion.Major -ge 6) {
            $health = Invoke-RestMethod -Uri "https://localhost:$VaultCenterPort/health" `
                          -SkipCertificateCheck -TimeoutSec 3 -ErrorAction Stop
        } else {
            # PowerShell 5.1: TLS 인증서 검증 우회
            if (-not ([System.Management.Automation.PSTypeName]'TrustAllCerts').Type) {
                Add-Type @"
using System.Net;
using System.Security.Cryptography.X509Certificates;
public class TrustAllCerts : ICertificatePolicy {
    public bool CheckValidationResult(ServicePoint sp, X509Certificate cert,
        WebRequest req, int err) { return true; }
}
"@ -ErrorAction SilentlyContinue
            }
            [System.Net.ServicePointManager]::CertificatePolicy = New-Object TrustAllCerts
            [System.Net.ServicePointManager]::SecurityProtocol  =
                [System.Net.SecurityProtocolType]::Tls12 -bor
                [System.Net.SecurityProtocolType]::Tls11
            $health = Invoke-RestMethod -Uri "https://localhost:$VaultCenterPort/health" `
                          -TimeoutSec 3 -ErrorAction Stop
        }
        if ($health.status) { break }
    } catch {}
    Start-Sleep -Seconds 5
}

# Restore default certificate validation
[System.Net.ServicePointManager]::CertificatePolicy = $null

$localIp = (Get-NetIPAddress -AddressFamily IPv4 |
    Where-Object { $_.InterfaceAlias -notmatch 'Loopback' -and
                   $_.IPAddress -notmatch '^169' } |
    Select-Object -First 1).IPAddress

if ($health -and $health.status) {
    $healthJson = $health | ConvertTo-Json -Compress
    Write-Host ""
    Write-Host "=== 설치 완료 ===" -ForegroundColor Green
    Write-Host ""
    Write-Host "  VaultCenter: https://${localIp}:${VaultCenterPort}"
    Write-Host "  LocalVault:  https://${localIp}:${LocalVaultPort}"
    Write-Host "  상태:        $healthJson"
    Write-Host ""
    Write-Host "다음 단계:"
    Write-Host "  1. 초기 설정 (최초 1회):"
    Write-Host "     curl.exe -sk -X POST https://localhost:${VaultCenterPort}/api/setup/init ``"
    Write-Host "       -H `"Content-Type: application/json`" ``"
    Write-Host "       -d `'{`"password`":`"<MASTER_PASSWORD>`",`"admin_password`":`"<ADMIN_PASSWORD>`"}`'"
    Write-Host ""
    Write-Host "  2. 이후 재시작 시 unlock:"
    Write-Host "     curl.exe -sk -X POST https://localhost:${VaultCenterPort}/api/unlock ``"
    Write-Host "       -H `"Content-Type: application/json`" ``"
    Write-Host "       -d `'{`"password`":`"<MASTER_PASSWORD>`"}`'"
    Write-Host ""
    Write-Host "  3. PTY 마스킹 (veil wrap-pty):"
    Write-Host "     cd $InstallDir"
    Write-Host "     powershell -File veil.ps1 wrap-pty bash -c 'aws s3 ls'"
    Write-Host "     # 또는 직접:"
    Write-Host "     docker compose exec -it veil veilkey-cli wrap-pty <명령어>"
    Write-Host ""
    Write-Host "관리 명령 (설치 경로: $InstallDir):"
    Write-Host "  cd $InstallDir"
    Write-Host "  docker compose ps           # 상태 확인"
    Write-Host "  docker compose logs -f      # 로그 보기"
    Write-Host "  docker compose down         # 중지"
    Write-Host ""
    Write-Host "언인스톨:"
    Write-Host "  powershell -ExecutionPolicy Bypass -File $InstallDir\install\windows\uninstall-veilkey.ps1"
    Write-Host ""
} else {
    Write-Host ""
    Write-Warn "Health check 가 시간 내에 응답하지 않았습니다."
    Write-Host "  서비스가 아직 빌드 중일 수 있습니다. 아래 명령으로 확인하세요:"
    Write-Host "    cd $InstallDir"
    Write-Host "    docker compose ps"
    Write-Host "    docker compose logs"
}
