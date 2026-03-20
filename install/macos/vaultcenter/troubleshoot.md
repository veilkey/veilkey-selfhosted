# macOS — VaultCenter Setup Troubleshooting

VaultCenter uses auto-generated self-signed TLS certificates. On macOS, this causes browser and CLI trust issues that require manual steps.

## Browser: HTTPS certificate warning

When you open the VaultCenter URL (default `https://localhost:11181`), your browser will show a security warning.

### Chrome

1. On the warning page, type `thisisunsafe` (no input field — just type it)
2. Or: click "Advanced" → "Proceed to localhost (unsafe)"

### Safari

1. Click "Show Details" → "visit this website"
2. Enter your macOS password to confirm

### Firefox

1. Click "Advanced" → "Accept the Risk and Continue"

## CLI: self-signed cert rejection

The `veil` CLI and `curl` will reject self-signed certs by default.

### Option 1: Environment variable (recommended for development)

```bash
export VEILKEY_TLS_INSECURE=1
```

This is already set in `.veilkey/env` by the installer script (`install/macos/veil-cli/install.sh`).

### Option 2: Trust the certificate system-wide

```bash
# Extract the cert from the running container
docker compose cp vaultcenter:/data/tls/cert.pem /tmp/veilkey-cert.pem

# Add to macOS Keychain
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain /tmp/veilkey-cert.pem
```

After this, browsers and CLI tools will trust the certificate without `VEILKEY_TLS_INSECURE=1`.

> **Note:** The certificate is regenerated if you delete `/data/tls/` or recreate the container. You'll need to re-trust it.

## Codesign (macOS Sequoia+)

macOS Sequoia introduced stricter Gatekeeper checks. Unsigned binaries are blocked even from terminal.

The installer script handles this automatically, but if you installed manually:

```bash
sudo codesign --force --sign - $(npm prefix -g)/lib/node_modules/veilkey-cli/native/*
```

This ad-hoc signs all native binaries so Gatekeeper allows execution.

## Docker Desktop: port binding

If port 11181 is already in use:

```bash
# Check what's using the port
lsof -i :${VAULTCENTER_HOST_PORT:-11181}

# Use a different port
VAULTCENTER_HOST_PORT=11182 docker compose up -d

# Then access https://localhost:11182
```

## Common errors

### `curl: (60) SSL certificate problem: self-signed certificate`

Set `VEILKEY_TLS_INSECURE=1` or trust the cert (see above).

### `Error: killed` or `operation not permitted` when running veil

Binary not codesigned. Run the codesign command above.

### VaultCenter URL shows nothing / connection refused

```bash
# Check if services are running
docker compose ps

# Check VaultCenter logs
docker compose logs vaultcenter

# Verify health (adjust port if changed in .env)
curl -sk https://localhost:${VAULTCENTER_HOST_PORT:-11181}/health
```
