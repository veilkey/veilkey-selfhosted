# Proxmox Host LocalVault Commands

이 폴더는 `proxmox-host-localvault` 프로파일을 빠르게 다루기 위한 wrapper 모음이다.

기본 profile:

- `proxmox-host-localvault = localvault`

주요 명령:

- `./scripts/proxmox-host-localvault/plan.sh /`
- `./scripts/proxmox-host-localvault/install.sh --activate /`
- `./scripts/proxmox-host-localvault/configure.sh /`
- `./scripts/proxmox-host-localvault/activate.sh /`
- `./scripts/proxmox-host-localvault/health.sh /`
- `./scripts/proxmox-host-localvault/purge.sh /`

이 폴더는 빠른 운영용 wrapper이고, canonical install engine은 top-level `install.sh`다.

## Command contract

- `plan.sh`
  - 설치/변경 예정 경로만 미리 본다.
- `install.sh --activate /`
  - `install-profile -> init -> activate`를 한 번에 수행한다.
  - live host에서는 DB가 없을 때 root node init까지 포함한다.
- `configure.sh`
  - env/service scaffold만 다시 렌더한다.
- `activate.sh`
  - service enable/restart만 수행한다.
- `health.sh`
  - top-level `post-install-health`를 호출해 설치 후 scaffold 상태를 검증한다.
- `purge.sh`
  - KeyCenter unregister 후 local service/env/binary/data를 제거한다.

## Default behavior

- `VEILKEY_LOCALVAULT_ADDR`
  - 기본값: `0.0.0.0:10180`
- `VEILKEY_LOCALVAULT_DB_PATH`
  - 기본값: `/opt/veilkey/localvault/data/veilkey.db`
- `VEILKEY_LOCALVAULT_TRUSTED_IPS`
  - 기본값: `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1`
- `VEILKEY_KEYCENTER_URL`
  - env가 없으면 `VEILKEY_KEYCENTER_HOST` 환경변수를 이용해 자동 감지한다.
- `VEILKEY_LOCALVAULT_PASSWORD`
  - 권장 경로는 `/etc/veilkey/localvault.password` 같은 password file이다.
  - wrapper input env가 없으면 `/opt/veilkey/data/password`에서 `VEILKEY_PASSWORD`를 읽어 사용한다.

## Destructive behavior

- `purge.sh /`
  - host LocalVault service 정지
  - KeyCenter unregister
  - local env/binary/data 삭제
  - 즉, 완전 삭제용이다
