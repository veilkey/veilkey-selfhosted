# VK:STATIC — Name-Based Secret References

## Overview

`VK:STATIC:{name}` is a human-readable reference that resolves secrets by **name** instead of hash.

| Type | Example | Lookup |
|------|---------|--------|
| `VK:LOCAL:3c3d53ea` | Hash-based | Fast, but unreadable |
| `VK:STATIC:OWNER_PASSWORD` | Name-based | Readable, fixed reference |

Both resolve to the same encrypted value. `VK:STATIC` is an alias — no separate storage.

## Usage

### Create a secret

```bash
# Value as argument
veilkey-cli secret add OWNER_PASSWORD "mypassword"

# Or prompt (value hidden)
veilkey-cli secret add OWNER_PASSWORD
Secret value: ******
```

### Reference in config files

```env
# .env
VEILKEY_PASSWORD=VK:STATIC:OWNER_PASSWORD
DB_PASSWORD=VK:STATIC:DB_PASSWORD
SMTP_PASSWORD=VK:STATIC:SMTP_PASSWORD
```

### Resolve

```bash
veilkey-cli resolve VK:STATIC:OWNER_PASSWORD
# → mypassword
```

### Change value

```bash
veilkey-cli secret add OWNER_PASSWORD "newpassword"
# Same name → overwrites value
# All VK:STATIC:OWNER_PASSWORD references auto-resolve to new value
```

## Comparison with VK:LOCAL

```
VK:LOCAL:3c3d53ea     ← hash changes if you recreate the secret
VK:STATIC:OWNER_PASSWORD  ← always the same, regardless of value changes
```

| | VK:LOCAL | VK:STATIC |
|---|---|---|
| Reference format | `VK:LOCAL:{8-char hash}` | `VK:STATIC:{name}` |
| Human-readable | No | Yes |
| Stable across value changes | Hash stays same on update | Name stays same always |
| Use in .env | Works but opaque | Works and self-documenting |
| Resolution speed | Direct hash lookup | Name → hash → value (one extra step) |
| PTY masking | Masked in output | Masked in output |

## Best Practices

1. **Use VK:STATIC for config files** — `.env`, `config.toml`, docker-compose environment variables
2. **Use VK:LOCAL for programmatic access** — API calls, automated scripts where readability doesn't matter
3. **Name conventions** — use UPPER_SNAKE_CASE matching the env var name:
   ```
   DB_PASSWORD=VK:STATIC:DB_PASSWORD
   SMTP_PASSWORD=VK:STATIC:SMTP_PASSWORD
   ```
4. **Don't put values in CLI arguments** — use the prompt mode:
   ```bash
   veilkey-cli secret add SENSITIVE_KEY
   Secret value: ******
   ```

## How It Works

```
VK:STATIC:OWNER_PASSWORD
    │
    ▼ (VaultCenter)
    GetRefBySecretName("OWNER_PASSWORD")
    │
    ▼ (found: VK:LOCAL:3c3d53ea, agent_hash: a0a761c6)
    resolveTrackedRef()
    │
    ▼ (decrypt with agentDEK)
    → plaintext value
```

No new database tables or storage — `VK:STATIC` is a lookup alias over existing `VK:LOCAL` refs.
