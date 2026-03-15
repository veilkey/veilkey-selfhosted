# Docs CI Integration

This repository includes two GitLab CI jobs that validate the documentation on every push and merge request that touches docs, facts, or versioning files.

## CI Jobs

### `docs:doctor`

Runs `bash docs/tools/doctor.sh` to check documentation health. This script validates that required files exist, cross-references are intact, and the docs structure follows project conventions.

- **Trigger**: changes to `docs/**/*`, `facts/**/*`, `VERSION`, or `VERSIONING.md`
- **Image**: `alpine:3.20`
- **Allowed to fail**: yes (advisory only during initial rollout)

### `docs:generate`

Runs `bash docs/tools/generate.sh` to regenerate derived documentation (inventory, terminology, etc.) and then verifies that the output matches what is already committed. If the generated files differ from what is in the repo, the job fails with a message asking you to regenerate and commit.

- **Trigger**: same file patterns as `docs:doctor`
- **Image**: `alpine:3.20`
- **Artifacts**: `docs/generated/` (kept for 1 week)
- **Allowed to fail**: yes (advisory only during initial rollout)

## Running Locally

You can run the same checks locally before pushing:

```bash
# Validate documentation health
bash docs/tools/doctor.sh

# Regenerate derived docs and check for drift
bash docs/tools/generate.sh
git diff --name-only -- docs/generated/
```

If `git diff` shows changes after running `generate.sh`, commit the updated files in `docs/generated/`.

## What Gets Validated

| Check | Tool | Description |
|-------|------|-------------|
| Required files exist | `doctor.sh` | Ensures core docs files are present |
| Cross-references valid | `doctor.sh` | Checks that links between docs resolve |
| Structure conventions | `doctor.sh` | Validates naming, directory layout |
| Generated docs up to date | `generate.sh` | Regenerates and diffs against committed files |

## Configuration

The CI configuration lives in two files:

- **`/.gitlab-ci.yml`** -- root pipeline config that includes the docs CI via `local: docs/.gitlab-ci.yml`
- **`/docs/.gitlab-ci.yml`** -- defines both `docs:doctor` and `docs:generate` jobs

The docs CI is self-contained and does not interfere with service-level CI pipelines defined in subdirectories.
