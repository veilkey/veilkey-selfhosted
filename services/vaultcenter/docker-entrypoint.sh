#!/bin/sh
set -e

DATA_DIR="/data"
SALT_FILE="$DATA_DIR/salt"
ADDR="${VEILKEY_ADDR:?VEILKEY_ADDR is required}"
AUTO_INSTALL_COMPLETE="${VEILKEY_AUTO_COMPLETE_INSTALL_FLOW:?VEILKEY_AUTO_COMPLETE_INSTALL_FLOW is required}"
PASSWORD_FILE="${VEILKEY_PASSWORD_FILE:-}"

# Reject legacy VEILKEY_PASSWORD env var
if [ -n "${VEILKEY_PASSWORD:-}" ]; then
  echo "ERROR: VEILKEY_PASSWORD env var is no longer supported (exposes password in process environment)."
  echo "Use VEILKEY_PASSWORD_FILE instead."
  exit 1
fi

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

if [ ! -f "$SALT_FILE" ] && [ -n "$PASSWORD_FILE" ] && [ -f "$PASSWORD_FILE" ]; then
  MODE="${VEILKEY_MODE:?VEILKEY_MODE is required}"

  case "$MODE" in
    root)
      echo "=== VeilKey HKM Init (root) ==="
      veilkey-vaultcenter init --root < "$PASSWORD_FILE"
      ;;
    child)
      if [ -z "${VEILKEY_PARENT_URL:-}" ]; then
        echo "ERROR: VEILKEY_PARENT_URL required for child mode."
        exit 1
      fi
      LABEL="${VEILKEY_LABEL:?VEILKEY_LABEL is required}"
      echo "=== VeilKey HKM Init (child) ==="
      veilkey-vaultcenter init --child \
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
# If no salt and no password file: server starts in web setup mode automatically.

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
