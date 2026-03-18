# Contributing

## Development Setup

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
```

Go services use SQLCipher (`CGO_ENABLED=1` required). Rust CLIs use Cargo.

```bash
# Go services (requires gcc for CGO/SQLCipher)
cd services/vaultcenter
CGO_ENABLED=1 go build -o veilkey-vaultcenter ./cmd

# Rust CLIs
cd services/veil-cli && cargo build --release
cd services/veilkey-cli && cargo build --release
```

## Branch Strategy

- `main` — protected, requires PR + CI pass
- `feature/*` — new features
- `fix/*` — bug fixes
- `setup/*` — infrastructure and tooling
- `docs/*` — documentation

## Pull Request Process

1. Create a branch from `main`
2. Make your changes
3. Ensure tests pass locally: `go test ./... -race`
4. Push and open a PR against `main`
5. CI runs automatically (only changed components are tested)
6. `pr-gate` must pass before merge

### CI Jobs

| Job | Trigger | What it does |
|-----|---------|-------------|
| vaultcenter | `services/vaultcenter/**` changed | lint (golangci-lint) + build (CGO/SQLCipher) |
| localvault | `services/localvault/**` changed | lint (golangci-lint) + build (CGO/SQLCipher) |
| veil-cli | `services/veil-cli/**` changed | clippy + test + release build |
| veilkey-cli | `services/veilkey-cli/**` changed | clippy + release build |
| pr-gate | always | verifies all required jobs passed |

## Code Style

- Go: follow `gofmt` and `golangci-lint`
- Shell: `set -euo pipefail`, quote variables, use `shellcheck`
- No plaintext secrets in code, tests, or comments
- Password handling: always via file, never env var

## Testing

### Rust tests (veil-cli)

```bash
cd services/veil-cli && cargo test
```

### Shell tests

```bash
cd services/vaultcenter && bash tests/test_mr_guard.sh
```

## Component Ownership

Each top-level directory is responsible for its own:
- Source code
- Tests
- README and documentation
- CI configuration (via root workflow)

Cross-component changes should be reviewed carefully to avoid breaking contracts.

## License

By contributing, you agree that your contributions will be licensed under AGPL-3.0.
