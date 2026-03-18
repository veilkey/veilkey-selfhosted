# Architecture

## Overview

VeilKey is a distributed secret lifecycle system. It replaces plaintext secrets with encrypted, scoped references and manages the full lifecycle — creation, rotation, rebind, and revocation — across a hierarchy of nodes.

The system follows a hub-and-spoke model:

- **VaultCenter** (hub): single control plane per deployment
- **LocalVault** (spoke): one per node, reports to VaultCenter
- **CLI**: operator tool, talks to LocalVault or VaultCenter
- **Proxy**: optional egress enforcement layer

## Components

### VaultCenter

The central control plane. Runs as a single server per deployment.

**Responsibilities:**
- Node identity management (`vault_node_uuid`, `vault_hash`, `key_version`)
- Data Encryption Key (DEK) lifecycle — generation, rotation, rebind
- Secret registry — canonical `VK:{SCOPE}:{REF}` references
- Config storage — encrypted key-value pairs
- Agent inventory — tracks all connected LocalVault nodes
- Policy enforcement — rotation schedules, blocked states, approval flows
- Admin web UI — operations console for monitoring and management

**Does NOT:**
- Store plaintext secrets (only encrypted ciphertext)
- Run on every node (single instance per deployment)

**Port:** `:10181` (default when co-located with LocalVault)

### LocalVault

A node-local agent that runs on every machine that needs secret access.

**Responsibilities:**
- Local ciphertext storage
- Heartbeat to VaultCenter (5-minute interval)
- Execute lifecycle actions under VaultCenter policy (rekey, rebind)
- Report node identity and managed paths
- Serve setup wizard for fresh installs

**Does NOT:**
- Handle plaintext encryption/decryption directly (delegates to VaultCenter)
- Make policy decisions (follows VaultCenter instructions)

**Port:** `:10180`

### CLI (`veilkey-cli`)

The operator-facing command-line tool.

**Commands:**

| Command | Description | Requires API |
|---------|-------------|:---:|
| `scan [file\|-]` | Detect secrets using 222+ patterns | No |
| `filter [file\|-]` | Replace detected secrets with VK tokens | Yes |
| `wrap <cmd>` | Execute command with output masking | Yes |
| `wrap-pty [cmd]` | Interactive PTY with masking | Yes |
| `exec <cmd>` | Resolve VK tokens before execution | Yes |
| `resolve <VK:hash>` | Decrypt a single token | Yes |
| `function <sub>` | Manage function wrappers | Yes |
| `list` | List detected secrets | No |
| `status` | Show connection status | No |

### Proxy

Optional outbound enforcement layer using eBPF (Linux only).

**Responsibilities:**
- Monitor egress traffic from wrapped workloads
- Detect plaintext secrets in outbound requests
- Issue `VK:TEMP:*` replacement tokens or block requests
- Profile-based enforcement (default, codex, claude, opencode)
- Access logging in JSONL format

**Profiles and ports:**

| Profile | Port |
|---------|------|
| default | 18080 |
| codex | 18081 |
| opencode | 18083 |
| claude | 18084 |

**Note:** Proxy requires Linux with eBPF support. Not available on macOS.

## Encryption Model

VeilKey uses a hierarchical key management (HKM) model:

```
KEK (Key Encryption Key)
 │  derived from operator password + salt via scrypt
 │
 └─► DEK (Data Encryption Key)
      │  encrypted at rest with KEK
      │  rotated via VaultCenter policy
      │
      └─► Secrets / Configs
           encrypted with DEK
           referenced as VK:{SCOPE}:{REF}
```

### Key Hierarchy

1. **KEK**: Derived from the operator's password and a random salt. Never stored — derived on unlock.
2. **DEK**: Random 256-bit key. Encrypted with KEK and stored in the database. One per node, versioned.
3. **Ciphertext**: Each secret is encrypted with the current DEK. Stored with its nonce.

### Scoped References

Secrets are referenced using `VK:{SCOPE}:{REF}` tokens:

| Scope | Meaning |
|-------|---------|
| `VK:LOCAL:xxxx` | Encrypted by a specific LocalVault node |
| `VK:TEMP:xxxx` | Temporary token issued by Proxy during interception |
| `VK:EXTERNAL:xxxx` | Reference to an external secret provider |

## Data Flow

### Secret Creation (via CLI)

