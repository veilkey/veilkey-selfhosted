#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mockbin="$tmp/mockbin"
mkdir -p "$mockbin" "$tmp/profile.d"

cat >"$mockbin/systemctl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$mockbin/curl" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
cat >"$mockbin/session-config" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  veilroot-default-profile) echo codex ;;
  tool-shell-exports)
    cat <<'OUT'
export VEILKEY_KEYCENTER_URL='http://127.0.0.1:10180'
OUT
    ;;
  *) exit 1 ;;
esac
EOF
cat >"$mockbin/veilkey" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" > "${VEILKEY_WRAP_PTY_LOG}"
EOF
chmod +x "$mockbin/systemctl" "$mockbin/curl" "$mockbin/session-config" "$mockbin/veilkey"
touch "$tmp/profile.d/veilroot-veilkey-proxy.sh"

wrap_log="$tmp/veilkey.args"
PATH="$mockbin:$PATH" \
VEILROOT_USER="$(id -un)" \
VEILKEY_BIN="$mockbin/veilkey" \
VEILKEY_WRAP_PTY_LOG="$wrap_log" \
VEILKEY_SESSION_CONFIG_BIN="$mockbin/session-config" \
VEILKEY_VEILROOT_PROFILE_ACTIVE="$tmp/profile.d/veilroot-veilkey-proxy.sh" \
  bash ./deploy/host/veilroot-shell open codex >/dev/null

grep -Fx 'wrap-pty bash -li' "$wrap_log" >/dev/null

echo "ok: veilroot-shell open uses wrap-pty"
