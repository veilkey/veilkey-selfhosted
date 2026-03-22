#!/bin/sh
set -e

# VeilKey Docker entrypoint wrapper
# Resolves VK:LOCAL:xxx and VK:TEMP:xxx in environment variables before starting the app.
#
# Usage in Dockerfile:
#   COPY docker-entrypoint-veilkey.sh /usr/local/bin/
#   COPY --from=veilkey /usr/local/bin/veilkey-cli /usr/local/bin/
#   ENTRYPOINT ["docker-entrypoint-veilkey.sh"]
#   CMD ["node", "app.js"]
#
# Or in docker-compose.yml:
#   services:
#     myapp:
#       entrypoint: ["docker-entrypoint-veilkey.sh"]
#       command: ["node", "app.js"]
#       environment:
#         DB_PASSWORD: "VK:LOCAL:xxx"
#         VEILKEY_LOCALVAULT_URL: "https://localvault:10180"
#         VEILKEY_TLS_INSECURE: "1"
#
# Requires:
#   - veilkey-cli binary in PATH
#   - VEILKEY_LOCALVAULT_URL pointing to VaultCenter or LocalVault

# Check if veilkey-cli exists
if ! command -v veilkey-cli >/dev/null 2>&1; then
    echo "[veilkey-entrypoint] WARNING: veilkey-cli not found — skipping VK ref resolution"
    exec "$@"
fi

# Check if VeilKey URL is set
if [ -z "$VEILKEY_LOCALVAULT_URL" ]; then
    echo "[veilkey-entrypoint] WARNING: VEILKEY_LOCALVAULT_URL not set — skipping VK ref resolution"
    exec "$@"
fi

# Resolve VK refs in all environment variables
resolved=0
failed=0
for var in $(env | grep '=VK:' | cut -d= -f1); do
    value=$(printenv "$var")
    case "$value" in
        VK:LOCAL:*|VK:TEMP:*)
            plaintext=$(veilkey-cli resolve "$value" 2>/dev/null) || true
            if [ -n "$plaintext" ]; then
                export "$var=$plaintext"
                resolved=$((resolved + 1))
            else
                echo "[veilkey-entrypoint] WARNING: failed to resolve $var ($value)"
                failed=$((failed + 1))
            fi
            ;;
    esac
done

if [ "$resolved" -gt 0 ] || [ "$failed" -gt 0 ]; then
    echo "[veilkey-entrypoint] resolved $resolved variable(s), $failed failed"
fi

exec "$@"
