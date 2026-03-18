# Contributing

This repository is the self-hosted VeilKey product surface.

Contributions are welcome, but changes should prove the exact path they touch.

## Ground Rules

1. Runtime, install, deploy, CLI, and API behavior changes must include focused regression tests.
2. User-facing or operator-facing behavior changes must update README or docs in the same change.
3. Installer and deploy changes should verify one real operator path whenever practical.
4. Do not introduce plaintext-secret examples where an existing masked or file-based path already exists.
5. Do not hardcode default values (ports, file paths, URLs) in Go source code. All runtime values must come from environment variables. If a value is missing, the process must exit with a clear error — no silent fallbacks. Default values belong in `.env.example` only.

A pre-commit hook enforces rule 5. To install it:

```bash
cp scripts/pre-commit .git/hooks/pre-commit
chmod +x .git/hooks/pre-commit
```

## Where To Start

- repository overview: [`README.md`](./README.md)
- installer operator path: [`installer/INSTALL.md`](./installer/INSTALL.md)
- installer-specific guard rules: [`installer/CONTRIBUTING.md`](./installer/CONTRIBUTING.md)

## Collaboration Surface

Use GitHub as the collaboration surface for this repository.

- issues
  - describe the operator-visible problem first
- pull requests
  - describe what changed, why it changed, and how it was verified
- branch and review discussion
  - prefer GitHub-first wording in docs and change summaries

Do not assume a GitLab issue or MR flow for current contribution guidance unless a document is explicitly about legacy CI or internal artifact plumbing.

## Common Contribution Areas

- installation and Proxmox wrapper improvements
- VaultCenter or LocalVault runtime fixes
- CLI UX and operator workflow improvements
- docs that make the self-hosted model easier to understand

## Minimum Validation

Use the smallest relevant validation first.

- installer

```bash
cd installer
./install.sh validate
```

- Go services or CLI

```bash
go test ./...
```

If your change is narrower than the full suite, add a focused regression test for that path.

## Pull Request Standard

A good contribution makes all of these easy to answer:

- what changed
- why it changed
- how it was verified
- what operator behavior changed, if any

If the answer to one of those is missing, the change is not ready yet.
