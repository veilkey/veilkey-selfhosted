# Security Model

## Design Principles

1. **No plaintext at rest.** Secrets are encrypted the moment they enter the system. Config files, databases, and logs contain only `VK:` references or ciphertext.

2. **Password never in process environment.** KEK passwords are read from files (`VEILKEY_PASSWORD_FILE`), not env vars. The `VEILKEY_PASSWORD` env var is explicitly rejected to prevent exposure via `ps`, `/proc`, or crash dumps.

3. **Least privilege by default.** LocalVault cannot decrypt secrets on its own — it delegates plaintext operations to VaultCenter. Write operations require trusted IP verification.

4. **Key hierarchy with rotation.** KEK → DEK → ciphertext. DEK rotation is managed by VaultCenter and executed by each LocalVault node independently.

5. **Defense in depth.** The optional Proxy layer intercepts egress traffic and blocks leaked secrets even if application code mishandles them.

## Threat Model

### What VeilKey protects against

| Threat | Mitigation |
|--------|-----------|
| Secrets in config files / repos | `scan` detects, `filter` replaces with VK tokens |
| Secrets in CI/CD logs | `wrap` masks output in real-time |
| Secrets in process environment | `exec` resolves at launch, never writes to disk |
| Compromised node stealing secrets | LocalVault has only ciphertext; DEK is encrypted with KEK |
| Lateral movement after node compromise | Each node has its own DEK; compromising one doesn't expose others |
| Brute-force KEK guessing | scrypt-based key derivation with random salt |
| Stale keys after rotation | VaultCenter tracks `key_version` per node, enforces rotation |
| Egress of secrets via HTTP | Proxy detects and blocks plaintext in outbound traffic |

### What VeilKey does NOT protect against

| Threat | Reason |
|--------|--------|
| Compromised VaultCenter | VaultCenter holds the master key hierarchy — protect it |
| Memory dump of running process | Decrypted DEK exists in memory while server is unlocked |
| Operator with KEK password | By design — the operator is trusted |
| Physical access to the machine | Standard physical security applies |

## Encryption Details

### Key Derivation

```
password (operator-provided, min 8 chars)
    + salt (32 bytes, crypto/rand)
    → scrypt(N=32768, r=8, p=1)
    → KEK (32 bytes)
```

### Data Encryption

- **Algorithm:** AES-256-GCM (via Go `crypto/aes` + `cipher.NewGCM`)
- **DEK:** 32 bytes from `crypto/rand`
- **Nonce:** 12 bytes, unique per encryption operation
- **Storage:** `{encrypted_dek, dek_nonce}` in node_info table; `{ciphertext, nonce}` per secret

### Salt Storage

The salt file is stored alongside the database:
- `{data_dir}/salt` — 32 bytes, mode `0600`
- Required for KEK derivation on every unlock
- If lost, all data is unrecoverable

## Network Security

### Default (no TLS)

Services bind to `127.0.0.1` or a specified address. Without TLS, traffic is plaintext on the wire.

**Recommendation:** Enable TLS for any deployment where VaultCenter and LocalVault communicate over a network.

### TLS Configuration

```bash
export VEILKEY_TLS_CERT=/path/to/cert.pem
export VEILKEY_TLS_KEY=/path/to/key.pem
```

### Trusted IPs

Write operations (store config, delete, etc.) are restricted to trusted IPs:

```bash
export VEILKEY_TRUSTED_IPS="127.0.0.1/32,10.0.0.0/8"
```

If `VEILKEY_TRUSTED_IPS` is not set, all IPs are allowed (with a warning).

## Proxy Enforcement

The Proxy component provides an additional security boundary:

1. **Shell guard hook** — blocks `curl`, `wget`, direct `.env` access, and `git credential` commands in veilroot shells
2. **Egress proxy** — inspects outbound HTTP traffic for plaintext secrets
3. **Access logging** — JSONL audit trail of all proxied requests

## Password File Security

```bash
# Correct: password in file with restricted permissions
echo -n 'my-password' > /etc/veilkey/vaultcenter.password
chmod 600 /etc/veilkey/vaultcenter.password

# WRONG: password in environment (rejected by VeilKey)
export VEILKEY_PASSWORD='my-password'  # will cause fatal error
```

## Audit

VaultCenter logs:
- All unlock/lock events
- Agent registration and heartbeat
- Config changes
- Admin session activity
- Rotation and rebind operations

Access the audit log via the admin UI at `http://<vaultcenter>:10181` or the admin API (`/api/admin/audit/recent`).
