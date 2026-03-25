# Architecture

## Overview

VeilKey is a self-hosted secret management system with PTY-level bidirectional masking. AI coding tools never see plaintext secrets.

```
VaultCenter (열쇠)              LocalVault (금고)
┌───────────────────┐          ┌──────────────────┐
│ agentDEK 보관      │          │ ciphertext 저장    │
│ 암호화/복호화 수행   │          │ 복호화 불가        │
│ CometBFT 감사 체인  │          │ heartbeat 전송     │
│ 관리자 웹 UI       │          │                    │
└───────────────────┘          └──────────────────┘
         │                              │
         └──── 둘 다 있어야 복호화 ────────┘

veil CLI (PTY 마스킹)
┌──────────────────────────────────────────┐
│ env: VK:LOCAL:xxx → 실제 값 (프로세스)     │
│ output: 실제 값 → VK:LOCAL:xxx (화면/AI)  │
└──────────────────────────────────────────┘
```

## Components

### VaultCenter

Central management server. Single instance per deployment.

**Responsibilities:**
- agentDEK management — per-vault encryption key, encrypted with VC's KEK
- Admin web UI — keycenter, vault management, audit log
- CometBFT blockchain — immutable audit trail for all key operations
- Registration token issuance — secure LocalVault onboarding
- Tracked ref registry — maps VK refs to vault/agent

**Does NOT:**
- Store ciphertext (only agentDEK)
- Auto-unlock (starts LOCKED; requires master password via `POST /api/unlock`)
- Allow password files or env vars for auto-unlock (KEK is memory-only)

**Port:** `:10181` (host: `11181`)

### LocalVault

Node-local storage vault. One per machine/container.

**Responsibilities:**
- Ciphertext storage (encrypted with agentDEK by VaultCenter)
- Heartbeat to VaultCenter (5-minute interval)
- Registration token consumption (one-time)
- Config storage

**Does NOT:**
- Decrypt secrets (no agentDEK, no decrypt/promote/resolve endpoints)
- Make policy decisions
- Store encryption keys

**Port:** `:10180` (host: `11180`)

### veil-cli (Rust) — `services/veil-cli/`

Single Rust package, 4 binaries:

| Binary | Purpose |
|--------|---------|
| `veil` | Enter protected session (PTY masking shell) |
| `veilkey` | State, crypto, policy control |
| `veilkey-cli` | CLI tools: scan, filter, wrap-pty, exec, resolve |
| `veilkey-session-config` | Session TOML config loader |

**veilkey commands:**

| Command | Description | API Required |
|---------|-------------|:---:|
| `scan [file\|-]` | Detect secrets (222+ patterns) | No |
| `filter [file\|-]` | Replace secrets with VK tokens | Yes |
| `wrap-pty [cmd]` | Interactive PTY with bidirectional masking | Yes |
| `exec <cmd>` | Resolve VK tokens in args before execution | Yes |
| `resolve <VK:ref>` | Decrypt a single token | Yes |
| `status` | Show connection status | No |

## Encryption Model

### Key Hierarchy

```
Master Password (operator's memory, setup time only)
  → KEK (PBKDF2/scrypt, never stored)
    → agentDEK (per-vault, stored encrypted with KEK in VC)
      → ciphertext (stored in LV, encrypted with agentDEK)
```

### Security Properties

| Scenario | Result |
|----------|--------|
| LV compromised | ciphertext only, no agentDEK → cannot decrypt |
| VC compromised | agentDEK (encrypted), no ciphertext → cannot decrypt |
| Both compromised | agentDEK + ciphertext, but KEK needed to unwrap agentDEK |
| KEK extracted | Requires master password → only in operator's memory |

### Scoped References

| Scope | Meaning |
|-------|---------|
| `VK:LOCAL:xxxx` | Encrypted by agentDEK, stored in LocalVault |
| `VK:TEMP:xxxx` | Temporary token (1h expiry, stored in VaultCenter) |
| `VK:EXTERNAL:xxxx` | Reference to external secret provider |

## Data Flow

### Promote (Keycenter → Vault)

```
1. Admin creates temp key in keycenter (name + value)
2. VaultCenter encrypts value with VC's DEK → VK:TEMP ref
3. Admin selects vault → "promote to vault"
4. VaultCenter resolves VK:TEMP → plaintext (in memory)
5. VaultCenter encrypts with agentDEK → ciphertext
6. VaultCenter sends ciphertext to LV POST /api/cipher
7. LV stores ciphertext (cannot decrypt)
8. VaultCenter registers tracked ref (VK:LOCAL:{ref})
```

### Resolve (API / CLI)

