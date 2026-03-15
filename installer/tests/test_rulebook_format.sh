#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VALIDATOR="${REPO_ROOT}/scripts/validate_rulebook.py"

python3 "$VALIDATOR" --rulebook "${REPO_ROOT}/rulebook/veilkey-rulebook.toml" >/dev/null

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat > "${tmp}/bad.toml" <<'EOF'
version = 1
name = "bad"
channel = "draft"

[categories]
required = ["ref_model"]

[consumers.mr_guard]
runtime_paths = ["^internal/"]
deploy_paths = ["^scripts/deploy"]
docs_required_paths = ["^internal/api/"]
doc_paths = ["^README\\.md$"]
test_paths = ["^tests?/"]

[[rules]]
id = "VK-REF-001"
category = "missing"
level = "error"
summary = "bad"
message = "bad"
EOF

if python3 "$VALIDATOR" --rulebook "${tmp}/bad.toml" >/tmp/rulebook.out 2>/tmp/rulebook.err; then
  echo "expected invalid rulebook to fail" >&2
  exit 1
fi
grep -q "unknown category" /tmp/rulebook.err

echo "ok"