```
1. Operator runs: echo "TOKEN=sk-abc123" | veilkey-cli filter -
2. CLI detects "sk-abc123" via pattern matching
3. CLI sends plaintext to VaultCenter POST /api/encrypt
4. VaultCenter encrypts with DEK, stores ciphertext
5. CLI outputs: TOKEN=VK:LOCAL:a1b2c3d4
```

### Secret Resolution (via CLI)

```
1. Application config contains: API_KEY=VK:LOCAL:a1b2c3d4
2. Operator runs: veilkey-cli exec ./my-app
3. CLI finds VK: tokens in environment/args
4. CLI resolves via GET /api/resolve/VK:LOCAL:a1b2c3d4
5. Decrypted value injected into subprocess environment
6. my-app runs with real secret, never written to disk
```

### Node Registration

```
1. LocalVault starts, no salt file → enters setup wizard mode
2. Operator provides password + VaultCenter URL via wizard or API
3. LocalVault generates salt, derives KEK, creates DEK
4. LocalVault registers with VaultCenter (node_id, vault_hash)
5. VaultCenter records agent, begins accepting heartbeats
6. LocalVault sends heartbeat every 5 minutes
```

### Key Rotation

```
1. VaultCenter schedules rotation for a node
2. On next heartbeat, LocalVault receives rotation instruction
3. LocalVault generates new DEK (version N+1)
4. LocalVault re-encrypts all secrets with new DEK
5. LocalVault reports completion to VaultCenter
6. VaultCenter updates agent record with new key_version
```

## Network Topology

### Minimal (Single Node)

```
┌─────────────────────────┐
│     Single Machine      │
│                         │
│  VaultCenter (:10181)     │
│       ▲                 │
│       │ heartbeat       │
│  LocalVault (:10180)    │
│       ▲                 │
│       │                 │
│  CLI / Applications     │
└─────────────────────────┘
```

### Production (Multi-Node)

```
┌──────────────────┐
│   Control Plane   │
│  VaultCenter :10181 │
│  LocalVault :10180│
└────────┬──────────┘
         │ heartbeat (every 5m)
    ┌────┼────┐
    │    │    │
┌───▼┐ ┌▼──┐ ┌▼───┐
│LV-A│ │LV-B│ │LV-C│
│node│ │node│ │host│
└────┘ └────┘ └────┘
```

### With Proxy Enforcement

```
┌──────────────────────────────────┐
│         Proxmox Host              │
│                                   │
│  ┌─────────────────────────────┐ │
│  │    LXC (All-in-One)         │ │
│  │  VaultCenter :10181           │ │
│  │  LocalVault :10180          │ │
│  └─────────────────────────────┘ │
│                                   │
│  Proxy (host boundary)            │
│    egress-proxy@default :18080    │
│    egress-proxy@codex   :18081    │
│    egress-proxy@claude  :18084    │
│                                   │
│  veilroot shell (sandboxed user)  │
│    curl/wget blocked              │
│    direct .env access blocked     │
└──────────────────────────────────┘
```

## API Summary

### VaultCenter API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| GET | `/api/status` | Server status, mode, counts |
| GET | `/api/agents` | List registered LocalVault nodes |
| GET | `/api/configs` | List configs |
| POST | `/api/configs` | Store config (trusted IP only) |
| GET | `/api/configs/{key}` | Get config |
| DELETE | `/api/configs/{key}` | Delete config (trusted IP only) |
| POST | `/api/unlock` | Unlock with password |
| GET | `/api/admin/*` | Admin session endpoints |

### LocalVault API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (`ok` or `setup`) |
| GET | `/api/status` | Node status and identity |
| GET | `/api/secrets` | List secrets (metadata only) |
| GET | `/api/install/status` | Setup wizard state |
| POST | `/api/install/init` | Initialize node (wizard) |
| PATCH | `/api/install/vaultcenter-url` | Set VaultCenter URL |

## Database

Both VaultCenter and LocalVault use SQLCipher (encrypted SQLite):

- **VaultCenter**: `VEILKEY_DB_PATH` (default: `/opt/veilkey/vaultcenter/data/veilkey.db`)
- **LocalVault**: `VEILKEY_DB_PATH` (default: `/opt/veilkey/localvault/data/veilkey.db`)

Each database stores:
- Node identity (node_id, version, parent URL)
- Encrypted DEK + nonce
- Secrets (name, encrypted value, nonce, metadata)
- Configs (key-value pairs)
- Audit log
