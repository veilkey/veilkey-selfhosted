#!/bin/bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

assert_missing_cmd_fails() {
  local missing_path="$1"
  local want="$2"
  local out
  if out="$(PATH="$missing_path" bash "${REPO_ROOT}/scripts/deploy-lxc.sh" 2>&1)"; then
    echo "expected deploy-lxc.sh to fail without prerequisites" >&2
    exit 1
  fi
  grep -F "$want" <<<"$out" >/dev/null || {
    echo "missing expected error: $want" >&2
    echo "$out" >&2
    exit 1
  }
}

tmp_bin="$(mktemp -d)"
trap 'rm -rf "$tmp_bin"' EXIT

ln -sf /usr/bin/env "$tmp_bin/env"
ln -sf /bin/bash "$tmp_bin/bash"

assert_missing_cmd_fails "$tmp_bin" "required command not found: pct"
