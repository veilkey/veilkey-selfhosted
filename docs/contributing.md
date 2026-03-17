# Contributing

## Development Setup

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
```

Each component is an independent Go module. To work on a specific service:

```bash
cd services/keycenter
go test ./...
go build -o veilkey-keycenter ./cmd
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
| keycenter | `services/keycenter/**` changed | lint + test + build |
| localvault | `services/localvault/**` changed | lint + test + build |
| proxy | `services/proxy/**` changed | lint + test + build |
| cli | `client/cli/**` changed | lint + test + build |
| installer | `installer/**` changed | manifest validate + shell tests |
| shared | `shared/**` changed | shell hook tests |
| pr-gate | always | verifies all required jobs passed |

## Code Style

- Go: follow `gofmt` and `golangci-lint`
- Shell: `set -euo pipefail`, quote variables, use `shellcheck`
- No plaintext secrets in code, tests, or comments
- Password handling: always via file, never env var

## Testing

### Go tests

```bash
cd services/keycenter && go test ./... -race -count=1
cd services/localvault && go test ./... -race -count=1
cd services/proxy && go test ./... -race -count=1
cd client/cli && go test ./... -race -count=1
```

### Shell tests

```bash
cd installer && bash tests/test_install_profile_command.sh
cd shared && bash tests/test_veilroot_shell_hook.sh
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
