#!/bin/sh
set -e

DATA_DIR="/data"

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

# If no salt file exists, the server starts in web setup mode automatically.

exec veilkey-localvault "$@"
