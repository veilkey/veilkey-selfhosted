# VeilKey Self-Hosted

`veilkey-selfhosted` is the canonical repository for the self-hosted VeilKey product surface.

It keeps the active self-hosted surface in one repository while preserving clear boundaries between installation, runtime services, and operator-facing clients.

Current product version:

- `0.1.0`

## Repository Position

VeilKey is organized in two product domains:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - `installer`
  - `keycenter`
  - `localvault`
  - `cli`
  - `proxy`

This repository is the source of truth for the `self-hosted` domain.

## Repository Layout

- `installer/`
  - packaging, install profiles, Proxmox wrappers, health checks
- `services/`
  - runtime services
  - `keycenter/`
  - `localvault/`
  - `proxy/`
- `client/`
  - operator-facing surfaces
  - `cli/`
- `docs/`
  - repository-local architecture and operations references
- `docs/cue/`
  - canonical repository facts tracked in CUE

Primary documentation model:

- this file is the main repository hub
- product version and release policy live in `VERSION` and `VERSIONING.md`
- `docs/` is the repository-local documentation hub
- service READMEs stay intentionally thin
- canonical repository facts live in `docs/cue/`
- detailed behavior should live close to source in `cmd/`, `internal/`, `scripts/`, `tests/`, or `docs/examples/`

## Runtime Model

Use [`docs/architecture.md`](./docs/architecture.md) for the cross-component runtime shape and responsibility boundaries.

## Responsibility Boundary

This repository is intended to keep the self-hosted VeilKey surface in one place without flattening component responsibilities.

Each top-level area remains responsible for its own source, tests, and operational contracts.

## Validation and CI

Use [`docs/testing.md`](./docs/testing.md) for the canonical docs doctor flow, local test entry points, and top-level CI references.

## Local Test Entry Points

Use [`docs/testing.md`](./docs/testing.md) for the repository-level testing entry points.

## Service Docs

Use service README files only for service-local orientation:

- [`services/keycenter/README.md`](./services/keycenter/README.md)
- [`services/localvault/README.md`](./services/localvault/README.md)

Use code directories directly for implementation detail:

- `services/keycenter/internal/`
- `services/localvault/internal/`
- `installer/scripts/`
- `client/cli/`

Use CUE facts for repository-wide canonical data:

- `docs/cue/repository.cue`

Use repository-local docs for cross-component self-hosted references:

- [`docs/README.md`](./docs/README.md)
- [`docs/architecture.md`](./docs/architecture.md)
- [`docs/testing.md`](./docs/testing.md)
- [`docs/runtime-contracts.md`](./docs/runtime-contracts.md)
