#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../.."

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

fake_bin="${tmp_dir}/bin"
mkdir -p "${fake_bin}"
cat > "${fake_bin}/claudebridge" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "$1" == "send" ]] || exit 9
exit 0
EOF
chmod +x "${fake_bin}/claudebridge"
export PATH="${fake_bin}:$PATH"

ok_task="${tmp_dir}/ok.txt"
bad_task="${tmp_dir}/bad.txt"

cat > "${ok_task}" <<'EOF'
VeilKey Task:
  Check deploy readiness.
Workspace:
  /opt/veilkey-delegates/team-1
Goal:
  Decide whether deploy can proceed.
Scope:
  installer/ only
Constraints:
  Korean response, no unrelated edits
Deliverables:
  files changed, blockers, tests
Reply:
  concise Korean findings
EOF

cat > "${bad_task}" <<'EOF'
Workspace:
  /tmp/example
Goal:
  Broken task
EOF

if ! ./shared/scripts/send-veilkey-claude-task.sh --target 20:0.0 --file "${ok_task}" >/dev/null 2>&1; then
  echo "expected valid VeilKey task format to pass validation" >&2
  exit 1
fi

if ./shared/scripts/send-veilkey-claude-task.sh --target 20:0.0 --file "${bad_task}" >/dev/null 2>&1; then
  echo "expected invalid VeilKey task format to fail validation" >&2
  exit 1
fi

echo "ok: send-veilkey-claude-task"
