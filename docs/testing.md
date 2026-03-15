# Testing

This document collects the primary self-hosted validation entry points.

## Docs

Use the docs-local tools first:

```bash
bash docs/tools/doctor.sh
```

Supporting commands:

```bash
bash docs/tools/validate.sh
bash docs/tools/generate.sh
bash docs/tools/check-generated.sh
bash docs/tools/report-hardcoded-values.sh
bash docs/tools/update-hardcoding-baseline.sh
```

## Repository Validate Jobs

The canonical top-level CI jobs are defined in [`cue/ci.cue`](./cue/ci.cue) and enforced by [`../.gitlab-ci.yml`](../.gitlab-ci.yml).

Primary validate jobs:

- `facts-validate`
- `installer-validate`
- `keycenter-validate`
- `localvault-validate`
- `cli-validate`
- `proxy-validate`

## Local Test Entry Points

Run the smallest relevant target first:

- `installer`
  - `cd installer`
  - `./install.sh init`
  - `./install.sh validate`
  - `./install.sh doctor`
- `services/keycenter`
  - `cd services/keycenter`
  - `go test ./cmd/... ./internal/api/... ./internal/db/...`
- `services/localvault`
  - `cd services/localvault`
  - `go test ./cmd/... ./internal/api/... ./internal/db/...`
- `client/cli`
  - `cd client/cli`
  - `go test ./...`
- `services/proxy`
  - `cd services/proxy`
  - `go test ./...`

## Generated References

For a generated summary of components, CI jobs, and identity terms, use [`generated/summary.md`](./generated/summary.md).
