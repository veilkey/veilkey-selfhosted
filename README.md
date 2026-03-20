<div align="center">
  <img src=".github/banner.png" alt="VeilKey" width="720">
  <h1>VeilKey Self-Hosted</h1>
  <p><strong>Secret management where AI never sees your passwords.<br>PTY-level bidirectional masking + blockchain audit.</strong></p>
  <p>
    <a href="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml"><img src="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
    <a href="https://github.com/veilkey/veilkey-selfhosted/releases"><img src="https://img.shields.io/github/v/release/veilkey/veilkey-selfhosted?display_name=tag" alt="GitHub release"></a>
    <a href="https://www.npmjs.com/package/veilkey-cli"><img src="https://img.shields.io/npm/v/veilkey-cli?color=cb3837" alt="npm"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-2563eb.svg" alt="MIT"></a>
  </p>
</div>

## What is VeilKey?

<div align="center">
  <img src=".github/concept.jpeg" alt="VeilKey concept — AI sees VK:AF1Y45, real key is PASS1234" width="640">
  <p><em>AI sees <code>VK:AF1Y45</code> on screen. The real key <code>PASS1234</code> stays hidden.</em></p>
</div>

VeilKey was built for one reason: **AI should never see your secrets.**

AI coding tools (Claude Code, Cursor, Copilot) read your terminal output, environment variables, and files. If a password appears anywhere, AI sees it. VeilKey wraps your terminal in a PTY filter that replaces real secrets with encrypted references in real time.

```bash
# Inside a veil shell
$ cat .env
DB_PASSWORD=VK:LOCAL:ea2bfd16    # What AI sees

# The actual app receives the real password
$ npm start                       # DB_PASSWORD=actual-secret
```

When secrets appear in PTY output, they are automatically replaced with VK refs. 222 built-in patterns detect API keys, tokens, and passwords on the fly — even ones you haven't registered yet.

## Architecture

<div align="center">
  <img src=".github/architecture.jpeg" alt="VaultCenter and LocalVaults" width="720">
  <p><em>VaultCenter manages encryption keys. LocalVaults store ciphertext only.</em></p>
</div>

```
VaultCenter (key manager)          LocalVault (vault)
┌──────────────────────┐          ┌──────────────────┐
│ agentDEK (encryption │          │ ciphertext only   │
│ key, KEK-protected)  │          │ cannot decrypt    │
│ blockchain audit log │          │                   │
└──────────────────────┘          └──────────────────┘
         │                                  │
         └──── both required to decrypt ────┘

veil CLI (PTY masking)
┌──────────────────────────────────────────┐
│ env vars: VK:LOCAL:xxx → real value (app)│
│ output: real value → VK:LOCAL:xxx (AI)   │
└──────────────────────────────────────────┘
```

**Both must be compromised to access secrets:**
- VaultCenter only → has agentDEK but no ciphertext
- LocalVault only → has ciphertext but no agentDEK

## Installation

### macOS

```bash
# Option 1: npm (recommended)
npm install -g veilkey-cli
sudo codesign --force --sign - $(npm prefix -g)/lib/node_modules/veilkey-cli/native/*

# Option 2: source build + Docker
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
bash scripts/install-veil-mac.sh
```

After install:
1. Open `https://localhost:11181` → set master + admin password
2. `cd veilkey-selfhosted && veil` → enter protected shell

### Linux

```bash
# 1. Dependencies
sudo apt install -y git docker.io docker-compose-plugin nodejs npm
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env

# 2. Clone + start services
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
cp .env.example .env
docker compose up -d

# 3. Install CLI
npm install -g veilkey-cli

# 4. Setup + enter
# https://localhost:11181 → set passwords
veil
```

### Update

```bash
npm update -g veilkey-cli          # CLI update
cd veilkey-selfhosted && git pull  # Server update
docker compose up --build -d       # Docker rebuild
```

### Add a LocalVault

Add a LocalVault to an existing VaultCenter:

```bash
curl -sL "https://gist.githubusercontent.com/dalsoop/11e00346263678340189cdfdc79644b5/raw/install-localvault.sh?$(date +%s)" | \
  VEILKEY_CENTER_URL=https://your-vaultcenter:11181 bash
```

Or via `veil` CLI:
```bash
veil localvault init      # Install + start
veil localvault stop      # Stop
veil localvault log       # Tail logs
veil localvault status    # Health check
```

### Setup (after install)

1. **`https://localhost:11181`** → set master + admin password
2. **Register LocalVault** — issue a registration token from keycenter:
```bash
docker compose exec localvault sh -c \
  "echo 'password' | veilkey-localvault init --root \
    --token vk_reg_xxx --center https://vaultcenter:10181"
docker compose restart localvault
```
3. **Store secrets** — create temp keys in keycenter → promote to vault
4. **`veil`** → enter protected shell. All registered secrets are auto-masked.

