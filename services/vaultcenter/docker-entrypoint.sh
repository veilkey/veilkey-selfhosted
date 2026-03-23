#!/bin/sh
set -e

DATA_DIR="/data"
ADDR="${VEILKEY_ADDR:?VEILKEY_ADDR is required}"

# Auto-generate self-signed TLS certificate on first run
CERT_DIR="$DATA_DIR/certs"
CERT_FILE="$CERT_DIR/server.crt"
KEY_FILE="$CERT_DIR/server.key"
if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
  mkdir -p "$CERT_DIR"
  openssl req -x509 -newkey rsa:4096 -keyout "$KEY_FILE" -out "$CERT_FILE" \
    -days 3650 -nodes -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" 2>/dev/null
  echo "TLS certificate generated."
fi
export VEILKEY_TLS_CERT="$CERT_FILE"
export VEILKEY_TLS_KEY="$KEY_FILE"
AUTO_INSTALL_COMPLETE="${VEILKEY_AUTO_COMPLETE_INSTALL_FLOW:?VEILKEY_AUTO_COMPLETE_INSTALL_FLOW is required}"

wait_for_http() {
  url="$1"
  retries="${2:-30}"
  while [ "$retries" -gt 0 ]; do
    if curl -fsSk "$url" >/dev/null 2>&1; then
      return 0
    fi
    retries=$((retries - 1))
    sleep 1
  done
  return 1
}

seed_install_complete() {
  if [ "$AUTO_INSTALL_COMPLETE" != "1" ]; then
    return 0
  fi

  status="$(curl -fsSk "https://127.0.0.1${ADDR}/api/install/state" 2>/dev/null || curl -fsS "http://127.0.0.1${ADDR}/api/install/state" 2>/dev/null || true)"
  if printf '%s' "$status" | grep -q '"exists":true'; then
    if printf '%s' "$status" | grep -q '"last_stage":"final_smoke"'; then
      echo "Install flow already marked complete."
      return 0
    fi
  fi

  echo "=== VeilKey Install Flow Seed (proof runtime) ==="
  curl -fsSk -X POST "https://127.0.0.1${ADDR}/api/install/session" \
    -H "Content-Type: application/json" \
    -d '{
      "session_id":"proof-runtime-install",
      "version":1,
      "language":"ko",
      "quickstart":true,
      "flow":"quickstart",
      "deployment_mode":"container-compose",
      "install_scope":"proof-runtime",
      "bootstrap_mode":"email",
      "mail_transport":"smtp-mock",
      "planned_stages":["language","bootstrap","final_smoke"],
      "completed_stages":["language","bootstrap","final_smoke"],
      "last_stage":"final_smoke"
    }' >/dev/null
}

# If no salt file exists, the server starts in web setup mode automatically.

if [ "$AUTO_INSTALL_COMPLETE" = "1" ]; then
  veilkey-vaultcenter "$@" &
  server_pid="$!"
  trap 'kill "$server_pid" >/dev/null 2>&1 || true' EXIT INT TERM

  wait_for_http "https://127.0.0.1${ADDR}/health" 30
  seed_install_complete

  kill "$server_pid"
  wait "$server_pid" || true
  trap - EXIT INT TERM
fi

exec veilkey-vaultcenter "$@"
