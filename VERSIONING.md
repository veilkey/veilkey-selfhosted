# VeilKey Versioning

This repository uses a single product version for the self-hosted VeilKey surface.

The canonical current version lives in `VERSION`.

## Scope

The product version applies to the self-hosted monorepo as a whole:

- `installer`
- `services/vaultcenter`
- `services/localvault`
- `services/proxy`
- `client/cli`

Component-specific commit SHAs may still differ internally for debugging or packaging, but operator-facing release management should use one product version.

## Policy

VeilKey is currently in beta and uses `0.x.y` semantic versioning.

- `0.1.0`
  - first managed beta baseline for the unified self-hosted monorepo
- `0.1.z`
  - bug fixes, install fixes, docs fixes, operational hardening
  - no intentional workflow or packaging break unless explicitly documented
- `0.x.0` where `x > 1`
  - new product capabilities
  - install/update path changes
  - workflow, fleet, or rollout behavior changes
  - compatibility may still change while the product remains pre-`1.0.0`

## Pre-release Labels

Pre-release builds should use semver pre-release suffixes when needed:

- `0.1.0-beta.1`
- `0.1.0-beta.2`
- `0.2.0-rc.1`

Build metadata may be appended for internal traceability:

- `0.1.0+<gitsha>`

## Update Planning Guidance

When VeilKey becomes its own update control plane, the product version should be the primary release unit used by:

- installer manifests
- update plans
- rollout approvals
- fleet status views
- post-update health checks

Component-level package revisions should remain implementation detail under the product version unless an operator explicitly requests component debugging detail.
