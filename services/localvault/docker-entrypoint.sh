#!/bin/sh
set -e

DATA_DIR="/data"
SALT_FILE="$DATA_DIR/salt"
PASSWORD_FILE="${VEILKEY_PASSWORD_FILE:-}"

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

# Reject legacy VEILKEY_PASSWORD env var
if [ -n "${VEILKEY_PASSWORD:-}" ]; then
  echo "ERROR: VEILKEY_PASSWORD env var is no longer supported (exposes password in process environment)."
  echo "Use VEILKEY_PASSWORD_FILE instead."
  exit 1
fi

if [ ! -f "$SALT_FILE" ] && [ -n "$PASSWORD_FILE" ] && [ -f "$PASSWORD_FILE" ]; then
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
# If no salt and no password file: server starts in web setup mode automatically.

exec veilkey-localvault "$@"
