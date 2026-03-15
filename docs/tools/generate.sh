#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cue_bin="${repo_root}/.tmp/bin/cue"
out_dir="${repo_root}/docs/generated"
out_file="${out_dir}/summary.md"
hardcoding_file="${out_dir}/hardcoding-report.md"

ensure_cue() {
  if command -v cue >/dev/null 2>&1; then
    command -v cue
    return
  fi
  mkdir -p "${repo_root}/.tmp/bin"
  GOBIN="${repo_root}/.tmp/bin" /usr/local/go/bin/go install cuelang.org/go/cmd/cue@v0.14.0
  printf '%s\n' "${cue_bin}"
}

cue_cmd="$(ensure_cue)"

cue_string() {
  "${cue_cmd}" export ./docs/cue -e "$1" | tr -d '"'
}

cue_lines() {
  "${cue_cmd}" export ./docs/cue -e "$1" | sed -n 's/^[[:space:]]*"\(.*\)"[,]*$/\1/p'
}

generate_components() {
  while IFS= read -r name; do
    [ -n "${name}" ] || continue
    printf '| `%s` | `%s` | `%s` | [`%s`](../%s) |\n' \
      "${name}" \
      "$(cue_string "components.${name}.kind")" \
      "$(cue_string "components.${name}.path")" \
      "$(cue_string "components.${name}.readme")" \
      "$(cue_string "components.${name}.readme")"
  done < <(cue_lines components.names)
}

generate_job_list() {
  local expr="$1"
  while IFS= read -r item; do
    [ -n "${item}" ] || continue
    printf -- '- `%s`\n' "${item}"
  done < <(cue_lines "${expr}")
}

generate_hardcoding_summary() {
  while IFS= read -r item; do
    [ -n "${item}" ] || continue
    printf -- '- `%s`\n' "${item}"
  done < <(cue_lines hardcoding.enforced_literals)
}

cd "${repo_root}"
mkdir -p "${out_dir}"

cat > "${out_file}" <<EOF
# Generated Summary

This file is generated from \`docs/cue/\`. Do not edit it manually.

## Components

| Component | Kind | Path | README |
|---|---|---|---|
$(generate_components)

## Top-Level Validate Jobs

$(generate_job_list ci.validate_jobs)

## Top-Level E2E Jobs

$(generate_job_list ci.e2e_jobs)

## Identity Terms

### Primary Terms

$(generate_job_list identity.primary_terms)

### Compatibility Aliases

$(generate_job_list identity.compatibility_aliases)

## Hardcoding Guardrails

Blocking checks currently fail on these literals:

$(generate_hardcoding_summary)

For the broader audit report, see ./hardcoding-report.md.
EOF

bash docs/tools/report-hardcoded-values.sh --markdown > "${hardcoding_file}"

printf 'Generated %s\n' "${out_file}"
printf 'Generated %s\n' "${hardcoding_file}"
