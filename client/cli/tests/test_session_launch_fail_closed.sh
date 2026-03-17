#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/fakebin" "$tmp/home" "$tmp/etc"

cat >"$tmp/session-tools.toml" <<'SCRIPT'
version = 1
SCRIPT

cat >"$tmp/fakebin/id" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "-u" ]]; then
  printf '%s\n' "1234"
  exit 0
fi
exit 0
SCRIPT
chmod +x "$tmp/fakebin/id"

cat >"$tmp/fakebin/getent" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "passwd" ]]; then
  printf 'veiltest:x:1234:1234::%s:/bin/bash\n' "${VEILKEY_TEST_HOME:?}"
  exit 0
fi
exit 1
SCRIPT
chmod +x "$tmp/fakebin/getent"

cat >"$tmp/fakebin/chown" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
exit 0
SCRIPT
chmod +x "$tmp/fakebin/chown"

cat >"$tmp/fakebin/systemctl" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
exit 0
SCRIPT
chmod +x "$tmp/fakebin/systemctl"

cat >"$tmp/stub-session-config" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  tool-bin)
    printf '%s\n' "${VEILKEY_REAL_TOOL:?}"
    ;;
  tool-shell-exports)
    exit 0
    ;;
  shell-exports)
    exit 0
    ;;
  *)
    exit 2
    ;;
esac
SCRIPT
chmod +x "$tmp/stub-session-config"

cat >"$tmp/real-tool" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
echo "real-tool:$*"
SCRIPT
chmod +x "$tmp/real-tool"

VEILKEY_ALLOW_SESSION_BOOTSTRAP=1 \
VEILKEY_SKIP_PACKAGE_INSTALL=1 \
VEILKEY_SKIP_SESSION_PRIME=1 \
VEILKEY_TEST_HOME="$tmp/home" \
VEILKEY_SESSION_CONFIG_DST="$tmp/etc/veilkey/session-tools.toml" \
VEILKEY_SESSION_CONFIG_INSTALL_BIN="$tmp/fakebin/veilkey-session-config" \
VEILKEY_SESSION_LAUNCH_BIN="$tmp/fakebin/veilkey-session-launch" \
VEILKEY_PROFILE_DIR="$tmp/etc/profile.d" \
VEILKEY_PROXY_LOG_DIR="$tmp/log" \
PATH="$tmp/fakebin:$PATH" \
deploy/host/install-user-boundary.sh veiltest "$tmp/session-tools.toml" >/dev/null

cp "$tmp/stub-session-config" "$tmp/fakebin/veilkey-session-config"

out="$(VEILKEY_SESSION_CONFIG_BIN="$tmp/fakebin/veilkey-session-config" VEILKEY_ACTIVE=1 VEILKEY_REAL_TOOL="$tmp/real-tool" "$tmp/fakebin/veilkey-session-launch" codex 2>&1 || true)"
assert_contains "$out" "refusing direct exec without a verified Veil session boundary"

out="$(VEILKEY_SESSION_CONFIG_BIN="$tmp/fakebin/veilkey-session-config" VEILKEY_VEILROOT=1 VEILKEY_REAL_TOOL="$tmp/real-tool" "$tmp/fakebin/veilkey-session-launch" codex hello)"
assert_contains "$out" "real-tool:hello"

echo "ok: session launch fail closed"
