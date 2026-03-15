#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cue_bin="${repo_root}/.tmp/bin/cue"

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

cd "${repo_root}"

"${cue_cmd}" vet ./docs/cue

cue_string() {
  "${cue_cmd}" export ./docs/cue -e "$1" | tr -d '"'
}

cue_lines() {
  "${cue_cmd}" export ./docs/cue -e "$1" | sed -n 's/^[[:space:]]*"\(.*\)"[,]*$/\1/p'
}

require_path() {
  local expr="$1"
  local path
  path="$(cue_string "$expr")"
  test -e "${repo_root}/${path}"
}

require_job() {
  local expr="$1"
  local job
  job="$(cue_string "$expr")"
  grep -F "${job}:" "${repo_root}/.gitlab-ci.yml" >/dev/null
}

require_local_ci_job() {
  local path_expr="$1"
  local job_expr="$2"
  local path job
  path="$(cue_string "${path_expr}")"
  job="$(cue_string "${job_expr}")"
  grep -F "${job}:" "${repo_root}/${path}" >/dev/null
}

require_contains() {
  local path_expr="$1"
  local expected_expr="$2"
  local path expected
  path="$(cue_string "${path_expr}")"
  expected="$(cue_string "${expected_expr}")"
  grep -F "${expected}" "${repo_root}/${path}" >/dev/null
}

require_contains_text() {
  local path_expr="$1"
  local expected="$2"
  local path
  path="$(cue_string "${path_expr}")"
  grep -F "${expected}" "${repo_root}/${path}" >/dev/null
}

require_no_forbidden_literals() {
  local literal path output
  while IFS= read -r literal; do
    [ -n "${literal}" ] || continue
    while IFS= read -r path; do
      [ -n "${path}" ] || continue
      if [ -d "${repo_root}/${path}" ]; then
        output="$(grep -RInF --exclude-dir=.git --exclude-dir=.tmp --exclude-dir=generated -- "${literal}" "${repo_root}/${path}" 2>/dev/null || true)"
        while IFS= read -r skip_path; do
          [ -n "${skip_path}" ] || continue
          output="$(printf '%s\n' "${output}" | grep -v "^${repo_root}/${skip_path}/" || true)"
        done < <(cue_lines endpoints.forbidden_skip_paths)
      else
        output="$(grep -nF -- "${literal}" "${repo_root}/${path}" 2>/dev/null || true)"
      fi
      if [ -n "${output}" ]; then
        printf 'Forbidden literal found: %s\n%s\n' "${literal}" "${output}" >&2
        return 1
      fi
    done < <(cue_lines endpoints.forbidden_scan_paths)
  done < <(cue_lines endpoints.forbidden_literals)
}

test "$(cue_string repo.name)" = "veilkey-selfhosted"
test "$(cue_string repo.domain)" = "self-hosted"

require_path repo.root_readme
require_path repo.facts_dir
require_path repo.canonical_docs.root
require_path repo.canonical_docs.keycenter
require_path repo.canonical_docs.localvault
require_path repo.paths.installer
require_path repo.paths.keycenter
require_path repo.paths.localvault
require_path repo.paths.proxy
require_path repo.paths.cli
require_path repo.paths.docs
require_path docs.primary_entrypoint
require_path docs.repository_docs_hub
while IFS= read -r path; do
  test -e "${repo_root}/${path}"
done < <(cue_lines docs.service_readmes)

while IFS= read -r name; do
  require_path "components.${name}.path"
  require_path "components.${name}.readme"
  require_path "components.${name}.local_ci_file"
  while IFS= read -r local_job; do
    require_local_ci_job "components.${name}.local_ci_file" "\"${local_job}\""
  done < <(cue_lines "components.${name}.local_ci_jobs")
done < <(cue_lines components.names)

while IFS= read -r job; do
  require_job "\"${job}\""
done < <(cue_lines ci.validate_jobs)

while IFS= read -r job; do
  require_job "\"${job}\""
done < <(cue_lines ci.e2e_jobs)

require_contains docs.primary_entrypoint repo.name
require_contains docs.primary_entrypoint docs.canonical_facts_dir
while IFS= read -r expected; do
  require_contains_text docs.primary_entrypoint "${expected}"
done < <(cue_lines docs.root_required_strings)

for name in keycenter localvault; do
  while IFS= read -r expected; do
    require_contains_text "components.${name}.readme" "${expected}"
  done < <(cue_lines "docs.service_readme_required_strings.${name}")
done

require_no_forbidden_literals

echo "Docs validation passed."
