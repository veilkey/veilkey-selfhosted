#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

TEST_PROXY_HOST="10.0.0.3"
TEST_NO_PROXY_DOMAINS=".test.internal,.vhost.test"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/home/workspace"

cat > "$tmp/session-config" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
cmd="\$1"
shift
case "\$cmd" in
  tool-shell-exports)
    tool="\$1"
    case "\$tool" in
      codex)
        cat <<EOF
export VEILKEY_PROXY_URL=http://${TEST_PROXY_HOST}:18081
export HTTP_PROXY=http://${TEST_PROXY_HOST}:18081
export HTTPS_PROXY=http://${TEST_PROXY_HOST}:18081
export ALL_PROXY=http://${TEST_PROXY_HOST}:18081
export NO_PROXY=127.0.0.1,localhost,${TEST_NO_PROXY_DOMAINS}
EOF
        ;;
      claude)
        cat <<EOF
export VEILKEY_PROXY_URL=http://${TEST_PROXY_HOST}:18084
export HTTP_PROXY=http://${TEST_PROXY_HOST}:18084
export HTTPS_PROXY=http://${TEST_PROXY_HOST}:18084
export ALL_PROXY=http://${TEST_PROXY_HOST}:18084
export NO_PROXY=127.0.0.1,localhost,${TEST_NO_PROXY_DOMAINS}
EOF
        ;;
      *)
        exit 2
        ;;
    esac
    ;;
  tool-proxy-url)
    tool="\$1"
    case "\$tool" in
      codex) echo http://${TEST_PROXY_HOST}:18081 ;;
      claude) echo http://${TEST_PROXY_HOST}:18084 ;;
      *) exit 2 ;;
    esac
    ;;
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

cat > "$tmp/systemd-run" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
tmpdir="${TMPDIR:-/tmp}"
printf '%s\n' "$@" > "$tmpdir/systemd-run.args"
while (($#)); do
  case "$1" in
    --quiet|--scope)
      shift
      ;;
    --working-directory)
      wd="$2"
      cd "$wd"
      shift 2
      ;;
    --unit)
      shift 2
      ;;
    --setenv)
      kv="$2"
      export "$kv"
      shift 2
      ;;
    *)
      break
      ;;
  esac
done
exec "$@"
SCRIPT
chmod +x "$tmp/systemd-run"

cat > "$tmp/cgroup-cat" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-lc" ]]; then
  printf '%s\n' "VEILKEY_PROXY_URL=${VEILKEY_PROXY_URL}"
  printf '%s\n' "HTTP_PROXY=${HTTP_PROXY}"
  printf '%s\n' "HTTPS_PROXY=${HTTPS_PROXY}"
  printf '%s\n' "ALL_PROXY=${ALL_PROXY}"
  printf '%s\n' "NO_PROXY=${NO_PROXY}"
  printf '%s\n' "VEILKEY_VEILROOT=${VEILKEY_VEILROOT}"
  printf '%s\n' "VEILKEY_VEILROOT_PROFILE=${VEILKEY_VEILROOT_PROFILE}"
  echo ---
  echo '0::/system.slice/veilroot-codex.scope'
else
  exec "$@"
fi
SCRIPT
chmod +x "$tmp/cgroup-cat"

launcher="$PWD/deploy/host/veilkey-veilroot-session"
verify="$PWD/deploy/host/verify-veilroot-session.sh"
observe="$PWD/deploy/host/veilkey-veilroot-observe"
install_script="$PWD/deploy/host/install-veilroot-boundary.sh"
install_codex_script="$PWD/deploy/host/install-veilroot-codex.sh"

out="$(HOME="$tmp/home" TMPDIR="$tmp" VEILKEY_SESSION_CONFIG_BIN="$tmp/session-config" SYSTEMD_RUN_BIN="$tmp/systemd-run" SHELL_BIN="$tmp/cgroup-cat" VEILKEY_VEILROOT_SCOPE='veilroot-codex.scope' "$launcher" codex)"
printf '%s\n' "$out" | grep -q "^VEILKEY_PROXY_URL=http://${TEST_PROXY_HOST}:18081$"
printf '%s\n' "$out" | grep -q '^VEILKEY_VEILROOT=1$'
printf '%s\n' "$out" | grep -q '^VEILKEY_VEILROOT_PROFILE=codex$'
grep -q -- '--unit' "$tmp/systemd-run.args"
grep -q -- 'veilroot-codex.scope' "$tmp/systemd-run.args"

HOME="$tmp/home" VEILKEY_SESSION_CONFIG_BIN="$tmp/session-config" VEILKEY_VEILROOT_LAUNCHER="$launcher" SYSTEMD_RUN_BIN="$tmp/systemd-run" SHELL_BIN="$tmp/cgroup-cat" VEILKEY_VEILROOT_SCOPE='veilroot-codex.scope' "$verify" codex >/dev/null

