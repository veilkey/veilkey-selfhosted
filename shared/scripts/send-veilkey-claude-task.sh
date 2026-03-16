#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  ./shared/scripts/send-veilkey-claude-task.sh --target <tmux-pane> --file <task.txt>
  ./shared/scripts/send-veilkey-claude-task.sh --target <tmux-pane> --text "<task text>"

Required VeilKey task sections:
  VeilKey Task:
  Workspace:
  Goal:
  Scope:
  Constraints:
  Deliverables:
  Reply:
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Error: required command '$1' not found" >&2
    exit 1
  }
}

validate_task_format() {
  local task_file="$1"
  local missing=0
  local header
  local -a required_headers=(
    "VeilKey Task:"
    "Workspace:"
    "Goal:"
    "Scope:"
    "Constraints:"
    "Deliverables:"
    "Reply:"
  )

  for header in "${required_headers[@]}"; do
    if ! grep -Eq "^${header}" "${task_file}"; then
      echo "Error: missing required section header '${header}'" >&2
      missing=1
    fi
  done

  [[ "${missing}" == "0" ]] || exit 1
}

target=""
task_file=""
task_text=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      target="${2:-}"
      shift 2
      ;;
    --file)
      task_file="${2:-}"
      shift 2
      ;;
    --text)
      task_text="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown argument '$1'" >&2
      exit 2
      ;;
  esac
done

[[ -n "${target}" ]] || {
  echo "Error: --target is required" >&2
  exit 2
}

if [[ -n "${task_file}" && -n "${task_text}" ]]; then
  echo "Error: use either --file or --text, not both" >&2
  exit 2
fi

if [[ -z "${task_file}" && -z "${task_text}" ]]; then
  echo "Error: one of --file or --text is required" >&2
  exit 2
fi

require_cmd claudebridge

tmp_file=""
cleanup() {
  if [[ -n "${tmp_file}" ]]; then
    rm -f "${tmp_file}"
  fi
}
trap cleanup EXIT

if [[ -n "${task_text}" ]]; then
  tmp_file="$(mktemp)"
  printf '%s\n' "${task_text}" > "${tmp_file}"
  task_file="${tmp_file}"
fi

[[ -f "${task_file}" ]] || {
  echo "Error: task file not found: ${task_file}" >&2
  exit 2
}

validate_task_format "${task_file}"

CLAUDEBRIDGE_TARGET="${target}" claudebridge send "$(cat "${task_file}")"
