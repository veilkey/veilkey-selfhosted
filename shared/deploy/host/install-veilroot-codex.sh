#!/usr/bin/env bash
set -euo pipefail

user_name="${VEILKEY_VEILROOT_USER:-veilroot}"
real_bin="${VEILKEY_CODEX_REAL_BIN:-/usr/bin/codex}"

if ! id "$user_name" >/dev/null 2>&1; then
  echo "user not found: $user_name" >&2
  exit 1
fi

if [[ ! -x "$real_bin" ]]; then
  echo "codex binary is not executable: $real_bin" >&2
  exit 1
fi

home_dir="$(getent passwd "$user_name" | cut -d: -f6)"
user_bin_dir="${VEILKEY_VEILROOT_USER_BIN_DIR:-$home_dir/.local/bin}"

install -d -o "$user_name" -g "$user_name" -m 0755 "$user_bin_dir" "$home_dir/workspace"

cat >"$user_bin_dir/codex" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
exec /usr/local/bin/veilkey-session-launch codex -C "$HOME/workspace" "$@"
EOF

chmod 0755 "$user_bin_dir/codex"
chown "$user_name":"$user_name" "$user_bin_dir/codex"

echo "installed codex wrapper for ${user_name}"
echo "  wrapper: ${user_bin_dir}/codex"
echo "  real binary: ${real_bin}"
