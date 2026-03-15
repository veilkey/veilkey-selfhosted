#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
user_name="${VEILKEY_VEILROOT_USER:-veilroot}"
if ! [[ "$user_name" =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  echo "invalid user name: $user_name" >&2
  exit 2
fi
config_src="${1:-/etc/veilkey/session-tools.toml}"
bin_dir="${VEILKEY_VEILROOT_BIN_DIR:-/usr/local/bin}"
systemd_dir="${VEILKEY_VEILROOT_SYSTEMD_DIR:-/etc/systemd/system}"
log_dir="${VEILKEY_VEILROOT_LOG_DIR:-/var/log/veilkey-proxy}"
profile_dir="${VEILKEY_VEILROOT_PROFILE_DIR:-/etc/profile.d}"
sudoers_dir="${VEILKEY_VEILROOT_SUDOERS_DIR:-/etc/sudoers.d}"
systemctl_bin="${SYSTEMCTL_BIN:-systemctl}"
visudo_bin="${VISUDO_BIN:-visudo}"
install_user_boundary_script="${VEILKEY_INSTALL_USER_BOUNDARY_SCRIPT:-$repo_root/deploy/host/install-user-boundary.sh}"
home_dir_override="${VEILKEY_VEILROOT_HOME_DIR:-}"
skip_account_management="${VEILKEY_VEILROOT_SKIP_ACCOUNT_MANAGEMENT:-0}"
locale_lib_dir="${VEILKEY_VEILROOT_LIB_DIR:-/usr/local/lib/veilkey}"
locale_lib_path="${VEILKEY_VEILROOT_LOCALE_LIB_PATH:-$locale_lib_dir/veilkey-locale.sh}"
have_user=1

migrate_session_tools() {
  local path="${1:-}"
  [[ -n "$path" ]] || return 0
  [[ -f "$path" ]] || return 0
  if grep -q '^\[dalroot\]$' "$path" && ! grep -q '^\[veilroot\]$' "$path"; then
    perl -0pi -e 's/\[dalroot\]/[veilroot]/g; s/unit_prefix = "dalroot"/unit_prefix = "veilroot"/g' "$path"
  fi
}

cleanup_legacy_dalroot() {
  rm -f \
    "${bin_dir}/veilkey-dalroot-session" \
    "${bin_dir}/veilkey-dalroot-observe" \
    "${bin_dir}/veilkey-dalroot-egress-guard" \
    "${bin_dir}/verify-dalroot-session" \
    "${systemd_dir}/veilkey-dalroot-observe@.service" \
    "${systemd_dir}/veilkey-dalroot-egress-guard@.service" \
    "${profile_dir}/dalroot-workspace.sh" \
    "${profile_dir}/dalroot-veilkey-proxy.sh" \
    "${sudoers_dir}/dalroot"
}

cleanup_legacy_systemd_state() {
  local profile
  for profile in codex claude opencode default; do
    "$systemctl_bin" stop "veilkey-dalroot-observe@${profile}.service" "veilkey-dalroot-egress-guard@${profile}.service" >/dev/null 2>&1 || true
    "$systemctl_bin" disable "veilkey-dalroot-observe@${profile}.service" "veilkey-dalroot-egress-guard@${profile}.service" >/dev/null 2>&1 || true
    "$systemctl_bin" reset-failed "veilkey-dalroot-observe@${profile}.service" "veilkey-dalroot-egress-guard@${profile}.service" >/dev/null 2>&1 || true
  done
  "$systemctl_bin" stop "veilkey-proxy-observe@${user_name}.service" "veilkey-user-egress-guard@${user_name}.service" >/dev/null 2>&1 || true
  "$systemctl_bin" disable "veilkey-proxy-observe@${user_name}.service" "veilkey-user-egress-guard@${user_name}.service" >/dev/null 2>&1 || true
  "$systemctl_bin" reset-failed "veilkey-proxy-observe@${user_name}.service" "veilkey-user-egress-guard@${user_name}.service" >/dev/null 2>&1 || true
}

ensure_tmux() {
  if command -v tmux >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${VEILKEY_SKIP_PACKAGE_INSTALL:-0}" == "1" ]]; then
    echo "tmux is missing (skipped package install by VEILKEY_SKIP_PACKAGE_INSTALL=1)" >&2
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y tmux >/dev/null
  else
    echo "tmux is missing and no supported package manager (apt-get) was found" >&2
  fi
}

if [[ -n "$home_dir_override" ]]; then
  home_dir="$home_dir_override"
  if ! id "$user_name" >/dev/null 2>&1; then
    have_user=0
  fi
elif [[ "$skip_account_management" == "1" ]]; then
  home_dir="/home/${user_name}"
  if id "$user_name" >/dev/null 2>&1; then
    home_dir="$(getent passwd "$user_name" | cut -d: -f6)"
  else
    have_user=0
  fi
else
  if ! id "$user_name" >/dev/null 2>&1; then
    useradd -m -s /bin/bash "$user_name"
  fi
  home_dir="$(getent passwd "$user_name" | cut -d: -f6)"
fi
user_bin_dir="${VEILKEY_VEILROOT_USER_BIN_DIR:-$home_dir/.local/bin}"
hook_path="${VEILKEY_VEILROOT_HOOK_PATH:-$user_bin_dir/hook-veilkey-shell.sh}"
snippet_dir="${VEILKEY_VEILROOT_SNIPPET_DIR:-$home_dir/.local/share/veilkey/snippets}"
snippet_path="${VEILKEY_VEILROOT_SNIPPET_PATH:-$snippet_dir/veilroot-veilkey-shell.sh}"

if [[ "$skip_account_management" != "1" && $have_user -eq 1 ]]; then
  gpasswd -d "$user_name" sudo >/dev/null 2>&1 || true
fi

ensure_tmux
migrate_session_tools "$config_src"
VEILKEY_ALLOW_SESSION_BOOTSTRAP=1 "$install_user_boundary_script" "$user_name" "$config_src"
migrate_session_tools /etc/veilkey/session-tools.toml
cleanup_legacy_dalroot
install -d "$bin_dir" "$systemd_dir" "$log_dir" "$locale_lib_dir"
install -m 0755 "$repo_root/deploy/shared/veilkey-session-config" "$bin_dir/veilkey-session-config"
install -m 0644 "$repo_root/deploy/shared/veilkey-locale.sh" "$locale_lib_path"
install -m 0755 "$repo_root/deploy/host/veilroot-shell" "$bin_dir/veilroot-shell"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-session" "$bin_dir/veilkey-veilroot-session"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-observe" "$bin_dir/veilkey-veilroot-observe"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-egress-guard" "$bin_dir/veilkey-veilroot-egress-guard"
install -m 0755 "$repo_root/deploy/host/verify-veilroot-session.sh" "$bin_dir/verify-veilroot-session"
install -m 0644 "$repo_root/deploy/host/veilkey-veilroot-observe@.service" "$systemd_dir/veilkey-veilroot-observe@.service"
install -m 0644 "$repo_root/deploy/host/veilkey-veilroot-egress-guard@.service" "$systemd_dir/veilkey-veilroot-egress-guard@.service"

install -d "$sudoers_dir" "$profile_dir"

printf '%s\n' "Defaults:${user_name} !authenticate" "${user_name} ALL=(ALL) NOPASSWD: ALL" >"${sudoers_dir}/${user_name}"
chmod 0440 "${sudoers_dir}/${user_name}"
"$visudo_bin" -cf "${sudoers_dir}/${user_name}" >/dev/null

cat >"${profile_dir}/${user_name}-workspace.sh" <<EOF
[ "\${USER:-}" = "$user_name" ] || return 0
export DAL_MEMORY_TARGET_ROOT="\$HOME"
export DAL_MEMORY_STATE_DIR="\$HOME/.local/state/dal-memory"
export DAL_MEMORY_CODEX_SKILLS_DIR="\$HOME/.codex/skills"
export XDG_STATE_HOME="\$HOME/.local/state"
export XDG_CACHE_HOME="\$HOME/.cache"
export XDG_CONFIG_HOME="\$HOME/.config"
EOF
chmod 0644 "${profile_dir}/${user_name}-workspace.sh"

if [[ $have_user -eq 1 ]]; then
  install -d -o "$user_name" -g "$user_name" -m 0755 \
    "$home_dir/workspace" \
    "$home_dir/.local/state/dal-memory" \
    "$home_dir/.local/share/dal-memory/releases" \
    "$snippet_dir" \
    "$home_dir/.codex/skills" \
    "$user_bin_dir" \
    "$home_dir/.cache/codex" \
    "$home_dir/.config/opencode"

  chown -R "$user_name":"$user_name" \
    "$home_dir/.codex" \
    "$home_dir/.cache" \
    "$home_dir/.config" \
    "$home_dir/.local" \
    "$home_dir/workspace"
else
  install -d -m 0755 \
    "$home_dir/workspace" \
    "$home_dir/.local/state/dal-memory" \
    "$home_dir/.local/share/dal-memory/releases" \
    "$snippet_dir" \
    "$home_dir/.codex/skills" \
    "$user_bin_dir" \
    "$home_dir/.cache/codex" \
    "$home_dir/.config/opencode"
fi

cfg_path="$home_dir/.codex/config.toml"
if [[ ! -f "$cfg_path" ]]; then
  if [[ $have_user -eq 1 ]]; then
    install -o "$user_name" -g "$user_name" -m 0644 /dev/null "$cfg_path"
  else
    install -m 0644 /dev/null "$cfg_path"
  fi
fi

install -m 0755 "$repo_root/deploy/host/hook-veilkey-shell.sh" "$hook_path"
install -m 0755 "$repo_root/deploy/host/snippets/veilroot-veilkey-shell.sh" "$snippet_path"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-curl" "$user_bin_dir/curl"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-wget" "$user_bin_dir/wget"
install -m 0755 "$repo_root/deploy/host/veilkey-veilroot-http" "$user_bin_dir/http"
if [[ $have_user -eq 1 ]]; then
  chown "$user_name":"$user_name" \
    "$hook_path" \
    "$snippet_path" \
    "$user_bin_dir/curl" \
    "$user_bin_dir/wget" \
    "$user_bin_dir/http" \
    "$cfg_path"
fi

if ! grep -q 'veilroot veilkey shell hook' "$home_dir/.bashrc" 2>/dev/null; then
  cat >>"$home_dir/.bashrc" <<'EOF'

# veilroot veilkey shell hook
[ -f "$HOME/.local/share/veilkey/snippets/veilroot-veilkey-shell.sh" ] && . "$HOME/.local/share/veilkey/snippets/veilroot-veilkey-shell.sh"
[ -f "$HOME/.local/bin/hook-veilkey-shell.sh" ] && . "$HOME/.local/bin/hook-veilkey-shell.sh"
EOF
fi

if ! grep -q 'default working directory' "$home_dir/.profile" 2>/dev/null; then
  cat >>"$home_dir/.profile" <<'EOF'

# default working directory
if [ -d "$HOME/workspace" ]; then
  cd "$HOME/workspace"
fi
EOF
fi

if [[ "$skip_account_management" != "1" && $have_user -eq 1 ]]; then
  passwd -d "$user_name" >/dev/null || true
  passwd -u "$user_name" >/dev/null 2>&1 || true
fi

if [[ $have_user -eq 1 ]]; then
  chown "$user_name":"$user_name" "$home_dir/.bashrc" "$home_dir/.profile"
fi
"$systemctl_bin" daemon-reload
cleanup_legacy_systemd_state

echo "installed veilroot boundary for ${user_name}"
echo "  account: ${user_name} (sudo ALL enabled)"
echo "  workspace: ${home_dir}/workspace"
echo "  locale: VEILKEY_LOCALE=ko|en (default: LANG)"
echo "  verify: ${bin_dir}/verify-veilroot-session codex"
echo "  optional: $systemctl_bin enable --now veilkey-veilroot-observe@codex.service"
echo "  optional: $systemctl_bin enable --now veilkey-veilroot-egress-guard@codex.service"
