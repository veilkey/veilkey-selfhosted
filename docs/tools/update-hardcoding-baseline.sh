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

baseline_file="$(cue_string hardcoding.env_specific_baseline_file)"
mkdir -p "$(dirname "${baseline_file}")"
tmp_file="$(mktemp)"
trap 'rm -f "${tmp_file}"' EXIT

collect_env_specific_matches "${tmp_file}"
cp "${tmp_file}" "${baseline_file}"

printf 'Updated %s\n' "${baseline_file}"
