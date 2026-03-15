#!/usr/bin/env bash

vk_locale() {
  local locale="${VEILKEY_LOCALE:-}"
  if [[ -z "$locale" ]]; then
    locale="${LC_ALL:-${LANG:-en}}"
  fi
  case "${locale,,}" in
    ko|ko_*|ko-*)
      printf '%s' "ko"
      ;;
    *)
      printf '%s' "en"
      ;;
  esac
}

vk_msg() {
  local key="$1"
  local locale
  locale="$(vk_locale)"

  case "$locale:$key" in
    ko:veilroot_connected) printf '%s' "VeilKey: 연결됨" ;;
    en:veilroot_connected) printf '%s' "VeilKey: connected" ;;
    ko:veilroot_disconnected) printf '%s' "VeilKey: 연결 안 됨" ;;
    en:veilroot_disconnected) printf '%s' "VeilKey: disconnected" ;;
    ko:proxy_guard_enabled) printf '%s' "VeilKey Proxy Guard: 활성화" ;;
    en:proxy_guard_enabled) printf '%s' "VeilKey Proxy Guard: enabled" ;;
    ko:proxy_guard_disabled) printf '%s' "VeilKey Proxy Guard: 비활성화" ;;
    en:proxy_guard_disabled) printf '%s' "VeilKey Proxy Guard: disabled" ;;
    ko:observer_connected) printf '%s' "VeilKey Observer: 연결됨" ;;
    en:observer_connected) printf '%s' "VeilKey Observer: connected" ;;
    ko:observer_disconnected) printf '%s' "VeilKey Observer: 연결 안 됨" ;;
    en:observer_disconnected) printf '%s' "VeilKey Observer: disconnected" ;;
    ko:warn_sudo_all_1) printf '%s' "경고: veilroot 는 sudo ALL 권한을 가집니다." ;;
    en:warn_sudo_all_1) printf '%s' "WARNING: veilroot has sudo ALL privileges." ;;
    ko:warn_sudo_all_2) printf '%s' "경고: 모든 작업은 현재 작업자 책임으로 기록됩니다." ;;
    en:warn_sudo_all_2) printf '%s' "WARNING: all actions are attributable to the current operator." ;;
    ko:warn_sudo_all_3) printf '%s' "경고: 의도적인 특권 작업에만 veilroot 를 사용하세요." ;;
    en:warn_sudo_all_3) printf '%s' "WARNING: use veilroot only for intentional privileged operations." ;;
    ko:nonlogin_blocked_1) printf '%s' "[veilkey veilroot] non-login shell 접근은 지원하지 않습니다." ;;
    en:nonlogin_blocked_1) printf '%s' "[veilkey veilroot] non-login shell access is not supported." ;;
    ko:nonlogin_blocked_2) printf '%s' "[veilkey veilroot] 'veilroot' 또는 'su - veilroot' 를 사용하세요." ;;
    en:nonlogin_blocked_2) printf '%s' "[veilkey veilroot] use 'veilroot' or 'su - veilroot'." ;;
    ko:direct_tool_blocked) printf '[veilkey veilroot] %s 직접 실행은 차단됩니다.' "${2:-도구}" ;;
    en:direct_tool_blocked) printf '[veilkey veilroot] direct %s is blocked.' "${2:-tool}" ;;
    ko:use_proxy_hint) printf '%s' "[veilkey veilroot] 외부 API 요청은 'veilkey proxy ...' 를 사용하세요." ;;
    en:use_proxy_hint) printf '%s' "[veilkey veilroot] use 'veilkey proxy ...' for outbound API requests." ;;
    ko:root_cd_blocked_1) printf '%s' "[veilkey veilroot] /root 로의 cd 는 차단됩니다." ;;
    en:root_cd_blocked_1) printf '%s' "[veilkey veilroot] cd into /root is blocked." ;;
    ko:root_cd_blocked_2) printf '[veilkey veilroot] 대신 %s 를 사용하세요.' "${2:-$HOME/workspace}" ;;
    en:root_cd_blocked_2) printf "[veilkey veilroot] use %s instead." "${2:-$HOME/workspace}" ;;
    ko:blocked_sensitive_path_1) printf '[veilkey veilroot] 민감 경로 접근이 차단되었습니다: %s' "${2:-}" ;;
    en:blocked_sensitive_path_1) printf '[veilkey veilroot] blocked sensitive path access: %s' "${2:-}" ;;
    ko:blocked_sensitive_path_2) printf '%s' "[veilkey veilroot] 비밀 경로 직접 접근 대신 VeilKey 관리 경로를 사용하세요." ;;
    en:blocked_sensitive_path_2) printf '%s' "[veilkey veilroot] use VeilKey-managed flows instead of direct secret file access." ;;
    ko:blocked_credential_helper_output) printf '%s' "[veilkey veilroot] credential helper 출력은 직접 복사하지 말고 git/helper 경계 안에서만 사용하세요." ;;
    en:blocked_credential_helper_output) printf '%s' "[veilkey veilroot] do not print credential helper output directly; let git call the helper or use a wrapped workflow." ;;
    ko:blocked_http_client_1) printf '[veilkey veilroot] 직접 HTTP 클라이언트 명령이 차단되었습니다: %s' "${2:-}" ;;
    en:blocked_http_client_1) printf '[veilkey veilroot] blocked direct HTTP client command: %s' "${2:-}" ;;
    ko:blocked_sensitive_api_1) printf '[veilkey veilroot] 민감 API/토큰 패턴이 차단되었습니다: %s' "${2:-}" ;;
    en:blocked_sensitive_api_1) printf '[veilkey veilroot] blocked sensitive API/token pattern: %s' "${2:-}" ;;
    ko:blocked_sensitive_api_2) printf '%s' "[veilkey veilroot] VK:{SCOPE}:{REF} ref 는 반드시 'veilkey proxy' 경계에서만 사용하세요." ;;
    en:blocked_sensitive_api_2) printf '%s' "[veilkey veilroot] use VK:{SCOPE}:{REF} refs through 'veilkey proxy' only." ;;
    *)
      printf '%s' "$key"
      ;;
  esac
}
