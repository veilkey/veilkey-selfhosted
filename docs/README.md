# VeilKey Self-Hosted Docs

This directory is the in-repository documentation hub for `veilkey-selfhosted`.

## Scope

Use `docs/` for repository-local documentation that must evolve with the codebase:

- repository conventions
- runtime architecture notes
- install and deployment references that span multiple components
- operator workflows tied to current self-hosted behavior
- document contracts that are validated in CI

Do not treat external or archived documentation repositories as the active source of truth for self-hosted runtime behavior.

## Source of Truth

For self-hosted behavior, use the following order:

1. `docs/cue/`
2. top-level CI and tests
3. source code under `installer/`, `services/`, and `client/`
4. repository-local docs under `docs/`
5. thin service README files for orientation only

## Structure

- `README.md`
  - docs entry point for this repository
- `architecture.md`
  - cross-component runtime shape and responsibility boundaries
- `testing.md`
  - validation entry points and top-level CI references
- `runtime-contracts.md`
  - machine-checked contracts and generation flow
- `cue/`
  - CUE-backed documentation contracts and canonical facts
- `generated/`
  - generated summaries derived from `docs/cue/`
- `evidence/`
  - committed baselines and audit evidence for accepted repository debt
- `tools/`
  - documentation-local validation and generation entry points
- future topic documents should stay close to the self-hosted source tree and link back to the canonical code paths they describe

## Validation

Use a single repository command for docs and docs-contract validation:

```bash
bash docs/tools/validate.sh
```

Run the full blocking docs check with:

```bash
bash docs/tools/doctor.sh
```

Generate repository-local docs summaries from CUE with:

```bash
bash docs/tools/generate.sh
```

Check that generated docs are current with:

```bash
bash docs/tools/check-generated.sh
```

Check the currently enforced repository-wide hardcoded values with:

```bash
bash docs/tools/check-hardcoded-values.sh
```

Refresh the committed baseline for accepted environment-specific hardcoding with:

```bash
bash docs/tools/update-hardcoding-baseline.sh
```

Print a broader hardcoding report with:

```bash
bash docs/tools/report-hardcoded-values.sh
```

`doctor.sh` is the CI-equivalent blocking path for docs. `report-hardcoded-values.sh` is the broader audit path and does not fail the repo by itself.

`tests/test_cue_facts.sh` remains only as a compatibility wrapper and should not be treated as the primary operator entry point.

## Relationship to Other Paths

- `README.md`
  - primary repository hub
- `docs/cue/`
  - canonical repository facts in CUE
- `docs/generated/`
  - generated documentation summaries
  - [`generated/summary.md`](./generated/summary.md)
  - [`generated/hardcoding-report.md`](./generated/hardcoding-report.md)
- `docs/evidence/`
  - committed hardcoding baselines
  - [`evidence/hardcoding-env-specific-baseline.txt`](./evidence/hardcoding-env-specific-baseline.txt)
- core docs
  - [`architecture.md`](./architecture.md)
  - [`testing.md`](./testing.md)
  - [`runtime-contracts.md`](./runtime-contracts.md)
- `installer/docs/`
  - installer-local references
- `services/localvault/docs/`
  - LocalVault-local references and examples

If a document describes behavior shared across multiple self-hosted components, prefer placing it in this directory instead of scattering duplicated copies across service README files.