```
1. GET /api/resolve/VK:LOCAL:{ref}
2. VaultCenter finds tracked ref → agent_hash → agent record
3. VaultCenter decrypts agentDEK with KEK
4. VaultCenter fetches ciphertext from LV GET /api/cipher/{ref}
5. VaultCenter decrypts with agentDEK → plaintext
6. Returns plaintext (never touches LV)
```

### PTY Masking (wrap-pty)

```
1. wrap-pty starts, scans env vars for VK: refs
2. Resolves each ref → plaintext via VaultCenter API
3. Builds mask_map: {plaintext → VK ref}
4. Spawns child process with resolved env vars
5. Child output passes through PTY filter:
   - Any plaintext match → replaced with VK ref
6. Screen/AI sees only VK refs, child sees real values
```

## CometBFT Blockchain

All write operations go through chain transactions:

| TX Type | Operation |
|---------|-----------|
| TxSaveTokenRef | Create/update tracked ref |
| TxUpsertAgent | Agent registration/heartbeat |
| TxRegisterChild | Child node registration |
| TxSaveBinding | Function binding |
| TxSetConfig | Config storage |
| TxRecordAuditEvent | Explicit audit with metadata |

**Automatic audit:** Every TX generates an audit row in the executor.

**Chain properties:**
- VaultCenter runs as validator (block producer)
- LocalVault can run as full node (block verifier)
- Block hash chain prevents log tampering
- Replay deterministic (no time.Now() in executor)

## Network Topology

### Docker Compose (Default)

```
docker-compose.yml
├── vaultcenter (:10181, :26656 P2P, :26657 RPC)
├── localvault  (:10180)
└── veil        (sleep, exec into for CLI)

Host ports: 11181 (VC), 11180 (LV), 26656 (P2P), 26657 (RPC)
```

### Production (Multi-Node)

```
┌──────────────────┐
│  VaultCenter      │
│  :10181 (API)     │
│  :26656 (P2P)     │
└────────┬──────────┘
         │ heartbeat (5m)
    ┌────┼────┐
    │    │    │
┌───▼┐ ┌▼──┐ ┌▼───┐
│LV-A│ │LV-B│ │LV-C│
│cont│ │cont│ │host│
└────┘ └────┘ └────┘
```

## API Summary

### VaultCenter

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | - | Health check |
| GET | `/api/status` | - | Server status |
| POST | `/api/admin/login` | password | Admin login |
| GET | `/api/admin/check` | session | Session check |
| GET | `/api/keycenter/temp-refs` | admin | List temp refs |
| POST | `/api/keycenter/temp-refs` | admin | Create temp ref |
| POST | `/api/keycenter/promote` | admin | Promote to vault |
| POST | `/api/admin/registration-tokens` | admin | Issue registration token |
| POST | `/api/admin/change-password` | owner+trusted | Change admin password |
| GET | `/api/agents/{hash}/secrets` | agent | List agent secrets |
| GET | `/api/agents/{hash}/secrets/{name}` | agent | Get secret value |
| GET | `/api/resolve/{ref}` | - | Resolve VK ref |
| GET | `/api/vault-inventory` | - | List vaults |
| GET | `/api/vaults/{hash}/keys` | - | List vault keys |
| GET | `/api/vaults/{hash}/keys/{name}` | - | Get key value |

### LocalVault

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | - | Health check |
| GET | `/api/status` | - | Node status |
| GET | `/api/secrets` | - | List secrets (metadata) |
| GET | `/api/cipher/{ref}` | trusted | Get ciphertext |
| POST | `/api/cipher` | trusted | Store ciphertext |
| GET | `/api/install/status` | - | Setup state |

**Removed endpoints:** `/api/promote`, `/api/decrypt`, `/api/resolve`, `/api/encrypt` — LV is storage-only.

## Database

Both services use SQLCipher (encrypted SQLite, WAL mode). `VEILKEY_DB_KEY` is **required** — server refuses to start without it. Plain `sqlite3` cannot read the database.

- **VaultCenter:** agents, token_refs, audit_events, configs, secrets, admin_sessions
- **LocalVault:** secrets, configs, node_info, functions

## Agent Authentication (v0.5.0)

Each LocalVault authenticates to VaultCenter with an `agent_secret` Bearer token, issued during registration. Authentication is bidirectional:

| Direction | Mechanism |
|-----------|-----------|
| LV → VC | `Authorization: Bearer {agent_secret}` — VC verifies SHA256(token) → agent_hash |
| VC → LV | Same token — LV verifies via constant-time comparison |

Unauthenticated requests to agent APIs are rejected. `mask-map` endpoint remains open (trusted IP only) for PTY masking.

### Admin Password

- Initial setup: `POST /api/admin/setup` (requires owner password)
- Change: `POST /api/admin/change-password` (requires owner password — not admin session)
- No other method can change the admin password (DB is encrypted)
