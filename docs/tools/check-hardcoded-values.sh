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

cue_lines() {
  "${cue_cmd}" export ./docs/cue -e "$1" | sed -n 's/^[[:space:]]*"\(.*\)"[,]*$/\1/p'
}

cue_string() {
  "${cue_cmd}" export ./docs/cue -e "$1" | tr -d '"'
}

check_literal() {
  local literal="$1"
  local path output
  while IFS= read -r path; do
    [ -n "${path}" ] || continue
    if [ -d "${repo_root}/${path}" ]; then
      output="$(grep -RInF --exclude-dir=.git --exclude-dir=.tmp --exclude-dir=generated -- "${literal}" "${repo_root}/${path}" 2>/dev/null || true)"
      while IFS= read -r skip_path; do
        [ -n "${skip_path}" ] || continue
        output="$(printf '%s\n' "${output}" | grep -v "^${repo_root}/${skip_path}/" || true)"
      done < <(cue_lines hardcoding.enforced_skip_paths)
    else
      output="$(grep -nF -- "${literal}" "${repo_root}/${path}" 2>/dev/null || true)"
    fi

    if [ -n "${output}" ]; then
      printf 'Forbidden hardcoded value found: %s\n%s\n' "${literal}" "${output}" >&2
      return 1
    fi
  done < <(cue_lines hardcoding.enforced_scan_paths)
}

collect_env_specific_matches() {
  local out_file="$1"
  local regex path
  : > "${out_file}"
  while IFS= read -r regex; do
    [ -n "${regex}" ] || continue
    while IFS= read -r path; do
      [ -n "${path}" ] || continue
      if [ -d "${repo_root}/${path}" ]; then
        local grep_args=()
        while IFS= read -r skip_path; do
          [ -n "${skip_path}" ] || continue
          grep_args+=("--exclude-dir=$(basename "${skip_path}")")
        done < <(cue_lines hardcoding.env_specific_skip_paths)
        { grep -RInE "${grep_args[@]}" -- "${regex}" "${repo_root}/${path}" 2>/dev/null || true; } >> "${out_file}"
      else
        { grep -nE -- "${regex}" "${repo_root}/${path}" 2>/dev/null || true; } >> "${out_file}"
      fi
    done < <(cue_lines hardcoding.env_specific_scan_paths)
  done < <(cue_lines hardcoding.env_specific_regexes)

  sed -i '/^$/d' "${out_file}"
  sort -u -o "${out_file}" "${out_file}"
}

cd "${repo_root}"

while IFS= read -r literal; do
  [ -n "${literal}" ] || continue
  check_literal "${literal}"
done < <(cue_lines hardcoding.enforced_literals)

baseline_file="$(cue_string hardcoding.env_specific_baseline_file)"
tmp_current="$(mktemp)"
tmp_new="$(mktemp)"
trap 'rm -f "${tmp_current}" "${tmp_new}"' EXIT

collect_env_specific_matches "${tmp_current}"

if [ ! -f "${baseline_file}" ]; then
  printf 'Missing hardcoding baseline: %s\nRun: bash docs/tools/update-hardcoding-baseline.sh\n' "${baseline_file}" >&2
  exit 1
fi

sort -u "${baseline_file}" > "${tmp_new}.baseline"
comm -13 "${tmp_new}.baseline" "${tmp_current}" > "${tmp_new}" || true

if [ -s "${tmp_new}" ]; then
  printf 'New environment-specific hardcoded values detected beyond baseline:\n%s\n' "$(cat "${tmp_new}")" >&2
  printf 'Review and either remove them or refresh the baseline with: bash docs/tools/update-hardcoding-baseline.sh\n' >&2
  rm -f "${tmp_new}.baseline"
  exit 1
fi

rm -f "${tmp_new}.baseline"

echo "Hardcoded value check passed."
