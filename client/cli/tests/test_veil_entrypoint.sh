#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
. tests/lib/testlib.sh

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/veilkey-session-config" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  shell-exports)
    cat <<'OUT'
export VEILKEY_LOCALVAULT_URL='http://127.0.0.1:10180'
export VEILKEY_PLAINTEXT_ACTION='issue-temp-and-block'
OUT
    ;;
  *)
    exit 1
    ;;
esac
SCRIPT
chmod +x "$tmp/veilkey-session-config"

cat >"$tmp/veilkey" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-}"
if [[ -n "${VEIL_TEST_ARGS:-}" ]]; then
  printf 'args=%s\n' "$*" > "${VEIL_TEST_ARGS}"
fi
{
  printf 'VEILKEY_LOCALVAULT_URL=%s\n' "${VEILKEY_LOCALVAULT_URL:-}"
  printf 'VEILKEY_PLAINTEXT_ACTION=%s\n' "${VEILKEY_PLAINTEXT_ACTION:-}"
  printf 'VEILKEY_VEIL=%s\n' "${VEILKEY_VEIL:-}"
} > "${VEIL_TEST_ENV:-/dev/null}"
case "$cmd" in
  status)
    printf 'Veil:    active\n'
    printf 'Paste:   on\n'
    ;;
  paste-mode)
    shift
    exec "${VEILKEY_CLI_BIN}" paste-mode "$@"
    ;;
esac
SCRIPT
chmod +x "$tmp/veilkey"

cat >"$tmp/veilkey-cli" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
printf 'cli-args=%s\n' "$*" > "${VEIL_TEST_CLI_ARGS}"
SCRIPT
chmod +x "$tmp/veilkey-cli"

args_log="$tmp/veil.args"
env_log="$tmp/veil.env"
cli_args_log="$tmp/veil.cli.args"
VEILKEY_SESSION_CONFIG_BIN="$tmp/veilkey-session-config" \
VEILKEY_BIN="$tmp/veilkey" \
VEILKEY_CLI_BIN="$tmp/veilkey-cli" \
VEIL_TEST_ARGS="$args_log" \
VEIL_TEST_ENV="$env_log" \
  bash ./deploy/host/veil

assert_contains "$(cat "$args_log")" "args=session bash -li"
assert_contains "$(cat "$env_log")" "VEILKEY_LOCALVAULT_URL=http://127.0.0.1:10180"
assert_contains "$(cat "$env_log")" "VEILKEY_PLAINTEXT_ACTION=issue-temp-and-block"
assert_contains "$(cat "$env_log")" "VEILKEY_VEIL=1"

VEILKEY_SESSION_CONFIG_BIN="$tmp/veilkey-session-config" \
VEILKEY_BIN="$tmp/veilkey" \
VEILKEY_CLI_BIN="$tmp/veilkey-cli" \
VEIL_TEST_ARGS="$args_log" \
VEIL_TEST_ENV="$env_log" \
  bash ./deploy/host/veil >/dev/null

out_invalid="$(
  VEILKEY_SESSION_CONFIG_BIN="$tmp/veilkey-session-config" \
  VEILKEY_BIN="$tmp/veilkey" \
  VEILKEY_CLI_BIN="$tmp/veilkey-cli" \
    bash ./deploy/host/veil codex --version 2>&1 || true
)"
assert_contains "$out_invalid" "direct app launch is not supported"

out_status="$(
  VEILKEY_SESSION_CONFIG_BIN="$tmp/veilkey-session-config" \
  VEILKEY_BIN="$tmp/veilkey" \
  VEILKEY_CLI_BIN="$tmp/veilkey-cli" \
    bash ./deploy/host/veil status
)"
assert_contains "$out_status" "Veil:    active"
assert_contains "$out_status" "Paste:   on"

VEILKEY_SESSION_CONFIG_BIN="$tmp/veilkey-session-config" \
VEILKEY_BIN="$tmp/veilkey" \
VEILKEY_CLI_BIN="$tmp/veilkey-cli" \
VEIL_TEST_CLI_ARGS="$cli_args_log" \
  bash ./deploy/host/veil paste-mode off

assert_contains "$(cat "$cli_args_log")" "cli-args=paste-mode off"

out="$(
  VEILKEY_SESSION_CONFIG_BIN="$tmp/missing-session-config" \
    bash ./deploy/host/veil 2>&1 || true
)"
assert_contains "$out" "required session config binary not found"

echo "ok: veil entrypoint"
