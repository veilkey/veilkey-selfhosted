#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

script="$PWD/deploy/host/veil-work-container"

out="$(PATH="$tmp:/usr/bin:/bin" VEIL_WORK_CONTAINER_DRY_RUN=1 USER=alice "$script" 2>&1 || true)"
assert_contains "$out" "no supported container runtime found"

cat >"$tmp/docker" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "docker $*"
SCRIPT
chmod +x "$tmp/docker"

out="$(PATH="$tmp:$PATH" USER=alice VEIL_WORK_CONTAINER_DRY_RUN=1 VEIL_WORK_CONTAINER_WORKSPACE=/work/demo VEIL_WORK_CONTAINER_IMAGE=ghcr.io/example/veil:test "$script")"
assert_contains "$out" "runtime=docker"
assert_contains "$out" "inspect=docker"
assert_contains "$out" "inspect=veil-alice"
assert_contains "$out" "run=docker"
assert_contains "$out" "run=--name"
assert_contains "$out" "run=veil-alice"
assert_contains "$out" "veil-home-alice:/home/veil"
assert_contains "$out" "/work/demo:/workspace"
assert_contains "$out" "ghcr.io/example/veil:test"
assert_contains "$out" "exec=docker"
assert_contains "$out" "exec=veil-alice"
assert_contains "$out" "exec=sh"
assert_contains "$out" "exec=-lc"
assert_contains "$out" "tmux\\ new-session\\ -Ad\\ -s\\ main"

echo "ok: veil work container"
