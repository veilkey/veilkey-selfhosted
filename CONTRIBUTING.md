# Contributing

Contributions welcome. Changes should prove the exact path they touch.

## Ground Rules

1. Behavior changes must include focused regression tests.
2. User-facing changes must update README or docs in the same change.
3. Do not introduce plaintext-secret examples.
4. Do not hardcode default values in Go/Rust source. Use environment variables. Defaults in `.env.example` only.

A pre-commit hook enforces rule 4:

```bash
cp scripts/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Where To Start

- Repository overview: [`README.md`](./README.md)
- Architecture: [`docs/architecture.md`](./docs/architecture.md)
- Installation: [`docs/installation.md`](./docs/installation.md)

## Development

```bash
# Go services
cd services/vaultcenter && go build ./...
cd services/localvault && go build ./...

# Rust CLI
cd services/veilkey-cli && cargo build --release
cd services/veil-cli && cargo build --release

# Docker
docker compose up --build -d
```

## Validation

```bash
# Pre-commit
bash scripts/pre-commit

# Go build
cd services/vaultcenter && go build ./...
cd services/localvault && go build ./...
```

## Pull Request Standard

- What changed
- Why it changed
- How it was verified
- What operator behavior changed, if any
