#!/usr/bin/env bash
set -euo pipefail

TARGET_BRANCH="${CI_MERGE_REQUEST_TARGET_BRANCH_NAME:-main}"
if ! git rev-parse --verify "origin/${TARGET_BRANCH}" >/dev/null 2>&1; then
  git fetch origin "${TARGET_BRANCH}:${TARGET_BRANCH}" >/dev/null 2>&1 || git fetch origin "${TARGET_BRANCH}" >/dev/null 2>&1 || true
fi
BASE_SHA="${CI_MERGE_REQUEST_DIFF_BASE_SHA:-}"
if [[ -z "$BASE_SHA" ]]; then
  BASE_SHA="$(git merge-base HEAD "origin/${TARGET_BRANCH}" 2>/dev/null || true)"
fi
if [[ -z "$BASE_SHA" ]] || ! git cat-file -e "$BASE_SHA^{commit}" >/dev/null 2>&1; then
  BASE_SHA="$(git rev-parse HEAD~1 2>/dev/null || true)"
fi
if [[ -z "$BASE_SHA" ]]; then
  echo "mr-guard: could not determine base sha" >&2
  exit 1
fi

changed="$(git diff --name-only "$BASE_SHA" HEAD)"
[[ -n "$changed" ]] || { echo 'mr-guard: no changed files'; exit 0; }
printf '%s
' "$changed"

has_match() {
  local pattern="$1"
  printf '%s
' "$changed" | grep -Eq "$pattern"
}

runtime_pattern='(^cmd/|^internal/|^cli/|^cli-src/|^plugins/|^scripts/|(^|/)install\.sh$|^\.gitlab-ci\.yml$|(^|/)[^/]+\.go$)'
deploy_pattern='(^scripts/deploy|(^|/)install\.sh$|^\.gitlab-ci\.yml$|^docker/|^plugins/.*/install\.sh$)'
operator_doc_pattern='(^README\.md$|^CONTRIBUTING\.md$|^docs/|^\.gitlab/merge_request_templates/)'
test_pattern='(^tests?/|(^|/)[^/]+_test\.go$|(^|/)test\.sh$|(^|/)[^/]+_test\.sh$)'

need_tests=0
need_docs=0
if has_match "$runtime_pattern"; then
  need_tests=1
fi
if has_match '(^cli/|^cli-src/|^cmd/|^internal/api/|^scripts/deploy|(^|/)install\.sh$|^\.gitlab-ci\.yml$|^README\.md$)'; then
  need_docs=1
fi
if has_match "$deploy_pattern"; then
  need_tests=1
  need_docs=1
fi

if [[ "$need_tests" -eq 1 ]] && ! has_match "$test_pattern"; then
  echo 'mr-guard: runtime/deploy changes require test updates in the same MR' >&2
  exit 1
fi
if [[ "$need_docs" -eq 1 ]] && ! has_match "$operator_doc_pattern"; then
  echo 'mr-guard: user-facing or deploy changes require README/docs updates in the same MR' >&2
  exit 1
fi

echo 'mr-guard: pass'
