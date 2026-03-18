#!/usr/bin/env bash
set -euo pipefail

# VeilKey Self-Hosted Docs Governance — Doctor / Validation Script
# Validates repository structure, terminology compliance, and schema integrity.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

pass_count=0
fail_count=0

pass() {
  echo "  [PASS] $1"
  pass_count=$((pass_count + 1))
}

fail() {
  echo "  [FAIL] $1"
  fail_count=$((fail_count + 1))
}

header() {
  echo ""
  echo "=== $1 ==="
}

ensure_cue() {
  if command -v cue >/dev/null 2>&1; then
    command -v cue
    return
  fi

  # Download pre-built CUE binary (no Go toolchain required)
  local cue_bin="$REPO_ROOT/.tmp/bin/cue"
  local cue_version="v0.14.0"
  local os arch tarball
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  [[ "$arch" == "x86_64" ]] && arch="amd64"
  [[ "$arch" == "aarch64" ]] && arch="arm64"
  tarball="cue_${cue_version}_${os}_${arch}.tar.gz"

  mkdir -p "$REPO_ROOT/.tmp/bin"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "https://github.com/cue-lang/cue/releases/download/${cue_version}/${tarball}" \
      | tar xz -C "$REPO_ROOT/.tmp/bin" cue 2>/dev/null
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "https://github.com/cue-lang/cue/releases/download/${cue_version}/${tarball}" \
      | tar xz -C "$REPO_ROOT/.tmp/bin" cue 2>/dev/null
  fi

  if [[ -x "$cue_bin" ]]; then
    echo "$cue_bin"
    return
  fi

  return 1
}

echo "doctor.sh — VeilKey self-hosted docs governance validation"

# ---------------------------------------------------------------------------
# 1. Required root files
# ---------------------------------------------------------------------------
header "Required root files"

for f in README.md VERSION VERSIONING.md; do
  if [[ -f "$REPO_ROOT/$f" ]]; then
    pass "$f exists"
  else
    fail "$f is missing"
  fi
done

# ---------------------------------------------------------------------------
# 2. Required service docs
# ---------------------------------------------------------------------------
header "Required service docs"

for f in services/keycenter/README.md services/localvault/README.md; do
  if [[ -f "$REPO_ROOT/$f" ]]; then
    pass "$f exists"
  else
    fail "$f is missing"
  fi
done

# ---------------------------------------------------------------------------
# 3. docs/cue/docs_contract.cue
# ---------------------------------------------------------------------------
header "CUE contract"

if [[ -f "$REPO_ROOT/docs/cue/docs_contract.cue" ]]; then
  pass "docs/cue/docs_contract.cue exists"
else
  fail "docs/cue/docs_contract.cue is missing"
fi

# ---------------------------------------------------------------------------
# 4. docs/tools/generate.sh
# ---------------------------------------------------------------------------
header "Generator script"

if [[ -f "$REPO_ROOT/docs/tools/generate.sh" ]]; then
  pass "docs/tools/generate.sh exists"
else
  fail "docs/tools/generate.sh is missing"
fi

# ---------------------------------------------------------------------------
# 5. VERSION semver validation
# ---------------------------------------------------------------------------
header "VERSION semver check"