out_default="$(HOME="$tmp/home" TMPDIR="$tmp" VEILKEY_SESSION_CONFIG_BIN="$tmp/session-config" SYSTEMD_RUN_BIN="$tmp/systemd-run" SHELL_BIN="$tmp/cgroup-cat" VEILKEY_VEILROOT_SCOPE='veilroot-codex.scope' "$launcher")"
printf '%s\n' "$out_default" | grep -q '^VEILKEY_VEILROOT_PROFILE=codex$'

mkdir -p "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"
cat > "$tmp/observe-bin" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$@"
SCRIPT
chmod +x "$tmp/observe-bin"

obs_out="$(VEILKEY_SESSION_CONFIG_BIN="$tmp/session-config" VEILKEY_PROXY_BIN="$tmp/observe-bin" VEILKEY_VEILROOT_CGROUP_ROOT="$tmp/sys/fs/cgroup/system.slice" VEILKEY_VEILROOT_WAIT_SECONDS=1 "$observe" codex)"
printf '%s\n' "$obs_out" | grep -q '^observe$'
printf '%s\n' "$obs_out" | grep -qx -- '--cgroup'
printf '%s\n' "$obs_out" | grep -Fqx "$tmp/sys/fs/cgroup/system.slice/veilroot-codex.scope"
printf '%s\n' "$obs_out" | grep -q -- '--enforce-kill'

cat > "$tmp/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
tmpdir="${TMPDIR:-/tmp}"
printf '%s\n' "$@" >> "$tmpdir/systemctl.log"
SCRIPT
chmod +x "$tmp/systemctl"

cat > "$tmp/install-user-boundary.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
test "${VEILKEY_ALLOW_SESSION_BOOTSTRAP:-0}" = "1"
user_name="$1"
config_src="$2"
bin_dir="${VEILKEY_VEILROOT_BIN_DIR:-/usr/local/bin}"
profile_dir="${VEILKEY_VEILROOT_PROFILE_DIR:-/etc/profile.d}"
mkdir -p "$bin_dir" "$(dirname "$config_src")" "$profile_dir"
install -m 0755 deploy/shared/veilkey-session-config "$bin_dir/veilkey-session-config"
SCRIPT
chmod +x "$tmp/install-user-boundary.sh"

cat > "$tmp/visudo" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
test -f "$2"
SCRIPT
chmod +x "$tmp/visudo"

mkdir -p "$tmp/bin" "$tmp/systemd" "$tmp/log" "$tmp/etc/profile.d" "$tmp/etc/sudoers.d" "$tmp/home/veilroot"
VEILKEY_VEILROOT_BIN_DIR="$tmp/bin" \
VEILKEY_VEILROOT_SYSTEMD_DIR="$tmp/systemd" \
VEILKEY_VEILROOT_LOG_DIR="$tmp/log" \
VEILKEY_VEILROOT_PROFILE_DIR="$tmp/etc/profile.d" \
VEILKEY_VEILROOT_SUDOERS_DIR="$tmp/etc/sudoers.d" \
VEILKEY_VEILROOT_HOME_DIR="$tmp/home/veilroot" \
VEILKEY_VEILROOT_SKIP_ACCOUNT_MANAGEMENT=1 \
VEILKEY_INSTALL_USER_BOUNDARY_SCRIPT="$tmp/install-user-boundary.sh" \
SYSTEMCTL_BIN="$tmp/systemctl" \
VISUDO_BIN="$tmp/visudo" \
VEILKEY_SKIP_PACKAGE_INSTALL=1 \
TMPDIR="$tmp" \
  "$install_script" >/dev/null
test -x "$tmp/bin/veilkey-veilroot-session"
test -x "$tmp/bin/veilkey-veilroot-observe"
test -x "$tmp/bin/veilkey-veilroot-egress-guard"
test -f "$tmp/systemd/veilkey-veilroot-observe@.service"
test -f "$tmp/systemd/veilkey-veilroot-egress-guard@.service"
grep -q '^daemon-reload$' "$tmp/systemctl.log"
test -x "$tmp/home/veilroot/.local/bin/hook-veilkey-shell.sh"
test -x "$tmp/home/veilroot/.local/share/veilkey/snippets/veilroot-veilkey-shell.sh"
test -x "$tmp/home/veilroot/.local/bin/curl"
test -x "$tmp/home/veilroot/.local/bin/wget"
test -x "$tmp/home/veilroot/.local/bin/http"
grep -q 'veilroot veilkey shell hook' "$tmp/home/veilroot/.bashrc"
! grep -q 'veilroot codex workspace guard' "$tmp/home/veilroot/.bashrc"

echo "ok: veilroot session"
