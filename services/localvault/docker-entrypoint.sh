#!/bin/sh
set -e

DATA_DIR="/data"
SALT_FILE="$DATA_DIR/salt"
PASSWORD_FILE="${VEILKEY_PASSWORD_FILE:?VEILKEY_PASSWORD_FILE is required}"

# Reject legacy VEILKEY_PASSWORD env var
if [ -n "${VEILKEY_PASSWORD:-}" ]; then
  echo "ERROR: VEILKEY_PASSWORD env var is no longer supported (exposes password in process environment)."
  echo "Use VEILKEY_PASSWORD_FILE instead (default: /run/secrets/veilkey_password)."
  exit 1
fi

if [ ! -f "$SALT_FILE" ]; then
  if [ ! -f "$PASSWORD_FILE" ]; then
    echo "ERROR: VEILKEY_PASSWORD_FILE ($PASSWORD_FILE) required for first run."
    echo "Mount a Docker secret or bind-mount a password file."
    exit 1
  fi

  MODE="${VEILKEY_MODE:?VEILKEY_MODE is required}"

  case "$MODE" in
    root)
      echo "=== VeilKey Agent Init (root) ==="
      veilkey-localvault init --root < "$PASSWORD_FILE"
      ;;
    child)
      if [ -z "${VEILKEY_PARENT_URL:-}" ]; then
        echo "ERROR: VEILKEY_PARENT_URL required for child mode."
        exit 1
      fi
      LABEL="${VEILKEY_LABEL:?VEILKEY_LABEL is required}"
      echo "=== VeilKey Agent Init (child) ==="
      veilkey-localvault init --child \
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

exec veilkey-localvault "$@"