if [[ -f "$REPO_ROOT/VERSION" ]]; then
  version_content="$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")"
  # Match semver: MAJOR.MINOR.PATCH with optional pre-release and build metadata
  if [[ "$version_content" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
    pass "VERSION ($version_content) matches semver pattern"
  else
    fail "VERSION content '$version_content' does not match semver pattern"
  fi
else
  fail "VERSION file not found (cannot validate semver)"
fi

# ---------------------------------------------------------------------------
# 6. Blocked terminology scan
# ---------------------------------------------------------------------------
header "Blocked terminology scan"

# Blocked terms -- bare forms that must NOT appear in docs/.
# Source code may use compatibility aliases (node_id, vaultHash, etc.) legitimately.
# Canonical VeilKey identity terms are: vault_node_uuid, vault_hash, vault_runtime_hash.
blocked_terms=(
  "node_uuid"
  "nodeUUID"
  "nodeId"
  "node_id"
  "runtimeHash"
  "runtime_hash"
  "vaultHash"
)

term_violations=0

for term in "${blocked_terms[@]}"; do
  # Search *.cue, *.md, *.go, *.toml files.
  # Exclude the blocked_terms definition block inside docs_contract.cue itself.
  # Exclude lines where the term appears as part of a canonical vault_ prefixed identifier.
  # Only scan docs/ directory — source code may use compatibility aliases legitimately.
  # Exclude docs_contract.cue (defines the blocked list) and lines that document
  # compatibility aliases (containing "alias" or "compatibility").
  matches=$(
    grep -rn -P "(?<!vault_)(?<![a-zA-Z_])${term}(?![a-zA-Z_])" \
      --include='*.cue' --include='*.md' --include='*.toml' \
      "$REPO_ROOT/docs" 2>/dev/null \
    | grep -v 'docs/cue/docs_contract.cue' \
    | grep -v 'docs/cue/terminology.cue' \
    | grep -vi 'alias\|compatibility' \
    || true
  )

  if [[ -n "$matches" ]]; then
    fail "Blocked term '$term' found:"
    echo "$matches" | while IFS= read -r line; do
      echo "         $line"
    done
    term_violations=$((term_violations + 1))
  fi
done

if [[ "$term_violations" -eq 0 ]]; then
  pass "No blocked terminology found"
fi

# ---------------------------------------------------------------------------
# 7. Generated docs directory and files
# ---------------------------------------------------------------------------
header "Generated docs"

if [[ -d "$REPO_ROOT/docs/generated" ]]; then
  pass "docs/generated/ directory exists"
else
  fail "docs/generated/ directory is missing"
fi

for f in docs/generated/inventory.md docs/generated/terminology.md; do
  if [[ -f "$REPO_ROOT/$f" ]]; then
    pass "$f exists"
  else
    fail "$f is missing"
  fi
done

# ---------------------------------------------------------------------------
# 8. CUE schema validation (if cue CLI is available)
# ---------------------------------------------------------------------------
header "CUE schema validation"

if cue_cmd="$(ensure_cue)"; then
  if cue_output=$(cd "$REPO_ROOT" && "$cue_cmd" vet -c=false docs/cue/*.cue 2>&1); then
    pass "cue vet docs/cue/*.cue succeeded"
  else
    fail "cue vet docs/cue/*.cue failed"
    echo "$cue_output" | while IFS= read -r line; do
      echo "         $line"
    done
  fi

  tmp_version_file="$(mktemp --suffix=.cue)"
  trap 'rm -f "$tmp_version_file"' EXIT
  cat > "$tmp_version_file" <<EOF
package docs

version: "$(tr -d '[:space:]' < "$REPO_ROOT/VERSION")"
EOF

  if cue_output=$(cd "$REPO_ROOT" && "$cue_cmd" vet -c=false docs/cue/docs_contract.cue "$tmp_version_file" 2>&1); then
    pass "VERSION unifies with docs/cue/docs_contract.cue"
  else
    fail "VERSION does not unify with docs/cue/docs_contract.cue"
    echo "$cue_output" | while IFS= read -r line; do
      echo "         $line"
    done
  fi
else
  fail "cue CLI not found and could not be installed"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "==========================================="
total=$((pass_count + fail_count))
echo "  Total checks: $total"
echo "  Passed:       $pass_count"
echo "  Failed:       $fail_count"

if [[ "$fail_count" -eq 0 ]]; then
  echo "  Result:       PASS"
  echo "==========================================="
  exit 0
else
  echo "  Result:       FAIL"
  echo "==========================================="
  exit 1
fi
