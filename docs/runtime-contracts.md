# Runtime Contracts

This document describes the repository-level contracts that are treated as canonical for self-hosted documentation.

## Contract Sources

The primary machine-checked sources live under [`cue/`](./cue/):

- `repository.cue`
  - canonical repository identity and top-level paths
- `components.cue`
  - component catalog and service-local CI files
- `docs.cue`
  - docs entrypoints and required strings
- `ci.cue`
  - top-level validate and e2e job names
- `identity.cue`
  - primary and compatibility identity terms
- `runtime.cue`
  - runtime component groups
- `testing.cue`
  - local validation command references

## Validation Contract

The active validation flow is:

```bash
bash docs/tools/validate.sh
```

This command checks:

- CUE evaluation under `docs/cue/`
- required repository paths
- required top-level CI jobs
- required service-local CI jobs
- root README contract strings
- service README required identity terms and root links
- environment-specific hardcoded values against the committed baseline in [`evidence/hardcoding-env-specific-baseline.txt`](./evidence/hardcoding-env-specific-baseline.txt)

## Generation Contract

The active generation flow is:

```bash
bash docs/tools/generate.sh
```

Generated outputs must remain current:

```bash
bash docs/tools/check-generated.sh
```

For convenience, both checks are bundled in:

```bash
bash docs/tools/doctor.sh
```

For a wider hardcoding audit that stays non-blocking unless promoted into `check-hardcoded-values.sh`, use:

```bash
bash docs/tools/report-hardcoded-values.sh
```
