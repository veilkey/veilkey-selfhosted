#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

script="$PWD/deploy/host/veilcontainer"

out="$(PATH="$tmp" VEIL_WORK_CONTAINER_DRY_RUN=1 USER=alice /bin/bash "$script" 2>&1 || true)"
assert_contains "$out" "veilcontainer: no supported container runtime found"

cat >"$tmp/docker" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "docker $*"
SCRIPT
chmod +x "$tmp/docker"

out="$(env -u VEIL_WORK_CONTAINER_SHELL PATH="$tmp:$PATH" USER=alice VEIL_WORK_CONTAINER_DRY_RUN=1 VEIL_WORK_CONTAINER_WORKSPACE=/work/demo VEIL_WORK_CONTAINER_IMAGE=ghcr.io/example/veil:test "$script")"
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
assert_contains "$out" "veilcontainer-tmux"

tmux_script="$PWD/deploy/host/veilcontainer-tmux"
out="$(VEIL_WORK_TMUX_DRY_RUN=1 "$tmux_script")"
assert_contains "$out" "tmux has-session -t main"
assert_contains "$out" "tmux new-session -d -s main -n shell"
assert_contains "$out" "tmux new-session -d -s main -n shell veil"
assert_contains "$out" "tmux new-window -t main -n claude veil"
assert_contains "$out" "tmux new-window -t main -n codex veil"
assert_contains "$out" "exec tmux attach -t main"

echo "ok: veilcontainer"
