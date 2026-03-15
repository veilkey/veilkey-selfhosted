#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: ./scripts/android_adb_connect.sh <host> [port]

Examples:
  ./scripts/android_adb_connect.sh <DEVICE_IP>
  ./scripts/android_adb_connect.sh <DEVICE_IP> 5555
EOF
}

HOST="${1:-}"
PORT="${2:-5555}"

if [[ -z "${HOST}" ]]; then
  usage >&2
  exit 1
fi

command -v adb >/dev/null 2>&1 || {
  echo "Error: adb not found" >&2
  exit 1
}

TARGET="${HOST}:${PORT}"

adb start-server >/dev/null
adb connect "${TARGET}"
adb devices
adb -s "${TARGET}" shell getprop ro.product.model || true
