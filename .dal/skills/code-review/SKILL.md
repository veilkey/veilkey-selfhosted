# Code Review — Quality Standards for VeilKey

## Purpose

Defines code review standards for all VeilKey code (Go and Rust).

## Review Criteria

### Error Handling

**Go:**
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Never ignore errors — `_ = someFunc()` is not acceptable
- Return errors to the caller; only log at the top-level handler
- Use custom error types for domain-specific errors

**Rust:**
- Use `Result<T, E>` — no `unwrap()` in production code
- `?` operator for error propagation
- Custom error types with `thiserror` or manual impl
- `unwrap()` and `expect()` only in tests

### Naming

**Go:**
- Exported: `PascalCase` — `EncryptSecret`, `VaultConfig`
- Unexported: `camelCase` — `encryptSecret`, `vaultConfig`
- Interfaces: verb-based — `Encrypter`, `Resolver`
- Acronyms all caps: `HTTPClient`, `APIKey`, `KEK`

**Rust:**
- Types: `PascalCase` — `VeilConfig`, `SecretPattern`
- Functions/variables: `snake_case` — `encrypt_secret`, `vault_config`
- Constants: `SCREAMING_SNAKE_CASE` — `MAX_PATTERN_COUNT`

### Testing

- All new functions must have tests
- Test both success and error cases
- Use table-driven tests in Go
- No real secrets in test data — use generated values
- Test edge cases: empty input, max length, invalid UTF-8

### Code Structure

- Functions should do one thing
- Keep functions under 50 lines where possible
- No deep nesting (max 3 levels)
- Extract complex conditions into named booleans or functions

### Security-Specific

- See `skills/go-security` for crypto rules
- See `skills/security-audit` for audit checklist
- Any change to encryption, auth, or key handling requires extra scrutiny

## Review Checklist

```
[ ] Error handling follows conventions
[ ] Naming follows Go/Rust standards
[ ] New code has tests
[ ] No unwrap() in Rust production code
[ ] Functions are focused and reasonable length
[ ] No security regressions
```
