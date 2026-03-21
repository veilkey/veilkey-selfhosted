#Requires -Version 5.1
<#
.SYNOPSIS
    veil wrap-pty 래퍼 — Windows에서 PTY 마스킹을 컨테이너 내부로 우회합니다.

.DESCRIPTION
    veil-cli의 wrap-pty 기능은 Unix libc 전용으로 Windows 호스트에서 직접 실행할 수 없습니다.
    이 스크립트는 veil 컨테이너 내부에서 wrap-pty 를 실행하는 얇은 래퍼입니다.

.EXAMPLE
    # 기본 사용법 (설치 경로: C:\veilkey)
    cd C:\veilkey
    .\veil.ps1 wrap-pty bash -c 'aws s3 ls'
    .\veil.ps1 wrap-pty python script.py

    # 다른 설치 경로인 경우
    cd D:\veilkey
    .\veil.ps1 wrap-pty bash

.NOTES
    이 파일은 install-veilkey.ps1 이 $InstallDir 에 자동으로 배치합니다.
    docker-compose.yml 이 있는 디렉터리에서 실행해야 합니다.
#>

param([Parameter(ValueFromRemainingArguments)][string[]]$Args)

# docker-compose.yml 위치 확인 (이 스크립트가 설치 디렉터리에 있다고 가정)
$composeDir = $PSScriptRoot
if (-not (Test-Path (Join-Path $composeDir "docker-compose.yml"))) {
    Write-Error "docker-compose.yml 을 찾을 수 없습니다: $composeDir`n이 스크립트는 veilkey 설치 디렉터리에서 실행해야 합니다."
    exit 1
}

# veil 컨테이너 실행 여부 확인
Push-Location $composeDir
try {
    $running = docker compose ps --status running --format json 2>$null |
                   ConvertFrom-Json -ErrorAction SilentlyContinue |
                   Where-Object { $_.Service -eq "veil" }
    if (-not $running) {
        Write-Warning "veil 컨테이너가 실행 중이지 않습니다."
        Write-Host "  아래 명령으로 서비스를 먼저 시작하세요:" -ForegroundColor Yellow
        Write-Host "    cd $composeDir" -ForegroundColor DarkGray
        Write-Host "    docker compose up -d" -ForegroundColor DarkGray
        exit 1
    }

    # wrap-pty 를 컨테이너 내부에서 실행
    docker compose exec -it veil veilkey wrap-pty @Args
} finally {
    Pop-Location
}
