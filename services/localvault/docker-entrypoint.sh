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

  echo "=== VeilKey Agent Init ==="
  veilkey-localvault init --root < "$PASSWORD_FILE"

  echo "Init complete."
fi

exec veilkey-localvault "$@"
