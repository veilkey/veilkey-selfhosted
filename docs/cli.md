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

`veil` is a shorthand for `veilkey-cli wrap-pty`. All commands below also work via `veilkey-cli` directly.

## Usage via Docker

The veil container is included in docker-compose:

```bash
# Enter protected session
docker compose exec -it \
  -e DB_PASSWORD=VK:LOCAL:xxxx \
  veil veilkey-cli wrap-pty bash

# Single command
docker compose exec veil veilkey-cli status
docker compose exec veil veilkey-cli resolve VK:LOCAL:xxxx
```

## Configuration

The CLI needs a VeilKey API endpoint:

```bash
export VEILKEY_LOCALVAULT_URL=https://vaultcenter:10181
# Set in docker-compose.yml by default
```

For self-signed certs:
```bash
export VEILKEY_TLS_INSECURE=1
```

## Commands

### scan

Detect secrets in files or stdin. No API connection required.

```bash
# Scan a file
veilkey-cli scan .env

# Scan stdin
cat config.yaml | veilkey-cli scan -

# Scan multiple files
veilkey-cli scan .env config.yaml secrets.json
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
veilkey-cli filter .env

# Filter stdin
echo "TOKEN=ghp_abc123..." | veilkey-cli filter -
# Output: TOKEN=VK:LOCAL:a1b2c3d4
```

The original plaintext is encrypted and stored. The output contains only the `VK:` reference.

### wrap

Execute a command with automatic output masking. Any secret that appears in stdout/stderr is replaced with its `VK:` reference.

```bash
veilkey-cli wrap ./deploy.sh
veilkey-cli wrap env | grep SECRET
```

### wrap-pty

Allocates a PTY with **bidirectional masking**:

```bash
veilkey-cli wrap-pty bash
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
veilkey-cli exec ./my-app
# my-app sees the real API_KEY value in its environment
```

### resolve

Decrypt a single `VK:` token. Requires interactive terminal (TTY) and admin password:

```bash
veilkey-cli resolve VK:LOCAL:a1b2c3d4
# Prompts for admin password, then outputs plaintext
```

> **Security:** `resolve` is blocked in non-TTY (pipe) execution and requires admin authentication on both client and server. AI tools cannot use this command.

### function

Manage function wrappers — shell functions that auto-resolve secrets:

```bash
veilkey-cli function list
veilkey-cli function add my-tool
veilkey-cli function remove my-tool
```

### list

Show secrets detected in the current session:

```bash
veilkey-cli list
```

### status

Show CLI version, API connection, and pattern count:

```bash
veilkey-cli status
```

### clear

Clear the current session's detected secrets:

```bash
veilkey-cli clear
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
| `VEILKEY_LOCALVAULT_URL` | Primary API endpoint |
| `VEILKEY_API` | Alias for `VEILKEY_LOCALVAULT_URL` |
| `VEILKEY_HUB_URL` | Fallback API endpoint |
| `VEILKEY_STATE_DIR` | Session state directory (default: `$TMPDIR/veilkey-cli`) |
| `VEILKEY_FUNCTION_DIR` | Function wrapper directory |
| `VEILKEY_DB_KEY` | Derived from master password (KEK) during unlock. No manual setting needed |
| `VEILKEY_TLS_INSECURE` | Set `1` to skip TLS certificate verification |
| `VEILKEY_CURL_OPTS` | Custom curl options for bulk-apply sync (default: `-sk`) |
