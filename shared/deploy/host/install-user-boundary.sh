#!/usr/bin/env bash
set -euo pipefail

user_name="${1:-}"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
default_config_src="/etc/veilkey/session-tools.toml"
if [[ ! -f "$default_config_src" ]]; then
  default_config_src="$(dirname "$0")/session-tools.toml.example"
fi
config_src="${2:-$default_config_src}"
config_dst="/etc/veilkey/session-tools.toml"

if [[ -z "$user_name" ]]; then
  echo "usage: $(basename "$0") <user> [config-src]" >&2
  exit 2
fi
if ! [[ "$user_name" =~ ^[a-z_][a-z0-9_-]*$ ]]; then
  echo "invalid user name: $user_name" >&2
  exit 2
fi

if [[ "${VEILKEY_ALLOW_SESSION_BOOTSTRAP:-0}" != "1" ]]; then
  echo "$(basename "$0") is an internal session bootstrap helper; use install-veilroot-boundary.sh for supported host boundary setup" >&2
  exit 1
fi

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

ensure_go() {
  if command -v go >/dev/null 2>&1; then
    return 0
  fi
  if [[ "${VEILKEY_SKIP_PACKAGE_INSTALL:-0}" == "1" ]]; then
    echo "go is missing (skipped package install by VEILKEY_SKIP_PACKAGE_INSTALL=1)" >&2
    exit 1
  fi
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y golang-go >/dev/null
    return 0
  fi
  echo "go is missing and no supported package manager (apt-get) was found" >&2
  exit 1
}

id "$user_name" >/dev/null 2>&1 || { echo "unknown user: $user_name" >&2; exit 1; }

home_dir="$(getent passwd "$user_name" | cut -d: -f6)"
uid="$(id -u "$user_name")"

install -d /etc/veilkey /var/log/veilkey-proxy "$home_dir/.local/bin"
if [[ "$config_src" != "$config_dst" ]]; then
  install -m 0644 "$config_src" "$config_dst"
fi
ensure_go
session_config_tmp="$(mktemp)"
go build -o "$session_config_tmp" "$repo_root/cmd/veilkey-session-config"
install -m 0755 "$session_config_tmp" /usr/local/bin/veilkey-session-config
rm -f "$session_config_tmp"
ensure_tmux

cat >/usr/local/bin/veilkey-session-launch <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
if [[ $# -lt 1 ]]; then
  echo "usage: veilkey-session-launch <tool> [args...]" >&2
  exit 2
fi
tool="$1"
shift
real_bin="$(/usr/local/bin/veilkey-session-config tool-bin "$tool")" || {
  echo "unknown or invalid tool mapping: ${tool}" >&2
  exit 2
}
if [[ ! -x "$real_bin" ]]; then
  echo "mapped binary is not executable: $real_bin" >&2
  exit 2
fi
while IFS='=' read -r _vk_key _vk_val; do
  _vk_key="${_vk_key#export }"
  [[ "$_vk_key" =~ ^[A-Za-z_][A-Za-z_0-9]*$ ]] || continue
  _vk_val="${_vk_val%\"}" ; _vk_val="${_vk_val#\"}"
  export "${_vk_key}=${_vk_val}"
done < <(/usr/local/bin/veilkey-session-config tool-shell-exports "$tool")
if [[ "${VEILKEY_VERIFIED_SESSION:-}" == "1" || "${VEILKEY_VEILROOT:-}" == "1" || "${VEILKEY_ACTIVE:-}" == "1" ]]; then
  exec "$real_bin" "$@"
fi
exec /usr/local/bin/veilkey session "$real_bin" "$@"
SCRIPT
chmod 0755 /usr/local/bin/veilkey-session-launch

cat >/etc/profile.d/${user_name}-veilkey-proxy.sh <<SCRIPT
[ "\${USER:-}" = "$user_name" ] || return 0
while IFS='=' read -r _vk_key _vk_val; do
  _vk_key="\${_vk_key#export }"
  [[ "\$_vk_key" =~ ^[A-Za-z_][A-Za-z_0-9]*$ ]] || continue
  _vk_val="\${_vk_val%\"}" ; _vk_val="\${_vk_val#\"}"
  export "\${_vk_key}=\${_vk_val}"
done < <(/usr/local/bin/veilkey-session-config shell-exports)
export VEILKEY_PROXY_STATE=active
SCRIPT
chmod 0644 /etc/profile.d/${user_name}-veilkey-proxy.sh

for tool in codex claude opencode; do
  cat >"$home_dir/.local/bin/${tool}" <<SCRIPT
#!/usr/bin/env bash
set -euo pipefail
exec /usr/local/bin/veilkey-session-launch ${tool} "\$@"
SCRIPT
  chmod 0755 "$home_dir/.local/bin/${tool}"
done

cat >"$home_dir/.bash_profile" <<SCRIPT
# ${user_name} login boundary
[ -f ~/.profile ] && . ~/.profile
while IFS='=' read -r _vk_key _vk_val; do
  _vk_key="\${_vk_key#export }"
  [[ "\$_vk_key" =~ ^[A-Za-z_][A-Za-z_0-9]*$ ]] || continue
  _vk_val="\${_vk_val%\"}" ; _vk_val="\${_vk_val#\"}"
  export "\${_vk_key}=\${_vk_val}"
done < <(/usr/local/bin/veilkey-session-config shell-exports)
alias codex="\$HOME/.local/bin/codex"
alias claude="\$HOME/.local/bin/claude"
alias opencode="\$HOME/.local/bin/opencode"
SCRIPT

chown -R "${user_name}:${user_name}" "$home_dir/.config" "$home_dir/.local" "$home_dir/.bash_profile"
systemctl daemon-reload
su - "$user_name" -c "tmux has-session -t main >/dev/null 2>&1 || tmux new-session -d -s main" >/dev/null 2>&1 || true
echo "installed veilkey boundary for ${user_name}"
