# VeilKey CLI

`cli` is the operator-facing self-hosted VeilKey component.

It provides the command-line surface, secure terminal wrapping, and the `veilroot` host boundary.

## Product Position

VeilKey is split into:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - `installer`
  - `keycenter`
  - `localvault`
  - `cli`
  - `proxy`

## Responsibilities

This component owns:

- operator CLI commands
- secure wrapping and masking
- session configuration rendering
- `veilroot` host-boundary scripts
- host-side session helpers

## Core Surfaces

- `vk`
  - CLI entrypoint
- `veilkey-cli wrap-pty`
  - secure PTY wrapper
- `veilroot`
  - host boundary shell and session tooling

## Related Components

- `localvault`
  - local agent and secret/config runtime
- `keycenter`
  - central control plane
- `proxy`
  - outbound enforcement layer used by wrapped workloads

## Masking Behavior

When inside a `veilkey-cli wrap-pty` secure terminal session:

1. On Enter: watchlist values in the current line are masked on screen
2. Automatic VK:hash resolution: a DEBUG trap restores any `VK:xxxx` tokens to their original values before execution

The prompt displays as:

```
🔒VK $ cat .env
DATABASE_PASSWORD=VK:a1b2c3d4    <- VK token shown instead of the actual password
🔒VK $ curl -H "Auth: VK:a1b2c3d4"  <- automatically restored to the original value at execution
```

### vk -- Value Encryption (Watchlist Registration)

The `vk` command encrypts a plaintext value into a VeilKey token. The encrypted value is added to the watchlist and automatically masked inside a wrapped terminal.

```bash
# Hidden input (typing is not displayed)
vk
Enter secret value: ********
Confirm secret value: ********
VK:a1b2c3d4

# Pass via stdin
echo "my-secret" | vk
```

### exec -- Automatic VK Token Decryption and Execution

`veilkey-cli exec` restores VK tokens embedded in command arguments to their original values before executing the command. This allows scripts to use secrets safely without exposing plaintext.

```bash
# Command containing VK tokens -- automatically decrypted before execution
veilkey-cli exec curl -H "Authorization: Bearer VK:a1b2c3d4" https://api.example.com

# VK tokens in environment variables
veilkey-cli exec env DATABASE_URL="postgres://user:VK:b2c3d4e5@db:5432/app" ./migrate
```

## Installation

```bash
# Script install (binary + session config + vk helper)
bash install/install.sh

# Or build directly
make build
go build -o bin/veilkey-session-config ./cmd/veilkey-session-config
cp bin/veilkey-cli bin/veilkey-session-config /usr/local/bin/
cp scripts/vk /usr/local/bin/
```

## Veilroot Host Boundary

`veilroot` host-boundary assets now live in this repository.

Canonical surface:

- `deploy/host/install-veilroot-boundary.sh`
- `deploy/host/install-veilroot-codex.sh`
- `deploy/host/veilroot-shell`
- `deploy/host/veilkey-veilroot-session`
- `deploy/host/veilkey-veilroot-observe`
- `deploy/host/veilkey-veilroot-egress-guard`
- `deploy/host/verify-veilroot-session.sh`
- `cmd/veilkey-session-config`

Typical flow:

