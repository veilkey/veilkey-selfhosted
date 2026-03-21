#Requires -Version 5.1
<#
.SYNOPSIS
    VeilKey Self-Hosted uninstaller for Windows

.PARAMETER InstallDir
    제거할 설치 경로 (기본: C:\veilkey)

.PARAMETER VaultCenterPort
    제거할 Firewall 룰의 VaultCenter 포트 (기본: 11181)

.PARAMETER LocalVaultPort
    제거할 Firewall 룰의 LocalVault 포트 (기본: 11180)

.PARAMETER RemoveData
    true 시 data/ 디렉터리(DB, 인증서 등)도 함께 삭제합니다 (기본: false)

.PARAMETER RemoveImages
    true 시 VeilKey Docker 이미지도 삭제합니다 (기본: false)

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File install\windows\uninstall-veilkey.ps1

    # 데이터까지 완전 삭제
    powershell -ExecutionPolicy Bypass -File install\windows\uninstall-veilkey.ps1 -RemoveData -RemoveImages

.NOTES
    ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
        귀책사유는 실행자 본인에게 있습니다.
#>

[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$InstallDir      = "C:\veilkey",
    [int]   $VaultCenterPort = 11181,
    [int]   $LocalVaultPort  = 11180,
    [switch]$RemoveData,
    [switch]$RemoveImages
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$OutputEncoding           = [System.Text.Encoding]::UTF8
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8

function Write-Step { param($n, $msg) Write-Host "[$n] $msg" -ForegroundColor Cyan   }
function Write-Ok   { param($msg)     Write-Host "  ✓ $msg"  -ForegroundColor Green  }
function Write-Warn { param($msg)     Write-Host "  ⚠  $msg" -ForegroundColor Yellow }

if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()
        ).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Host "  ✗ 관리자 권한으로 실행하세요." -ForegroundColor Red; exit 1
}

Write-Host ""
Write-Host "=== VeilKey Uninstaller (Windows) ===" -ForegroundColor Magenta
Write-Host ""
Write-Host "  설치 경로:   $InstallDir"
Write-Host "  데이터 삭제: $($RemoveData.IsPresent)"
Write-Host "  이미지 삭제: $($RemoveImages.IsPresent)"
Write-Host ""

# 확인 프롬프트
$confirm = Read-Host "계속 진행하시겠습니까? [y/N]"
if ($confirm -notmatch '^[yY]$') {
    Write-Host "취소되었습니다." -ForegroundColor Yellow
    exit 0
}
Write-Host ""

# ── [1/4] 서비스 중지 ─────────────────────────────────────────────────────────
Write-Step "1/4" "서비스 중지 중..."
if (Test-Path (Join-Path $InstallDir "docker-compose.yml")) {
    Push-Location $InstallDir
    try {
        if ($RemoveData) {
            docker compose down -v 2>&1 | Select-Object -Last 3 | ForEach-Object { Write-Host "  $_" }
            Write-Ok "서비스 중지 및 볼륨 삭제 완료."
        } else {
            docker compose down 2>&1 | Select-Object -Last 3 | ForEach-Object { Write-Host "  $_" }
            Write-Ok "서비스 중지 완료 (데이터 볼륨 유지)."
        }
    } catch {
        Write-Warn "docker compose down 실패: $_"
    } finally {
        Pop-Location
    }
} else {
    Write-Warn "docker-compose.yml 을 찾을 수 없습니다 ($InstallDir). 서비스 중지 건너뜁니다."
}

# ── [2/4] Firewall 룰 제거 ────────────────────────────────────────────────────
Write-Step "2/4" "Windows Firewall 룰 제거 중..."
foreach ($port in @($VaultCenterPort, $LocalVaultPort)) {
    $ruleName = "VeilKey port $port"
    if (Get-NetFirewallRule -DisplayName $ruleName -ErrorAction SilentlyContinue) {
        Remove-NetFirewallRule -DisplayName $ruleName
        Write-Ok "룰 제거: $ruleName"
    } else {
        Write-Warn "룰 없음 (건너뜀): $ruleName"
    }
}

# ── [3/4] 설치 디렉터리 삭제 ─────────────────────────────────────────────────
Write-Step "3/4" "설치 디렉터리 삭제 중..."
if (Test-Path $InstallDir) {
    if ($RemoveData) {
        Remove-Item $InstallDir -Recurse -Force
        Write-Ok "디렉터리 삭제 완료: $InstallDir"
    } else {
        # data/ 는 유지, 나머지(소스 코드, config) 삭제
        Get-ChildItem $InstallDir -Exclude "data" | Remove-Item -Recurse -Force
        Write-Ok "소스/설정 삭제 완료 (data/ 디렉터리는 $InstallDir\data 에 유지)."
        Write-Warn "데이터까지 삭제하려면 -RemoveData 플래그를 사용하세요."
    }
} else {
    Write-Warn "설치 디렉터리가 없습니다: $InstallDir"
}

# ── [4/4] Docker 이미지 정리 (선택) ──────────────────────────────────────────
Write-Step "4/4" "Docker 이미지 정리..."
if ($RemoveImages) {
    $images = @("veilkey-vaultcenter", "veilkey-localvault", "veilkey-veil")
    foreach ($img in $images) {
        $id = docker images -q $img 2>$null
        if ($id) {
            docker rmi $img 2>$null | Out-Null
            Write-Ok "이미지 삭제: $img"
        } else {
            Write-Warn "이미지 없음 (건너뜀): $img"
        }
    }
} else {
    Write-Ok "이미지 유지 (-RemoveImages 플래그로 삭제 가능)."
}

Write-Host ""
Write-Host "=== 언인스톨 완료 ===" -ForegroundColor Green
Write-Host ""
