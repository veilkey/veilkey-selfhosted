# CLI Reference

## Quick Usage

```bash
veil                            # Enter protected shell
veil status                     # Check connection
veil exec echo VK:LOCAL:xxx     # Run command with real values
veil scan file.env              # Find secrets in files

veilkey create [value]          # Create a temp ref (VK:TEMP:xxx)
veilkey resolve VK:LOCAL:xxx    # Decrypt a reference
```

`veil` is a shorthand for `veilkey wrap-pty`. All commands below also work via `veilkey` directly.

## Usage via Docker

The veil container is included in docker-compose:

```bash
# Enter protected session
docker compose exec -it \
  -e DB_PASSWORD=VK:LOCAL:xxxx \
  veil veilkey wrap-pty bash

# Single command
docker compose exec veil veilkey status
docker compose exec veil veilkey resolve VK:LOCAL:xxxx
```

## Configuration

The CLI needs a VeilKey API endpoint:

```bash
export VEILKEY_LOCALVAULT_URL=https://vaultcenter:10181
# Set in docker-compose.yml by default
```

For a private CA or a verified self-signed service certificate, provision [CA trust](./setup/tls.md) first:
```bash
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem
```

Host installers verify TLS by default. The bundled Compose exception is restricted to single-host Docker-internal traffic; never copy it to host or remote clients.

## Commands

### scan

Detect secrets in files or stdin. No API connection required.

```bash
# Scan a file
veilkey scan .env

# Scan stdin
cat config.yaml | veilkey scan -

# Scan multiple files
veilkey scan .env config.yaml secrets.json
```

Output shows each detection with pattern name, confidence score, and matched value.

The scanner uses 222+ built-in patterns covering:
- API keys (AWS, GCP, GitHub, GitLab, Stripe, etc.)
- Passwords and tokens in common formats
- Private keys (RSA, EC, SSH)
- Database connection strings
- Generic high-entropy strings

### filter

Replace detected secrets with `VK:` tokens. Requires API connection.

```bash
# Filter a file
veilkey filter .env

# Filter stdin
echo "TOKEN=ghp_abc123..." | veilkey filter -
# Output: TOKEN=VK:LOCAL:a1b2c3d4
```

The original plaintext is encrypted and stored. The output contains only the `VK:` reference.

### wrap

Execute a command with automatic output masking. Any secret that appears in stdout/stderr is replaced with its `VK:` reference.

```bash
veilkey wrap ./deploy.sh
veilkey wrap env | grep SECRET
```

### wrap-pty

Allocates a PTY with **bidirectional masking**:

```bash
veilkey wrap-pty bash
```

**Bidirectional masking:**
- Environment variables with `VK:` refs are resolved → child process sees real values
- PTY output is filtered → any plaintext matching a resolved value is replaced with its VK ref
- AI tools seeing this output only see `VK:LOCAL:xxxx`, never the actual secret

```bash
# Inside wrap-pty:
$ echo $DB_PASSWORD        # shows VK:LOCAL:ea2bfd16 (masked)
$ cat .env                 # secrets in file also masked
$ node app.js              # app receives real DB_PASSWORD value
```

### exec

Resolve `VK:` tokens in environment variables before executing a command. The inverse of `filter`.

```bash
# .env contains: API_KEY=VK:LOCAL:a1b2c3d4
export $(cat .env | xargs)
veilkey exec ./my-app
# my-app sees the real API_KEY value in its environment
```

### resolve

Decrypt a single `VK:` token. Requires interactive terminal (TTY) and admin password:

```bash
veilkey resolve VK:LOCAL:a1b2c3d4
# Prompts for admin password, then outputs plaintext
```

> **Security:** `resolve` is blocked in non-TTY (pipe) execution and requires admin authentication on both client and server. AI tools cannot use this command.

### function

Manage function wrappers — shell functions that auto-resolve secrets:

```bash
veilkey function list
veilkey function add my-tool
veilkey function remove my-tool
```

### list

Show secrets detected in the current session:

```bash
veilkey list
```

### status

Show CLI version, API connection, and pattern count:

```bash
veilkey status
```

### clear

Clear the current session's detected secrets:

```bash
veilkey clear
```

## Project Config

Place a `.veilkey.yml` in your project root to configure scan behavior:

```yaml
# .veilkey.yml
scan:
  exclude:
    - "*.test.js"
    - "vendor/**"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `VEILKEY_ENV` | Explicit trusted environment file override; otherwise only `$HOME/.veilkey/env` is auto-loaded, never a project file |
| `VEILKEY_LOCALVAULT_URL` | Primary API endpoint |
| `VEILKEY_API` | Alias for `VEILKEY_LOCALVAULT_URL` |
| `VEILKEY_HUB_URL` | Fallback API endpoint |
| `VEILKEY_STATE_DIR` | Session state directory (default: `$TMPDIR/veilkey-cli`) |
| `VEILKEY_FUNCTION_DIR` | Function wrapper directory |
| `VEILKEY_DB_KEY` | Derived from master password (KEK) during unlock. No manual setting needed |
| `VEILKEY_TLS_INSECURE` | Defaults to `0` (verify TLS); do not disable for host/remote traffic |
| `VEILKEY_CA_CERT` | Verified PEM CA bundle for the Rust CLI; public roots used when unset |
| `VEILKEY_CURL_OPTS` | Custom curl options for bulk-apply sync; keep verification enabled and use `--cacert` for private CAs |

## Standalone CLI (without Docker)

The `veilkey` CLI can run directly on a host or LXC without Docker.

### Install

Clone the latest published release tag (replace the placeholder), then run the installation script. The CLI is built from the checkout, not installed from an npm registry package.

```bash
RELEASE_TAG="<latest-release-tag>"
git clone --depth 1 --branch "$RELEASE_TAG" https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
VEILKEY_URL=https://<VC_HOST>:<VC_PORT> bash install/common/install-veil-cli.sh
```

On macOS, use `bash install/macos/veil-cli/install.sh`; that installer retains local `npm install -g "$REPO_ROOT/packages/veil-cli"` linking and codesigning. To update, check out the latest release tag and rerun the same installer. The [standalone LocalVault installer](../install/proxmox-lxc-debian/install-localvault.sh) can also build the CLI.

### Setup

```bash
# Point to your LocalVault
export VEILKEY_LOCALVAULT_URL=https://<LV_IP>:10180
export VEILKEY_TLS_INSECURE=0
export VEILKEY_CA_CERT=/path/to/verified-ca.pem

# Verify
veilkey status
```

### Commands

```bash
veilkey status                  # Connection + patterns info
veilkey scan <file>             # Detect secrets in file
veilkey filter <file>           # Replace secrets with VK:TEMP refs (stdout)
veilkey create [value]          # Create a temp ref
veilkey resolve <VK:ref>        # Decrypt a reference (TTY only)
veilkey exec <command...>       # Resolve VK: refs and run command
veilkey function list           # List global functions
veilkey function add <name>     # Create a global function
veilkey function remove <name>  # Delete a global function
veilkey list                    # List detected secrets in session
veilkey clear                   # Clear session log
veilkey version                 # Show version
```
