#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

hook="$PWD/deploy/host/hook-veilkey-shell.sh"
snippet="$PWD/deploy/host/snippets/veilroot-veilkey-shell.sh"
locale_lib="$PWD/deploy/shared/veilkey-locale.sh"

run_hook() {
  local script="$1"
  BASH_ENV=/dev/null VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$hook'; $script" 2>&1
}

out="$(run_hook "_vk_veilroot_preexec_impl 'curl https://example.com' ''; printf 'rc=%s\n' \$?")"
printf '%s\n' "$out" | grep -q 'blocked direct HTTP client command'
printf '%s\n' "$out" | grep -q 'rc=1'

out="$(run_hook "_vk_veilroot_preexec_impl '/usr/bin/curl -H \"PRIVATE-TOKEN: abc\" https://gitlab.example' ''; printf 'rc=%s\n' \$?")"
printf '%s\n' "$out" | grep -q 'blocked direct HTTP client command'
printf '%s\n' "$out" | grep -q 'rc=1'

out="$(run_hook "_vk_veilroot_preexec_impl 'cat .env' ''; printf 'rc=%s\n' \$?")"
printf '%s\n' "$out" | grep -q 'blocked sensitive path access'
printf '%s\n' "$out" | grep -q 'rc=1'

mockbin="$tmp/mockbin"
mkdir -p "$mockbin" "$tmp/profile.d"

cat >"$mockbin/veilkey" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "resolve" ]]; then
  case "${2:-}" in
    VK:LOCAL:testref) printf '%s' 'demo secret value' ;;
    VE:LOCAL:testenv) printf '%s' 'resolved env value' ;;
    *) exit 1 ;;
  esac
  exit 0
fi
exit 1
EOF
chmod +x "$mockbin/veilkey"

rewritten="$(PATH="$mockbin:$PATH" BASH_ENV=/dev/null VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$hook'; _vk_veilroot_rewrite_command 'printf %s VK:LOCAL:testref'")"
printf '%s\n' "$rewritten" | grep -q "printf %s demo\\\\ secret\\\\ value"

out="$(PATH="$mockbin:$PATH" BASH_ENV=/dev/null VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$hook'; _vk_veilroot_preexec_impl 'printf \"%s\\n\" VK:LOCAL:testref' ''; printf 'rc=%s\n' \$?" 2>&1)"
printf '%s\n' "$out" | grep -q '^demo secret value$'
printf '%s\n' "$out" | grep -q 'rc=1'

out="$(PATH="$mockbin:$PATH" BASH_ENV=/dev/null VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$hook'; _vk_veilroot_preexec_impl 'VAR=VE:LOCAL:testenv env | grep ^VAR=' ''; printf 'rc=%s\n' \$?" 2>&1)"
printf '%s\n' "$out" | grep -q '^VAR=resolved env value$'
printf '%s\n' "$out" | grep -q 'rc=1'

TEST_GITLAB_HOST="gitlab.test.internal"

out="$(run_hook "_vk_veilroot_preexec_impl 'printf protocol=https\\nhost=${TEST_GITLAB_HOST}\\n\\n | git credential fill' ''; printf 'rc=%s\n' \$?")"
printf '%s\n' "$out" | grep -q 'blocked sensitive path access'
printf '%s\n' "$out" | grep -q 'credential helper output'
printf '%s\n' "$out" | grep -q 'rc=1'

out="$(BASH_ENV=/dev/null VEILKEY_LOCALE_LIB="$locale_lib" bash --noprofile --norc -ic "source '$hook'; printf 'protocol=https\\nhost=${TEST_GITLAB_HOST}\\n\\n' | git credential fill" 2>&1 || true)"
printf '%s\n' "$out" | grep -q 'blocked sensitive path access'
printf '%s\n' "$out" | grep -q 'credential helper output'
if printf '%s\n' "$out" | grep -q '^password='; then
  echo "interactive veilroot hook leaked credential helper password output" >&2
  exit 1
fi

echo "ok: veilroot shell hook"

out="$(BASH_ENV=/dev/null VEILKEY_VEILROOT_BYPASS_NONLOGIN=1 VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$snippet'; curl https://example.com" 2>&1 || true)"
printf '%s\n' "$out" | grep -q 'direct curl is blocked'