```bash
go build -o /usr/local/bin/veilkey-session-config ./cmd/veilkey-session-config
./deploy/host/install-veilroot-boundary.sh
./deploy/host/install-veilroot-codex.sh
/usr/local/bin/veilroot-shell status
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `VEILKEY_LOCALVAULT_URL` | Yes (recommended) | localvault endpoint URL |
| `VEILKEY_API` | No | Legacy endpoint variable (fallback) |
| `VEILKEY_STATE_DIR` | No | State directory (default: `$TMPDIR/veilkey-cli`) |
| `VEILKEY_FUNCTION_DIR` | No | Function catalog directory |

## Commands

| Command | Description |
|---------|-------------|
| `scan [file\|-]` | Detect secrets in a file or stdin (detection only, no API required) |
| `filter [file\|-]` | Replace secrets with VK tokens and write to stdout |
| `wrap <command...>` | Execute a command with automatic stdout replacement |
| `wrap-pty [command]` | Interactive PTY shell with automatic I/O replacement |
| `exec <command...>` | Restore VK tokens to original values and execute the command |
| `resolve <VK:token>` | Restore a VK token to its original value |
| `function <subcommand...>` | Manage repo-tracked TOML function wrappers |
| `list` | List detected VeilKey entries |
| `clear` | Clear session logs |
| `status` | Show current status |
| `version` | Print version |

## Options

| Option | Description |
|--------|-------------|
| `--format <text\|json\|sarif>` | Output format (default: text) |
| `--config <path>` | Project configuration file (default: .veilkey.yml) |
| `--exit-code` | Exit with code 1 when secrets are found (for CI) |
| `--patterns <path>` | Custom patterns file |

## Usage Examples

```bash
# Scan a file (runs locally without an API)
veilkey-cli scan .env
veilkey-cli scan --format json --exit-code .env

# Filter command output
export VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180
kubectl get secret -o yaml | veilkey-cli filter

# CI pre-commit hook (SARIF output)
veilkey-cli scan --exit-code --format sarif src/

# Enter the secure terminal
veilkey-cli wrap-pty

# Encrypt a value
echo "my-api-key" | vk

# List functions
VEILKEY_FUNCTION_DIR=/opt/veilkey/veilkey-cli/functions veilkey-cli function list
```

## Function Catalog

The `function` subcommand reads from the repo-tracked `functions/*.toml` catalog.

- One function per file
- Placeholders use the `{%{NAME}%}` syntax
- Variable values must be scoped refs (`VK:*:*`, `VE:*:*`)
- Executable commands are restricted to an allowlist (`curl`, `git`, `gh`, `glab`)
- All invocation forms are supported:
  - `veilkey-cli function run <name> [vault_hash]`
  - `veilkey-cli function <name> [vault_hash]`
  - `veilkey-cli function run <domain> <name> [vault_hash]`
  - `veilkey-cli function <domain> <name> [vault_hash]`
- When `vault_hash` is provided it is passed to the child process as `VEILKEY_CONTEXT_VAULT_HASH`

Domain-scoped functions are resolved from `functions/<domain>/<name>.toml` first.

Example:

```toml
name = "gitlab-project-get"
description = "Call GitLab API with VeilKey-managed token"
command = """curl -sS -H "PRIVATE-TOKEN: {%{GITLAB_TOKEN}%}" "https://gitlab.example.com/api/v4/projects/{%{PROJECT_ID}%}" """

[vars]
GITLAB_TOKEN = "VK:EXTERNAL:abcd1234"
PROJECT_ID = "VE:LOCAL:GITLAB_PROJECT_ID"
```

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  veilkey-cli wrap-pty (secure terminal)                  │
│  ┌─────────┐     ┌──────────────┐     ┌──────────────┐  │
│  │  stdin   │────>│ Input filter  │────>│  PTY (bash)  │  │
│  │(keyboard)│     │ 5ms paste    │     │              │  │
│  └─────────┘     │ detect+mask  │     │              │  │
│                  └──────────────┘     └──────┬───────┘  │
│                                              │          │
│  ┌─────────┐     ┌──────────────┐            │          │
│  │  stdout  │<───│ Output filter │<───────────┘          │
│  │ (screen) │     │ 30ms buffer  │                       │
│  └─────────┘     │ pattern+watch│                       │
│                  └──────────────┘                       │
├──────────────────────────────────────────────────────────┤
│  SecretDetector: 222 regex patterns + Shannon entropy    │
│  VeilKey API: encrypt(plaintext->VK token) /             │
│               decrypt(VK token->plaintext)               │
│  Watchlist: real-time masking of values registered via vk│
└──────────────────────────────────────────────────────────┘
```

## Build

```bash
make build        # Local build
make build-all    # Cross-platform build
make test         # Run tests
make lint         # Run linter
make bench        # Run benchmarks
make coverage     # Generate coverage report
make package      # Create distribution package
```
