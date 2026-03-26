# Security Audit — Cryptographic & Secret Security

## Purpose

Audits VeilKey code for cryptographic correctness and secret handling safety. Critical for a project whose entire purpose is protecting secrets from exposure.

## Audit Areas

### 1. Cryptographic Correctness

- **RNG**: Only `crypto/rand` (Go) or `rand::rngs::OsRng` (Rust). Never `math/rand` or `rand::thread_rng()` for key material.
- **AES-256-GCM**: Verify 256-bit keys (32 bytes), unique nonces per encryption, no nonce reuse.
- **Key derivation**: Proper KDF usage (Argon2, scrypt, or PBKDF2 with sufficient iterations).
- **No ECB mode**: Block cipher modes must be authenticated (GCM, CCM).

### 2. Secret Handling

- **No plaintext in logs**: Secrets must never appear in `log.*`, `fmt.Print*`, or `println!` output.
- **No plaintext in errors**: Error messages must not include secret values.
- **No hardcoded secrets**: No API keys, passwords, or tokens in source code.
- **Memory cleanup**: Sensitive bytes should be zeroed after use where possible.
- **No secrets in test fixtures**: Use generated test data, not real secrets.

### 3. Split Storage Integrity

- VaultCenter must never store encrypted data (only keys).
- LocalVault must never store encryption keys (only encrypted data).
- Cross-vault access must be rejected — verify agent_secret isolation.

### 4. Database Security

- SQLCipher must be used for all database access (not plain sqlite3).
- KEK (Key Encryption Key) must exist only in memory — no disk persistence.
- Database encryption key must be derived from master password, not stored.

### 5. Network Security

- TLS for all inter-service communication.
- Bearer token auth for vault-to-vault communication.
- Cookie-based auth with proper secure flags for admin UI.
- No CORS wildcards in production.

## Audit Checklist

```
[ ] No math/rand for security operations
[ ] AES-256-GCM nonces are unique
[ ] No secrets in log output
[ ] No secrets in error messages
[ ] No hardcoded secrets in source
[ ] SQLCipher used (not plain sqlite3)
[ ] KEK is memory-only
[ ] Split storage model preserved
[ ] TLS enforced for inter-service comms
[ ] Agent secret isolation verified
```

## Reporting

When issues are found, report:
- **File and line number**
- **Severity**: Critical / High / Medium / Low
- **Description**: What the issue is
- **Recommendation**: How to fix it
