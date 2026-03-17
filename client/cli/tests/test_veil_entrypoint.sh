#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/veil-work-container" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "work-container:$*"
SCRIPT
chmod +x "$tmp/veil-work-container"

cat >"$tmp/veilroot-shell" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "legacy:$*"
SCRIPT
chmod +x "$tmp/veilroot-shell"

out="$(VEIL_WORK_CONTAINER_BIN="$tmp/veil-work-container" "$PWD/deploy/host/veil" codex)"
assert_contains "$out" "work-container:codex"

out="$(VEIL_WORK_CONTAINER_BIN="$tmp/missing-work-container" "$PWD/deploy/host/veil" 2>&1 || true)"
assert_contains "$out" "work-container entrypoint not found"
assert_contains "$out" "VEIL_LEGACY_VEILROOT=1"

out="$(VEIL_WORK_CONTAINER_BIN="$tmp/missing-work-container" VEIL_LEGACY_VEILROOT=1 VEILKEY_VEILROOT_SHELL_BIN="$tmp/veilroot-shell" "$PWD/deploy/host/veil" open)"
assert_contains "$out" "legacy:open"

echo "ok: veil entrypoint"
