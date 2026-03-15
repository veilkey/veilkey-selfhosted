#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
iptables_log="$tmp/iptables.log"
dump_debug() {
  rc=$?
  if [[ $rc -ne 0 ]]; then
    echo '--- veilroot-egress-guard debug ---' >&2
    [[ -f "$iptables_log" ]] && cat "$iptables_log" >&2 || true
  fi
  rm -rf "$tmp"
  exit $rc
}
trap dump_debug EXIT

cat > "$tmp/session-config" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
cmd="$1"
shift
case "$cmd" in
  veilroot-default-profile)
    echo codex
    ;;
  veilroot-unit-prefix)
    echo veilroot
    ;;
  *)
    exit 2
    ;;
esac
SCRIPT
chmod +x "$tmp/session-config"

cat > "$tmp/iptables" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
printf 'iptables %s\n' "\$*" >> "$iptables_log"
SCRIPT
chmod +x "$tmp/iptables"

cat > "$tmp/ip6tables" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
printf 'ip6tables %s\n' "\$*" >> "$iptables_log"
SCRIPT
chmod +x "$tmp/ip6tables"

mkdir -p "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"

guard="$PWD/deploy/host/veilkey-veilroot-egress-guard"
export VEILKEY_SESSION_CONFIG_BIN="$tmp/session-config"
export IPTABLES_BIN="$tmp/iptables"
export IP6TABLES_BIN="$tmp/ip6tables"
export VEILKEY_VEILROOT_CGROUP_ROOT="$tmp/sys/fs/cgroup/system.slice"
export VEILKEY_VEILROOT_MATCH_PATH=/system.slice/veilroot-codex.scope

run_guard() {
  "$guard" codex "$1"
}

run_guard apply
test -s "$iptables_log"
grep -Fq -- '-C OUTPUT -m cgroup --path /system.slice/veilroot-codex.scope -j VK_VEILROOT_CODEX_EGRESS' "$iptables_log"
grep -Fq -- '-A VK_VEILROOT_CODEX_EGRESS -d 10.0.0.0/8 -j RETURN' "$iptables_log"
grep -Fq -- '-A VK_VEILROOT_CODEX_EGRESS -j REJECT --reject-with icmp-admin-prohibited' "$iptables_log"
grep -Fq -- 'ip6tables -w -C OUTPUT -m cgroup --path /system.slice/veilroot-codex.scope -j VK_VEILROOT_CODEX_EGRESS6' "$iptables_log"

: > "$iptables_log"
run_guard status >/dev/null
test -s "$iptables_log"
grep -Fq -- '-S OUTPUT' "$iptables_log"
grep -Fq -- '-S VK_VEILROOT_CODEX_EGRESS' "$iptables_log"

: > "$iptables_log"
run_guard remove
test -s "$iptables_log"
grep -Fq -- '-D OUTPUT -m cgroup --path /system.slice/veilroot-codex.scope -j VK_VEILROOT_CODEX_EGRESS' "$iptables_log"
grep -Fq -- '-X VK_VEILROOT_CODEX_EGRESS' "$iptables_log"

rm -rf "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"
: > "$iptables_log"
run_guard watch &
watch_pid=$!
sleep 1
mkdir -p "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"
sleep 2
rm -rf "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"
sleep 2
kill "$watch_pid"
wait "$watch_pid" || true
test -s "$iptables_log"
grep -Fq -- '-C OUTPUT -m cgroup --path /system.slice/veilroot-codex.scope -j VK_VEILROOT_CODEX_EGRESS' "$iptables_log"
grep -Fq -- '-D OUTPUT -m cgroup --path /system.slice/veilroot-codex.scope -j VK_VEILROOT_CODEX_EGRESS' "$iptables_log"

echo "ok: veilroot egress guard"
