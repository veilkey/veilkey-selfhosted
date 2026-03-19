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

## Blockchain Audit

- **Chain:** CometBFT (single validator + full nodes)
- **TX types:** SaveTokenRef, UpsertAgent, RegisterChild, SaveBinding, SetConfig, RecordAuditEvent
- **Auto-audit:** Every TX automatically generates audit row in executor
- **Key material exclusion:** DEK/nonce never on chain (PR #132)
- **Determinism:** No `time.Now()` in executor — uses TX timestamp for replay safety
