#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
GUARD_SCRIPT="${REPO_ROOT}/scripts/check-mr-guard.sh"

run_case() {
  local name="$1"
  local setup="$2"
  local expect="$3"
  local tmp
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  git -C "$tmp" init -q
  git -C "$tmp" config user.email test@example.com
  git -C "$tmp" config user.name test
  mkdir -p "$tmp/scripts" "$tmp/tests" "$tmp/internal/api" "$tmp/cli" "$tmp/docs" "$tmp/.gitlab/merge_request_templates"
  cp "$GUARD_SCRIPT" "$tmp/scripts/check-mr-guard.sh"
  chmod +x "$tmp/scripts/check-mr-guard.sh"
  cat > "$tmp/README.md" <<'EOF2'
base
EOF2
  cat > "$tmp/.gitlab-ci.yml" <<'EOF2'
stages:
  - test
EOF2
  git -C "$tmp" add .
  git -C "$tmp" commit -q -m base
  git -C "$tmp" branch -M main
  git -C "$tmp" checkout -q -b feature
  bash -c "cd '$tmp' && $setup"
  result=0
  if ! (cd "$tmp" && CI_MERGE_REQUEST_TARGET_BRANCH_NAME=main CI_MERGE_REQUEST_DIFF_BASE_SHA="$(git rev-parse main)" bash scripts/check-mr-guard.sh) >/tmp/mr-guard.out 2>/tmp/mr-guard.err; then
    result=1
  fi
  if [[ "$result" != "$expect" ]]; then
    echo "case $name failed" >&2
    echo "stdout:" >&2
    cat /tmp/mr-guard.out >&2 || true
    echo "stderr:" >&2
    cat /tmp/mr-guard.err >&2 || true
    exit 1
  fi
}

run_case "runtime without tests" "echo 'package api' > internal/api/new.go; git add internal/api/new.go; git commit -q -m runtime" 1
run_case "runtime with tests and docs" "echo 'package api' > internal/api/new.go; echo 'package api' > internal/api/new_test.go; echo 'doc' >> README.md; git add internal/api/new.go internal/api/new_test.go README.md; git commit -q -m runtime" 0
run_case "cli without docs" "echo '#!/usr/bin/env bash' > cli/veilkey; chmod +x cli/veilkey; echo 'package main' > tests/helper_test.go; git add cli/veilkey tests/helper_test.go; git commit -q -m cli" 1
run_case "docs only" "echo 'more docs' >> README.md; git add README.md; git commit -q -m docs" 0
run_case "deploy with tests and docs" "echo '#!/usr/bin/env bash' > scripts/deploy-host.sh; chmod +x scripts/deploy-host.sh; echo ok > tests/test_deploy.sh; echo 'deploy docs' > docs/deploy.md; git add scripts/deploy-host.sh tests/test_deploy.sh docs/deploy.md; git commit -q -m deploy" 0

echo ok
