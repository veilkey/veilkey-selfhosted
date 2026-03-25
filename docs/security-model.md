# Security Model

## Design Principles

1. **No plaintext at rest.** Secrets are encrypted the moment they enter the system. Databases and logs contain only `VK:` references or ciphertext.

2. **Split custody.** VaultCenter holds agentDEK (encryption keys). LocalVault holds ciphertext. Neither alone can access secrets.

3. **AI-safe by design.** PTY bidirectional masking ensures AI coding tools (Claude Code, Cursor, etc.) never see plaintext — only VK refs.

4. **Immutable audit.** All key operations are recorded on CometBFT blockchain. Tampering breaks the hash chain.

5. **LocalVault is storage-only.** No decrypt, no promote, no resolve endpoints. LV cannot access plaintext even if compromised.

## Threat Model

### What VeilKey protects against

| Threat | Mitigation |
|--------|-----------|
| AI reading secrets | PTY masking: output filtered, only VK refs visible |
| Secrets in config files | `scan` detects, `filter` replaces with VK tokens |
| Secrets in terminal output | `wrap-pty` masks plaintext in real-time |
| LocalVault compromised | Ciphertext only — no agentDEK, cannot decrypt |
| VaultCenter DB stolen | agentDEK encrypted with KEK — needs master password |
| Audit log tampering | CometBFT blockchain — hash chain breaks on modification |
| Lateral movement | Per-vault agentDEK — one vault compromised, others safe |
| Stale keys | VaultCenter tracks key_version, enforces rotation |
| Unauthorized agent | Registration token required for first heartbeat |
| Cross-vault secret access | Agent auth (Bearer token) — each vault can only query its own secrets |
| DB direct manipulation | SQLCipher encryption — `VEILKEY_DB_KEY` required, plain sqlite3 blocked |
| Password file on disk | Removed — KEK exists only in memory, entered via `POST /api/unlock` |
| Admin password hijack | Change requires owner password (KEK verification), not admin session |

### What VeilKey does NOT protect against

| Threat | Reason |
|--------|--------|
| Both VC + LV compromised + KEK extracted | By design — requires master password |
| Memory dump of running VaultCenter | DEK exists in memory while unlocked |
| Operator with master password | Trusted by design |
| Physical access | Standard physical security applies |

## Encryption Details

### Key Hierarchy

```
Master Password (operator memory)
  + salt (32 bytes, crypto/rand)
  → scrypt(N=32768, r=8, p=1)
  → KEK (32 bytes) — never stored, derived on unlock

KEK → agentDEK (per-vault, 32 bytes)
      stored encrypted in VC DB

agentDEK → ciphertext (per-secret)
           stored in LV DB
           AES-256-GCM, 12-byte nonce
```

### Security Properties

| Component | Holds | Cannot Access |
|-----------|-------|---------------|
| VaultCenter | KEK (memory), agentDEK (encrypted) | ciphertext (in LV) |
| LocalVault | ciphertext | agentDEK, KEK, plaintext |
| veil CLI | VK refs | plaintext (PTY-filtered) |
| Blockchain | TX metadata | key material (never on chain) |

## PTY Masking

```
Environment: DB_PASSWORD=VK:LOCAL:ea2bfd16
                     ↓ resolve
Child process sees:  DB_PASSWORD=actual-password
                     ↓ echo $DB_PASSWORD
PTY output:          actual-password
                     ↓ mask_map filter
Screen/AI sees:      VK:LOCAL:ea2bfd16
```

- Bidirectional: env → resolve (inbound), output → mask (outbound)
- `cat .env` also masked if file contains resolved values
- mask_map sorted by length (longest match first)

## Database Encryption

All databases (VaultCenter + LocalVault) are encrypted with SQLCipher. The encryption key is derived from the master password: `DB_KEY = SHA256(KEK)`. Database can only be opened after unlock — salt file alone cannot open the DB.

```
master password (entered during unlock)
  → KEK = DeriveKEK(password, salt)
  → DB_KEY = SHA256(KEK)
  → SQLCipher _pragma_key
  → plain sqlite3 cannot read DB
```

## Vault Isolation (Agent Auth)

Each LocalVault authenticates to VaultCenter with an `agent_secret` (Bearer token), issued during registration. Authentication is bidirectional:

```
LocalVault → VaultCenter:  Authorization: Bearer {agent_secret}
                           VaultCenter verifies SHA256(token) → agent_hash
                           URL {agent} must match authenticated agent

VaultCenter → LocalVault:  Authorization: Bearer {agent_secret}
                           LocalVault verifies via constant-time comparison
```

`mask-map` endpoint requires admin session authentication. The veil CLI prompts for admin password on startup, logs in, and uses the session cookie for mask-map requests.

`resolve` command requires both interactive terminal (TTY check) and admin password. Non-TTY execution (pipe) is blocked to prevent AI tools from reading plaintext.

### VE (Config) Display

VE config values (URLs, vault names) are displayed with green color in PTY output without text replacement. Values remain functional — only the color changes.

## Network Security

### TLS
- Self-signed certificates auto-generated on first run
- `VEILKEY_TLS_INSECURE=1` for inter-service communication (docker internal)
- Production: use proper CA certificates

### Trusted IPs
```bash
VEILKEY_TRUSTED_IPS="10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
```
Write operations (store secret, delete, etc.) restricted to trusted IPs.

### Admin Authentication
- bcrypt password hash + HttpOnly session cookie
- Rate limiting: 10 failed attempts → 15-minute lockout
- Configurable TTL: `VEILKEY_ADMIN_SESSION_TTL` (default: 2h)
- Password change: `POST /api/admin/change-password` — requires owner password (KEK), not admin session

## AI Agent Security Boundary

When running AI coding tools (Claude Code, Cursor, etc.) inside a `veil` session on a machine with a LocalVault:

### Allowed

| Action | Example |
|--------|---------|
| LocalVault API calls | `curl -sk https://localhost:10180/health` |
| Service management | `docker compose restart localvault` |
| Log viewing | `tail -f .localvault/localvault.log` |
| VK ref usage in code | `export DB_PASSWORD=VK:LOCAL:xxx` |
| Secret status check | `veilkey status` |

### Forbidden

| Action | Reason |
|--------|--------|
| Read `.env` files directly | Contains infra config + VaultCenter URL |
| Run `veilkey resolve` | Decrypts secret to plaintext |
| Read `/proc/*/environ` | Exposes resolved env vars |
| Access `~/.bash_history` | May contain unmasked commands |
| Read session log plaintext | `$TMPDIR/veilkey-cli/session.log` |

### Principle

> AI agents should interact with secrets **only through VK refs**. Any operation that would reveal plaintext must go through the VaultCenter admin UI or a human operator — never through the AI agent.

### CLAUDE.md Template

A template for `.claude/CLAUDE.md` is provided at [`examples/CLAUDE.md`](../examples/CLAUDE.md). Add this to any project where Claude Code runs alongside VeilKey to enforce the security boundary.

## Blockchain Audit

- **Chain:** CometBFT (single validator + full nodes)
- **TX types:** SaveTokenRef, UpsertAgent, RegisterChild, SaveBinding, SetConfig, RecordAuditEvent
- **Auto-audit:** Every TX automatically generates audit row in executor
- **Key material exclusion:** DEK/nonce never on chain (PR #132)
- **Determinism:** No `time.Now()` in executor — uses TX timestamp for replay safety
