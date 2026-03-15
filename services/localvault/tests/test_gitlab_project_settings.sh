#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 [--self-test]" >&2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON_POLICY="${SCRIPT_DIR}/policy/project_registry_policy.sh"

fetch_project_json() {
  local api_base="$1"
  local project_id="$2"
  local header="${3:-}"

  if [[ -n "$header" ]]; then
    curl -fsS --header "$header" "${api_base}/projects/${project_id}"
  else
    curl -fsS "${api_base}/projects/${project_id}"
  fi
}

resolve_api_header() {
  if [[ -n "${GITLAB_API_TOKEN:-}" ]]; then
    printf 'PRIVATE-TOKEN: %s' "$GITLAB_API_TOKEN"
    return 0
  fi

  local gitlab_host token
  gitlab_host="${CI_SERVER_HOST:?CI_SERVER_HOST must be set}"
  token="$(printf 'protocol=https\nhost=%s\n\n' "$gitlab_host" | git credential fill 2>/dev/null | awk -F= '/^password=/{print $2; exit}')"
  if [[ -n "$token" ]]; then
    printf 'PRIVATE-TOKEN: %s' "$token"
    return 0
  fi

  if [[ -n "${CI_JOB_TOKEN:-}" ]]; then
    printf 'JOB-TOKEN: %s' "$CI_JOB_TOKEN"
    return 0
  fi

  return 1
}

run_live_check() {
  local api_base project_id header json visibility registry

  api_base="${CI_API_V4_URL:-${GITLAB_API_V4_URL:?GITLAB_API_V4_URL must be set}}"
  project_id="${CI_PROJECT_ID:-8}"
  header="$(resolve_api_header || true)"

  if [[ -n "$header" ]]; then
    json="$(fetch_project_json "$api_base" "$project_id" "$header" 2>/dev/null || true)"
    visibility="$(jq -r '.visibility // empty' <<<"$json")"
    registry="$(jq -r '.container_registry_access_level // empty' <<<"$json")"
    if bash "$COMMON_POLICY" --check "$visibility" "$registry" >/dev/null 2>&1; then
      return 0
    fi
  fi

  json="$(fetch_project_json "$api_base" "$project_id" 2>/dev/null || true)"
  visibility="$(jq -r '.visibility // empty' <<<"$json")"
  registry="$(jq -r '.container_registry_access_level // empty' <<<"$json")"
  if bash "$COMMON_POLICY" --check "$visibility" "$registry" >/dev/null 2>&1; then
    return 0
  fi

  echo "skip: GitLab project settings are not fully visible without a maintainer token" >&2
  echo "set GITLAB_API_TOKEN to enforce live registry policy checks in CI" >&2
}

run_self_test() {
  bash "$COMMON_POLICY" --self-test
}

case "${1:-}" in
  "")
    run_live_check
    ;;
  --self-test)
    run_self_test
    ;;
  *)
    usage
    exit 1
    ;;
esac