## Key Features

### PTY Bidirectional Masking
```bash
# Inside veil shell — safe even if AI reads this output
$ echo $DB_PASSWORD
VK:LOCAL:ea2bfd16              # masked (real value: actual-password)

$ cat config.env
DB_PASSWORD=VK:LOCAL:ea2bfd16  # file reads masked too

# Real processes receive the actual value
$ node app.js                  # process.env.DB_PASSWORD = "actual-password"
```

### Real-time Pattern Detection
- 222 built-in patterns (npm tokens, AWS keys, GitHub PATs, etc.)
- Secrets detected in output are auto-registered as VK:TEMP
- No manual registration needed — just use `veil`

### CometBFT Blockchain Audit
- All key create/rotate/delete operations recorded on immutable chain
- LocalVault validates blocks as full node → prevents VaultCenter tampering
- DB hack detectable via broken block hash chain

### Split Storage
- VaultCenter: holds agentDEK (encryption key)
- LocalVault: holds ciphertext only (cannot decrypt)
- Single compromise = no access to secrets

### Admin Web UI
- Keycenter: temp key CRUD, vault promotion, registration tokens
- Vault management: secret browsing, function bindings, config
- Audit log: full key operation history

## Repository Structure

```
services/
  vaultcenter/     # Central management server (Go)
  localvault/      # Local vault (Go)
  veil-cli/        # veil, veilkey, veilkey-cli (Rust)
packages/
  veil-cli/        # npm package wrapper
docker-compose.yml # Full stack (VC + LV + veil)
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `veil` | Enter protected PTY session |
| `veil status` | Show connection status |
| `veil resolve VK:LOCAL:xxx` | Resolve ref to actual value |
| `veil exec echo VK:LOCAL:xxx` | Replace refs in args and execute |
| `veil scan file.env` | Detect secrets (222 patterns) |
| `veil localvault init` | Install LocalVault in current directory |

## Comparison

| Feature | 1Password CLI | Doppler | HashiCorp Vault | **VeilKey** |
|---------|---------------|---------|-----------------|-------------|
| Secret storage | Yes | Yes | Yes | Yes |
| Reference system | `op://` | No | No | `VK:LOCAL:` |
| Env var injection | Yes | Yes | Yes | Yes |
| **PTY output masking** | No | No | No | **Yes** |
| **Bidirectional replacement** | No | No | No | **Yes** |
| **File read masking** | No | No | No | **Yes** |
| **Pattern auto-detection** | No | No | No | **Yes (222)** |
| Blockchain audit | No | No | No | **Yes** |
| Split storage (VC/LV) | No | No | No | **Yes** |
| Self-hosted | No | No | Yes | **Yes** |

## Environment Variables

See `.env.example` files for all configurable values:
- `services/vaultcenter/.env.example`
- `services/localvault/.env.example`

Key settings:

| Variable | Default | Description |
|----------|---------|-------------|
| `VEILKEY_TEMP_REF_TTL` | `1h` | Temp key expiry |
| `VEILKEY_ADMIN_SESSION_TTL` | `2h` | Admin session duration |
| `VEILKEY_CHAIN_HOME` | `/data/chain` | CometBFT data path |
| `VEILKEY_TLS_INSECURE` | `0` | Allow self-signed certs |

## How It Works (server restart)

When VeilKey server restarts, you must **re-enter the master password**.

```
Server start → LOCKED (DEK not in memory)
  → Enter master password in web UI
  → KEK derived → DEK decrypted → loaded into memory
  → UNLOCKED (normal operation)
```

**The password is never stored on disk.** KEK is derived from password + salt each time. DEK exists only in encrypted form in DB. When the server shuts down, both KEK and DEK are wiped from memory.

You can set `VEILKEY_PASSWORD_FILE` for auto-unlock, but securing that file is your responsibility.

## Security

**Never run AI tools with root privileges.**

VeilKey prevents AI from accessing secrets, but with root access:
- Process memory dump → DEK extraction possible
- Direct `/data/` access → DB file manipulation possible
- PTY masking bypass → raw output via `/proc/{pid}/fd/`

**Recommendations:**
- Run AI coding tools as a regular user
- Work inside `veil` shell only → PTY masking guaranteed
- Perform `sudo` operations outside veil

## Security Disclaimer

VeilKey is a security-sensitive tool that handles secrets and cryptographic material.
This software is provided WITHOUT WARRANTY. Before using VeilKey in production,
conduct your own security audit and review.

If you discover a security issue, please report it privately via GitHub Security Advisories.

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md).

## License

MIT License. See [`LICENSE`](./LICENSE).

---

<sub>Images in this README are AI-generated. 본 README의 이미지는 AI로 생성되었습니다.</sub>
