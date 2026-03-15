#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cue_bin="${repo_root}/.tmp/bin/cue"
format="text"
tmp_dir="$(mktemp -d)"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

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

if [ "${1:-}" = "--markdown" ]; then
  format="markdown"
fi

cue_lines() {
  "${cue_cmd}" export ./docs/cue -e "$1" | sed -n 's/^[[:space:]]*"\(.*\)"[,]*$/\1/p'
}

scan_one_regex() {
  local regex="$1"
  local path
  local out_file="$2"
  : > "${out_file}"
  while IFS= read -r path; do
    [ -n "${path}" ] || continue
    if [ -d "${repo_root}/${path}" ]; then
      local grep_args=()
      while IFS= read -r skip_path; do
        [ -n "${skip_path}" ] || continue
        grep_args+=("--exclude-dir=$(basename "${skip_path}")")
      done < <(cue_lines hardcoding.report_skip_paths)
      { grep -RInE "${grep_args[@]}" -- "${regex}" "${repo_root}/${path}" 2>/dev/null || true; } >> "${out_file}"
    else
      { grep -nE -- "${regex}" "${repo_root}/${path}" 2>/dev/null || true; } >> "${out_file}"
    fi
  done < <(cue_lines hardcoding.report_scan_paths)
}

cd "${repo_root}"

found_any=0
summary_file="${tmp_dir}/summary.tsv"
: > "${summary_file}"
if [ "${format}" = "markdown" ]; then
  printf '# Hardcoding Report\n\n'
  printf 'This file is generated from `docs/cue/hardcoding.cue`.\n\n'
fi

while IFS= read -r regex; do
  [ -n "${regex}" ] || continue
  regex_key="$(printf '%s' "${regex}" | tr -c 'A-Za-z0-9' '_')"
  match_file="${tmp_dir}/${regex_key}.matches"
  scan_one_regex "${regex}" "${match_file}"
  sed -i '/^$/d' "${match_file}"
  if [ -s "${match_file}" ]; then
    found_any=1
    hits="$(wc -l < "${match_file}" | tr -d ' ')"
    files="$(cut -d: -f1 "${match_file}" | sort -u | wc -l | tr -d ' ')"
    printf '%s\t%s\t%s\n' "${regex}" "${files}" "${hits}" >> "${summary_file}"
  fi
done < <(cue_lines hardcoding.report_regexes)

if [ "${found_any}" -eq 0 ]; then
  echo "No hardcoded values matched the current report rules."
  exit 0
fi

if [ "${format}" = "markdown" ]; then
  printf '## Summary\n\n'
  printf '| Pattern | Files | Hits |\n'
  printf '|---|---:|---:|\n'
  while IFS=$'\t' read -r regex files hits; do
    printf '| `%s` | %s | %s |\n' "${regex}" "${files}" "${hits}"
  done < "${summary_file}"
  printf '\n'
else
  printf '== Summary ==\n'
  while IFS=$'\t' read -r regex files hits; do
    printf '%s | files=%s | hits=%s\n' "${regex}" "${files}" "${hits}"
  done < "${summary_file}"
  printf '\n'
fi

while IFS=$'\t' read -r regex files hits; do
  regex_key="$(printf '%s' "${regex}" | tr -c 'A-Za-z0-9' '_')"
  match_file="${tmp_dir}/${regex_key}.matches"
  if [ "${format}" = "markdown" ]; then
    printf '## `%s`\n\n' "${regex}"
    printf 'Matches: %s across %s files.\n\n' "${hits}" "${files}"
    printf '```text\n'
    cat "${match_file}"
    printf '\n```\n\n'
  else
    printf '== %s ==\n' "${regex}"
    printf 'Matches: %s across %s files.\n' "${hits}" "${files}"
    cat "${match_file}"
    printf '\n\n'
  fi
done < "${summary_file}"
