# CLI Reference

`veilkey-cli` is the operator-facing tool for detecting, encrypting, and managing secrets.

## Installation

The CLI is included in all install targets. After installation:

```bash
veilkey-cli version
# or via alias
vk version
```

## Configuration

The CLI needs a VeilKey API endpoint to encrypt/resolve secrets:

```bash
export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180
# or
export VEILKEY_API=http://127.0.0.1:10180
```

On macOS, the `install-mac.sh` installer adds these to `~/.veilkey.sh` automatically.

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

Like `wrap`, but allocates a PTY for interactive commands:

```bash
veilkey-cli wrap-pty bash
veilkey-cli wrap-pty ssh user@host
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

Decrypt a single `VK:` token:

```bash
veilkey-cli resolve VK:LOCAL:a1b2c3d4
```

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
