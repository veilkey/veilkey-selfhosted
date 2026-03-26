# Go Security — Cryptographic Safety for Go Code

## Purpose

Ensures all Go code in VeilKey follows cryptographic best practices and secure coding standards.

## Rules

### Randomness

- **MUST** use `crypto/rand` for all security-sensitive random values (keys, nonces, tokens)
- **NEVER** use `math/rand` for any security-related purpose
- Key generation must use `crypto/rand.Read()` or equivalent

### Encryption

- **AES-256-GCM** is the required encryption mode
- Keys must be exactly 32 bytes (256 bits)
- Nonces must be unique per encryption — use `crypto/rand` to generate
- Never reuse a nonce with the same key
- Use `cipher.NewGCM()` from `crypto/aes`

### Secret Handling

- Never log secrets with `log.*`, `fmt.Print*`, or `slog.*`
- Never include secrets in error messages
- Never hardcode secrets in source code
- Zero out sensitive byte slices after use: `for i := range key { key[i] = 0 }`
- Use `defer` to ensure cleanup runs

### Database

- All database access through SQLCipher — never plain `sqlite3`
- Database encryption key derived from master password (KEK)
- KEK exists in memory only — never written to disk

### Input Validation

- Validate all API inputs before processing
- Reject unexpected fields in JSON requests
- Sanitize error messages — no internal paths or secret values

## Review Checklist

```
[ ] crypto/rand used (not math/rand)
[ ] AES-256-GCM with unique nonces
[ ] No secrets in logs or errors
[ ] No hardcoded secrets
[ ] SQLCipher for all DB access
[ ] Sensitive data zeroed after use
[ ] API inputs validated
```
