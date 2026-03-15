#!/usr/bin/env bash

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

_vk_veilroot_should_skip_preexec() {
  local command="$1"
  local prompt_command="${2:-}"
  [[ "${command}" == "${prompt_command}" ]] && return 0
  [[ "${command}" == "echo -ne "* ]] && return 0
  [[ "${command}" == "_vk_veilroot_preexec "* ]] && return 0
  [[ "${command}" == "_vk_veilroot_preexec_impl "* ]] && return 0
  return 1
}

_vk_veilroot_is_sensitive_path() {
  local command="$1"
  [[ "${command}" =~ (^|[[:space:][:punct:]])\.env($|[[:space:][:punct:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])\.env\.[^[:space:]]+ ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])secrets(/|$) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])credentials(/|$) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])\.git-credentials($|[[:space:][:punct:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])[^[:space:]]+\.pem($|[[:space:][:punct:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])[^[:space:]]+\.key($|[[:space:][:punct:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])git[[:space:]]+credential[[:space:]]+(fill|approve|reject)($|[[:space:][:punct:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:][:punct:]])git-credential[[:space:]]+(get|store|erase)($|[[:space:][:punct:]]) ]] && return 0
  return 1
}

_vk_veilroot_is_direct_http_client() {
  local command="$1"
  [[ "${command}" =~ (^|[[:space:]])(/usr/bin/)?curl($|[[:space:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:]])(/usr/bin/)?wget($|[[:space:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:]])(/usr/bin/)?http($|[[:space:]]) ]] && return 0
  [[ "${command}" =~ (^|[[:space:]])(/usr/bin/)?https($|[[:space:]]) ]] && return 0
  return 1
}

_vk_veilroot_has_sensitive_api_pattern() {
  local command="$1"
  [[ "${command}" =~ PRIVATE-TOKEN[[:space:]]*: ]] && return 0
  [[ "${command}" =~ Authorization[[:space:]]*:[[:space:]]*Bearer ]] && return 0
  [[ "${command}" =~ [Xx]-API-[Kk]ey[[:space:]]*: ]] && return 0
  [[ "${command}" =~ (^|[[:space:]])api[-_]key= ]] && return 0
  [[ "${command}" =~ VK:(TEMP|LOCAL|EXTERNAL):[0-9A-Fa-f]{4,64} ]] && return 0
  return 1
}

_vk_veilroot_preexec_impl() {
  local command="$1"
  local prompt_command="${2:-}"

  _vk_veilroot_should_skip_preexec "${command}" "${prompt_command}" && return 0
  [[ "${VEILKEY_VEILROOT_GUARD_RUNNING:-0}" == "1" ]] && return 0

  if _vk_veilroot_is_sensitive_path "${command}"; then
    echo "$(_vk_veilroot_msg blocked_sensitive_path_1 "blocked sensitive path access: ${command}" "${command}")" >&2
    echo "$(_vk_veilroot_msg blocked_sensitive_path_2 'move the secret behind a VK ref or guarded workflow first.')" >&2
    if [[ "${command}" =~ (^|[[:space:][:punct:]])git[[:space:]]+credential[[:space:]]+(fill|approve|reject)($|[[:space:][:punct:]]) || "${command}" =~ (^|[[:space:][:punct:]])git-credential[[:space:]]+(get|store|erase)($|[[:space:][:punct:]]) ]]; then
      echo "$(_vk_veilroot_msg blocked_credential_helper_output 'do not print credential helper output directly; let git call the helper or use a wrapped workflow.')" >&2
    fi
    return 1
  fi

  if _vk_veilroot_is_direct_http_client "${command}"; then
    echo "$(_vk_veilroot_msg blocked_http_client_1 "blocked direct HTTP client command: ${command}" "${command}")" >&2
    echo "$(_vk_veilroot_msg use_proxy_hint "use 'veilkey proxy ...' for outbound API requests.")" >&2
    return 1
  fi

  if _vk_veilroot_has_sensitive_api_pattern "${command}"; then
    echo "$(_vk_veilroot_msg blocked_sensitive_api_1 "blocked sensitive API usage in command: ${command}" "${command}")" >&2
    echo "$(_vk_veilroot_msg blocked_sensitive_api_2 'use veilkey proxy or a wrapped tool instead of direct headers/tokens.')" >&2
    return 1
  fi

  return 0
}

_vk_veilroot_preexec() {
  local command="$1"
  local prompt_command="${PROMPT_COMMAND:-}"
  VEILKEY_VEILROOT_GUARD_RUNNING=1 _vk_veilroot_preexec_impl "${command}" "${prompt_command}"
}

_vk_veilroot_block_credential_helper_output() {
  local command="${1:-git credential fill}"
  echo "$(_vk_veilroot_msg blocked_sensitive_path_1 "blocked sensitive path access: ${command}" "${command}")" >&2
  echo "$(_vk_veilroot_msg blocked_credential_helper_output 'do not print credential helper output directly; let git call the helper or use a wrapped workflow.')" >&2
  return 126
}

_vk_veilroot_git_wrapper() {
  if [[ "${1:-}" == "credential" ]] && [[ "${2:-}" =~ ^(fill|approve|reject)$ ]]; then
    _vk_veilroot_block_credential_helper_output "git credential ${2}"
    return 126
  fi
  command git "$@"
}

_vk_veilroot_git_credential_wrapper() {
  if [[ "${1:-}" =~ ^(get|store|erase)$ ]]; then
    _vk_veilroot_block_credential_helper_output "git-credential ${1}"
    return 126
  fi
  command git-credential "$@"
}

git() {
  _vk_veilroot_git_wrapper "$@"
}

alias git-credential='_vk_veilroot_git_credential_wrapper'

if [[ $- == *i* ]]; then
  shopt -s extdebug
  trap '_vk_veilroot_preexec "$BASH_COMMAND"' DEBUG
fi
