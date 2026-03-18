#!/usr/bin/env bash

[ -n "${BASH_VERSION:-}" ] || return 0
[ -n "${PS1:-}" ] || return 0
[ "${VEILKEY_VEIL:-}" = "1" ] || return 0

_vk_veil_prompt_label="${VEILKEY_VEIL_PROMPT_LABEL:?VEILKEY_VEIL_PROMPT_LABEL is required}"
case "${PS1}" in
  *"[${_vk_veil_prompt_label}]"*) ;;
  *)
    PS1="\[\033[1;31m\][${_vk_veil_prompt_label}]\[\033[0m\] ${PS1}"
    ;;
esac

unset _vk_veil_prompt_label
