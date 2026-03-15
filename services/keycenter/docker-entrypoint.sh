#!/bin/sh
set -e

DATA_DIR="/data"
SALT_FILE="$DATA_DIR/salt"
ADDR="${VEILKEY_ADDR:-:10180}"
AUTO_INSTALL_COMPLETE="${VEILKEY_AUTO_COMPLETE_INSTALL_FLOW:-0}"
PASSWORD_FILE="${VEILKEY_PASSWORD_FILE:-/run/secrets/veilkey_password}"

# Reject legacy VEILKEY_PASSWORD env var
if [ -n "${VEILKEY_PASSWORD:-}" ]; then
  echo "ERROR: VEILKEY_PASSWORD env var is no longer supported (exposes password in process environment)."
  echo "Use VEILKEY_PASSWORD_FILE instead (default: /run/secrets/veilkey_password)."
  exit 1
fi

wait_for_http() {
  url="$1"
  retries="${2:-30}"
  while [ "$retries" -gt 0 ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
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

  status="$(curl -fsS "http://127.0.0.1${ADDR}/api/install/state" 2>/dev/null || true)"
  if printf '%s' "$status" | grep -q '"exists":true'; then
    if printf '%s' "$status" | grep -q '"last_stage":"final_smoke"'; then
      echo "Install flow already marked complete."
      return 0
    fi
  fi

  echo "=== VeilKey Install Flow Seed (proof runtime) ==="
  curl -fsS -X POST "http://127.0.0.1${ADDR}/api/install/session" \
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

if [ ! -f "$SALT_FILE" ]; then
  if [ ! -f "$PASSWORD_FILE" ]; then
    echo "ERROR: VEILKEY_PASSWORD_FILE ($PASSWORD_FILE) required for first run."
    echo "Mount a Docker secret or bind-mount a password file."
    exit 1
  fi

  MODE="${VEILKEY_MODE:-root}"

  case "$MODE" in
    root)
      echo "=== VeilKey HKM Init (root) ==="
      veilkey-keycenter init --root < "$PASSWORD_FILE"
      ;;
    child)
      if [ -z "$VEILKEY_PARENT_URL" ]; then
        echo "ERROR: VEILKEY_PARENT_URL required for child mode."
        exit 1
      fi
      LABEL="${VEILKEY_LABEL:-$(hostname)}"
      echo "=== VeilKey HKM Init (child) ==="
      veilkey-keycenter init --child \
        --parent "$VEILKEY_PARENT_URL" \
        --label "$LABEL" < "$PASSWORD_FILE"
      ;;
    *)
      echo "ERROR: Unknown VEILKEY_MODE '$MODE'. Use 'root' or 'child'."
      exit 1
      ;;
  esac

  echo "Init complete."
fi

if [ "$AUTO_INSTALL_COMPLETE" = "1" ]; then
  veilkey-keycenter "$@" &
  server_pid="$!"
  trap 'kill "$server_pid" >/dev/null 2>&1 || true' EXIT INT TERM

  wait_for_http "http://127.0.0.1${ADDR}/health" 30
  seed_install_complete

  kill "$server_pid"
  wait "$server_pid" || true
  trap - EXIT INT TERM
fi

exec veilkey-keycenter "$@"
