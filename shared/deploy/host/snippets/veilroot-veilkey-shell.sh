#!/usr/bin/env bash

_vk_veilroot_workspace="${VEILKEY_VEILROOT_WORKSPACE:-$HOME/workspace}"
_vk_veilroot_locale_lib="${VEILKEY_LOCALE_LIB:-/usr/local/lib/veilkey/veilkey-locale.sh}"

if [[ -f "$_vk_veilroot_locale_lib" ]]; then
  # shellcheck disable=SC1090
  source "$_vk_veilroot_locale_lib"
fi

_vk_veilroot_msg() {
  local key="${1:-}"
  local fallback="${2:-$key}"
  shift 2 || true
  if declare -F vk_msg >/dev/null 2>&1; then
    vk_msg "$key" "$@"
  else
    printf '%s' "$fallback"
  fi
}

_vk_veilroot_require_login_shell() {
  if shopt -q login_shell 2>/dev/null; then
    return 0
  fi
  if [[ -n "${VEILKEY_VEILROOT_BYPASS_NONLOGIN:-}" ]]; then
    return 0
  fi
  echo "$(_vk_veilroot_msg nonlogin_blocked_1 'non-login shell access is not supported.')" >&2
  echo "$(_vk_veilroot_msg nonlogin_blocked_2 \"use 'veilroot' or 'su - veilroot'.\")" >&2
  builtin exit 126
}

_vk_veilroot_is_forbidden_root_path() {
  local path="${1:-}"
  [[ -n "$path" ]] || return 1
  [[ "$path" == "/root" ]] && return 0
  [[ "$path" == /root/* ]] && return 0
  return 1
}

_vk_veilroot_ensure_workspace() {
  if _vk_veilroot_is_forbidden_root_path "${PWD:-}"; then
    builtin cd "$_vk_veilroot_workspace" || return 1
  fi
}

_vk_veilroot_block_http_client() {
  local tool="$1"
  shift || true
  echo "$(_vk_veilroot_msg direct_tool_blocked "direct ${tool} execution is blocked." "$tool")" >&2
  echo "$(_vk_veilroot_msg use_proxy_hint "use 'veilkey proxy ...' for outbound API requests.")" >&2
  return 126
}

curl() {
  _vk_veilroot_block_http_client "curl" "$@"
}

wget() {
  _vk_veilroot_block_http_client "wget" "$@"
}

http() {
  _vk_veilroot_block_http_client "http" "$@"
}

https() {
  _vk_veilroot_block_http_client "https" "$@"
}

cd() {
  local target="${1:-$HOME}"
  if _vk_veilroot_is_forbidden_root_path "$target"; then
    echo "$(_vk_veilroot_msg root_cd_blocked_1 'cd into /root is blocked for veilroot.')" >&2
    echo "$(_vk_veilroot_msg root_cd_blocked_2 "moving to ${_vk_veilroot_workspace}" "$_vk_veilroot_workspace")" >&2
    return 126
  fi
  builtin cd "$@"
}

_vk_veilroot_ensure_workspace

_vk_veilroot_require_login_shell