out="$(BASH_ENV=/dev/null VEILKEY_VEILROOT_BYPASS_NONLOGIN=1 VEILKEY_LOCALE=ko VEILKEY_LOCALE_LIB="$locale_lib" bash -c "source '$snippet'; curl https://example.com" 2>&1 || true)"
printf '%s\n' "$out" | grep -q '직접 실행은 차단됩니다'
printf '%s\n' "$out" | grep -q "외부 API 요청은 'veilkey proxy ...' 를 사용하세요"

out="$(HOME="$tmp/home" BASH_ENV=/dev/null VEILKEY_VEILROOT_BYPASS_NONLOGIN=1 VEILKEY_LOCALE_LIB="$locale_lib" bash -c 'mkdir -p "$HOME/workspace"; source "'"$snippet"'"; cd /root' 2>&1 || true)"
printf '%s\n' "$out" | grep -q 'cd into /root is blocked'

out="$(HOME="$tmp/home" BASH_ENV=/dev/null VEILKEY_VEILROOT_BYPASS_NONLOGIN=1 VEILKEY_LOCALE_LIB="$locale_lib" bash -c 'mkdir -p "$HOME/workspace"; cd /root 2>/dev/null || true; source "'"$snippet"'"; pwd' 2>&1 || true)"
printf '%s\n' "$out" | grep -q "$tmp/home/workspace"
printf '%s\n' "$out" | grep -vq '/root is not a valid working directory'

echo "ok: veilroot shell snippet"

cat >"$mockbin/systemctl" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  *'list-unit-files veilkey-veilroot-observe@.service'*) exit 0 ;;
  *'list-unit-files veilkey-veilroot-egress-guard@.service'*) exit 0 ;;
  *'--quiet is-active veilkey-veilroot-observe@codex.service'*) exit 0 ;;
  *'--quiet is-active veilkey-veilroot-egress-guard@codex.service'*) exit 0 ;;
  *'is-active veilkey-veilroot-observe@codex.service'*) echo active ;;
  *'is-active veilkey-veilroot-egress-guard@codex.service'*) echo active ;;
  *'is-enabled veilkey-veilroot-observe@codex.service'*) echo enabled ;;
  *'is-enabled veilkey-veilroot-egress-guard@codex.service'*) echo enabled ;;
  *) exit 1 ;;
esac
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
  *)
    exit 1
    ;;
esac
EOF
chmod +x "$mockbin/systemctl" "$mockbin/curl" "$mockbin/session-config"
touch "$tmp/profile.d/veilroot-veilkey-proxy.sh"

out="$(PATH="$mockbin:$PATH" VEILKEY_LOCALE=ko VEILKEY_LOCALE_LIB="$locale_lib" VEILKEY_SESSION_CONFIG_BIN="$mockbin/session-config" VEILKEY_VEILROOT_PROFILE_ACTIVE="$tmp/profile.d/veilroot-veilkey-proxy.sh" bash ./deploy/host/veilroot-shell status 2>&1)"
printf '%s\n' "$out" | grep -q 'VeilKey: 연결됨'
printf '%s\n' "$out" | grep -q 'VeilKey Proxy Guard: 활성화'
printf '%s\n' "$out" | grep -q 'VeilKey Observer: 연결됨'

echo "ok: veilroot shell status locale"

out="$(BASH_ENV=/dev/null VEILKEY_VEILROOT_BYPASS_NONLOGIN=1 VEILKEY_LOCALE_LIB="$tmp/missing-locale.sh" bash -c "source '$snippet'; curl https://example.com" 2>&1 || true)"
printf '%s\n' "$out" | grep -q 'direct curl'
printf '%s\n' "$out" | grep -q "use 'veilkey proxy ...' for outbound API requests"

out="$(PATH="$mockbin:$PATH" VEILKEY_LOCALE=ko VEILKEY_LOCALE_LIB="$tmp/missing-locale.sh" VEILKEY_SESSION_CONFIG_BIN="$mockbin/session-config" VEILKEY_VEILROOT_PROFILE_ACTIVE="$tmp/profile.d/veilroot-veilkey-proxy.sh" bash ./deploy/host/veilroot-shell status 2>&1)"
printf '%s\n' "$out" | grep -q 'VeilKey: connected'
printf '%s\n' "$out" | grep -q 'VeilKey Proxy Guard: enabled'
printf '%s\n' "$out" | grep -q 'VeilKey Observer: connected'

echo "ok: veilroot locale fallback"
