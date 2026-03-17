#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MANIFEST_FILE="${VEILKEY_INSTALLER_MANIFEST:-${ROOT_DIR}/components.toml}"
GO_BIN="${GO_BIN:-go}"
OS_MODULE_DIR="${ROOT_DIR}/scripts/os"
STAGE_DEFAULT_ROOT="${ROOT_DIR}/stage"
BUNDLE_DEFAULT_ROOT="${ROOT_DIR}/bundle"
PROFILE_DIR="${ROOT_DIR}/profiles"

usage() {
  cat <<'EOF'
Usage: ./install.sh <command> [options]

Commands:
  init                  Create components.toml from canonical component pins
  validate              Validate manifest structure
  doctor                Validate manifest and report legacy layout
  detect-os             Detect installer OS family and install paths
  list                  List components from manifest
  profiles              List install profiles
  plan [profile]        Show install order for a profile
  plan-stage [profile]
                       Show staged install metadata for a profile
  plan-download [profile]
                       Show artifact download URLs for a profile
  plan-install [profile] [root]
                       Show resolved install layout for a profile
  download [profile] [dest]
                       Download component artifacts for a profile
  stage [profile] [root]
                       Create staged install layout for a profile
  bundle [profile] [root]
                       Create stage plus downloads directory for a profile
  install [profile] [root] [bundle_root]
                       Install a bundled profile into root using OS modules
  configure [profile] [root]
                       Render profile env files into installed root
  install-profile [profile] [root]
                       Bundle, install, and configure a profile in one step
  plan-activate [root]
                       Show which services would be enabled/started
  activate [root]
                       Enable/start configured services on a live system
  post-install-health [root]
                       Run post-install scaffold checks inside root
  print-json            Print manifest as JSON

Environment:
  VEILKEY_INSTALLER_MANIFEST   Override manifest path
  GO_BIN                       Go binary used for manifest parsing helper
  VEILKEY_INSTALLER_OS_FAMILY  Override detected OS family
  VEILKEY_INSTALLER_AUTH_HEADER Custom curl auth header for package download
  VEILKEY_GITLAB_PACKAGE_PAT   PRIVATE-TOKEN used for package download
  VEILKEY_INSTALLER_GITLAB_API_BASE Override GitLab API base for package URL normalization
EOF
}

require_go() {
  local required_go_version current_go_bin installed_ok=0
  current_go_bin="${GO_BIN}"
  required_go_version="$(awk '/^toolchain /{print $2; exit} /^go /{print "go"$2; exit}' "${ROOT_DIR}/go.mod")"

  if command -v "${current_go_bin}" >/dev/null 2>&1; then
    if python3 - "${current_go_bin}" "${required_go_version}" <<'PY'
import re
import subprocess
import sys

go_bin = sys.argv[1]
required = sys.argv[2]

def norm(value: str):
    m = re.search(r'go(\d+)\.(\d+)(?:\.(\d+))?', value)
    if not m:
        return None
    return tuple(int(part or 0) for part in m.groups())

try:
    version = subprocess.check_output([go_bin, "version"], text=True, stderr=subprocess.DEVNULL)
except Exception:
    raise SystemExit(1)

current = norm(version)
target = norm(required)
if current is None or target is None:
    raise SystemExit(1)

raise SystemExit(0 if current >= target else 1)
PY
    then
      GO_BIN="${current_go_bin}"
      return 0
    fi
  fi

  if [[ "${VEILKEY_SKIP_PACKAGE_INSTALL:-0}" != "1" ]]; then
    if command -v apt-get >/dev/null 2>&1; then
      export DEBIAN_FRONTEND=noninteractive
      apt-get update >/dev/null
      apt-get install -y golang-go libsqlcipher1 >/dev/null
    elif command -v dnf >/dev/null 2>&1; then
      dnf install -y golang >/dev/null
    elif command -v yum >/dev/null 2>&1; then
      yum install -y golang >/dev/null
    elif command -v brew >/dev/null 2>&1; then
      brew install go >/dev/null
    fi
    if command -v "${current_go_bin}" >/dev/null 2>&1; then
      if python3 - "${current_go_bin}" "${required_go_version}" <<'PY'
import re
import subprocess
import sys

go_bin = sys.argv[1]
required = sys.argv[2]

def norm(value: str):
    m = re.search(r'go(\d+)\.(\d+)(?:\.(\d+))?', value)
    if not m:
        return None
    return tuple(int(part or 0) for part in m.groups())

try:
    version = subprocess.check_output([go_bin, "version"], text=True, stderr=subprocess.DEVNULL)
except Exception:
    raise SystemExit(1)

current = norm(version)
target = norm(required)
if current is None or target is None:
    raise SystemExit(1)

raise SystemExit(0 if current >= target else 1)
PY
      then
        GO_BIN="${current_go_bin}"
        return 0
      fi
    fi
  fi

  require_curl
  local os arch cache_root archive_url archive_path toolchain_name downloaded_go
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
  esac
  toolchain_name="${required_go_version#go}"
  cache_root="${XDG_CACHE_HOME:-${HOME}/.cache}/veilkey-installer/toolchains/go${toolchain_name}-${os}-${arch}"
  downloaded_go="${cache_root}/go/bin/go"
  if [[ ! -x "${downloaded_go}" ]]; then
    mkdir -p "${cache_root}"
    archive_path="${cache_root}/go${toolchain_name}.${os}-${arch}.tar.gz"
    archive_url="https://go.dev/dl/go${toolchain_name}.${os}-${arch}.tar.gz"
    curl -fsSL "${archive_url}" -o "${archive_path}"
    rm -rf "${cache_root}/go"
    tar -xzf "${archive_path}" -C "${cache_root}"
  fi
  [[ -x "${downloaded_go}" ]] || {
    echo "Error: unable to provision Go toolchain ${required_go_version}" >&2
    exit 1
  }
  GO_BIN="${downloaded_go}"
}

manifest_cmd() {
  require_go
  "${GO_BIN}" run ./cmd/installer-manifest --manifest "${MANIFEST_FILE}" "$@"
}

require_curl() {
  command -v curl >/dev/null 2>&1 || {
    echo "Error: curl not found" >&2
    exit 1
  }
}

json_field() {
  local field="$1"
  if command -v jq >/dev/null 2>&1; then
    jq -r ".${field}" 2>/dev/null
    return
  fi

  python3 -c '
import json
import sys

field = sys.argv[1]
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)

value = data
for key in field.split("."):
    if isinstance(value, dict):
        value = value.get(key)
    else:
        value = None
        break

if value is None:
    raise SystemExit(1)

if isinstance(value, (dict, list)):
    print(json.dumps(value))
else:
    print(value)
' "$field"
}

resolve_gitlab_pat() {
  if [[ -n "${VEILKEY_GITLAB_PACKAGE_PAT:-}" ]]; then
    printf '%s\n' "${VEILKEY_GITLAB_PACKAGE_PAT}"
    return 0
  fi

  if ! command -v git >/dev/null 2>&1; then
    return 0
  fi

  python3 - <<'PY' 2>/dev/null || true
import os
import subprocess

candidates = []

mirror_ip = os.environ.get("VEILKEY_MIRROR_IP", "").strip()
gitlab_host = os.environ.get("VEILKEY_GITLAB_HOST", "").strip()
vault_ip = os.environ.get("VEILKEY_VAULT_IP", "").strip()

if mirror_ip:
    candidates.append(("http", mirror_ip))
if gitlab_host:
    candidates.append(("https", gitlab_host))
if vault_ip:
    candidates.append(("http", vault_ip))

extra_host = os.environ.get("VEILKEY_GITLAB_PACKAGE_HOST", "").strip()
if extra_host:
    if "://" in extra_host:
        proto, host = extra_host.split("://", 1)
        candidates.insert(0, (proto, host))
    else:
        candidates.insert(0, ("http", extra_host))

for proto, host in candidates:
    req = f"protocol={proto}\nhost={host}\n\n".encode()
    try:
        res = subprocess.run(
            ["git", "credential", "fill"],
            input=req,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            check=True,
        )
    except Exception:
        continue

    for line in res.stdout.decode().splitlines():
        if line.startswith("password="):
            print(line.split("=", 1)[1])
            raise SystemExit(0)
PY
}

load_os_module() {
  local family="${1:-}"
  # shellcheck source=/dev/null
  source "${OS_MODULE_DIR}/common.sh"
  if [[ -z "${family}" ]]; then
    family="$(veilkey_detect_os_family)"
  fi
  # shellcheck source=/dev/null
  source "${OS_MODULE_DIR}/${family}.sh"
  veilkey_os_family="${family}"
}

cmd_detect_os() {
  load_os_module "${1:-}"
  veilkey_os_prepare_layout "/" 0
  cat <<EOF
family=${veilkey_os_family}
service_dir=${VEILKEY_OS_SERVICE_DIR}
profile_dir=${VEILKEY_OS_PROFILE_DIR}
bin_dir=${VEILKEY_OS_BIN_DIR}
install_root=/opt/veilkey
EOF
}

curl_auth_args() {
  if [[ -n "${VEILKEY_INSTALLER_AUTH_HEADER:-}" ]]; then
    printf -- "--header\0%s\0" "${VEILKEY_INSTALLER_AUTH_HEADER}"
    return 0
  fi
  local pat
  pat="$(resolve_gitlab_pat)"
  if [[ -n "${pat}" ]]; then
    printf -- "--header\0PRIVATE-TOKEN: %s\0" "${pat}"
  fi
}

normalize_download_url() {
  local url="$1"
  local api_base="${VEILKEY_INSTALLER_GITLAB_API_BASE:-}"
  local project rest project_json project_id suffix
  local -a auth_args=() tls_args=()

  case "${url}" in
    *://your-gitlab-host/*|*://your-keycenter-host/*|*://example.com/*|*://localhost/*)
      if [[ -z "${api_base}" ]]; then
        echo "Error: placeholder artifact_url requires VEILKEY_INSTALLER_GITLAB_API_BASE or a rewritten manifest URL" >&2
        exit 1
      fi
      ;;
  esac

  if [[ "${url}" =~ /api/v4/projects/([^/]+)/packages/generic/(.+)$ ]]; then
    project="${BASH_REMATCH[1]}"
    rest="${BASH_REMATCH[2]}"
    suffix="packages/generic/${rest}"
  elif [[ "${url}" =~ /api/v4/projects/([^/]+)/repository/archive\.tar\.gz\?sha=(.+)$ ]]; then
    project="${BASH_REMATCH[1]}"
    rest="${BASH_REMATCH[2]}"
    suffix="repository/archive.tar.gz?sha=${rest}"
  else
    printf '%s\n' "${url}"
    return 0
  fi

  if [[ -z "${api_base}" ]]; then
    case "${url}" in
      *) printf '%s\n' "${url}"; return 0 ;;
    esac
  fi

  case "${api_base}" in
    https://10.*) tls_args=(-k) ;;
  esac

  while IFS= read -r -d '' arg; do
    auth_args+=("${arg}")
  done < <(curl_auth_args)

  project_json="$(curl -fsSL "${tls_args[@]}" "${auth_args[@]}" "${api_base}/projects/${project}")"
  project_id="$(printf '%s' "${project_json}" | json_field "id")"
  printf '%s/projects/%s/%s\n' "${api_base}" "${project_id}" "${suffix}"
}

init_manifest() {
  cp "${ROOT_DIR}/components.toml.example" "${MANIFEST_FILE}"
  echo "Wrote ${MANIFEST_FILE}"
}

manifest_has_placeholder_urls() {
  if grep -Eq 'https?://(your-gitlab-host|your-keycenter-host|example.com|localhost)(/|:|$)' "${MANIFEST_FILE}"; then
    return 0
  fi
  return 1
}

cmd_doctor() {
  manifest_cmd validate >/dev/null
  manifest_cmd lint-legacy-layout
  if manifest_has_placeholder_urls && [[ -z "${VEILKEY_INSTALLER_GITLAB_API_BASE:-}" ]]; then
    echo "WARNING: manifest contains placeholder artifact URLs; set VEILKEY_INSTALLER_GITLAB_API_BASE or rewrite the manifest before bundle/download/install-profile" >&2
  fi
}

verify_sha256() {
  local file="$1"
  local expected="$2"

  if [[ -z "${expected}" || "${expected}" == "none" ]]; then
    echo "  WARNING: no sha256 checksum configured; skipping verification"
    return 0
  fi

  local actual
  if command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "${file}" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${file}" | awk '{print $1}')"
  else
    echo "  WARNING: sha256sum/shasum not found; skipping verification" >&2
    return 0
  fi

  if [[ "${actual}" != "${expected}" ]]; then
    echo "ERROR: SHA256 mismatch for ${file}" >&2
    echo "  expected: ${expected}" >&2
    echo "  actual:   ${actual}" >&2
    rm -f "${file}"
    return 1
  fi
  echo "  SHA256 verified: ${actual}"
}

cmd_download() {
  local profile="${1:-proxmox-lxc-allinone}"
  local dest="${2:-${ROOT_DIR}/downloads/${profile}}"
  local order name filename url sha256 resolved_url
  local -a auth_args=() tls_args=()

  require_curl
  mkdir -p "${dest}"
  while IFS= read -r -d '' arg; do
    auth_args+=("${arg}")
  done < <(curl_auth_args)

  while read -r order name filename url sha256; do
    [[ -z "${order:-}" ]] && continue
    if [[ "${order}" == "[profile]" ]]; then
      continue
    fi
    resolved_url="$(normalize_download_url "${url}")"
    tls_args=()
    case "${resolved_url}" in
      https://10.*) tls_args=(-k) ;;
    esac
    echo "Downloading ${name} -> ${dest}/${filename}"
    curl -fsSL "${tls_args[@]}" "${auth_args[@]}" "${resolved_url}" -o "${dest}/${filename}"
    verify_sha256 "${dest}/${filename}" "${sha256:-}"
  done < <(manifest_cmd plan-download "${profile}")
}

cmd_stage() {
  local profile="${1:-veilkey-allinone}"
  local root="${2:-${STAGE_DEFAULT_ROOT}/${profile}}"
  local artifacts_dir="${root}/artifacts"
  local manifests_dir="${root}/manifests"
  local state_dir="${root}/state"
  local install_plan="${state_dir}/install-plan.env"
  local release_env="${state_dir}/release.env"

  mkdir -p "${artifacts_dir}" "${manifests_dir}" "${state_dir}"
  cp "${MANIFEST_FILE}" "${manifests_dir}/components.toml"
  manifest_cmd plan-stage "${profile}" > "${install_plan}"

  {
    printf 'VEILKEY_INSTALLER_PROFILE=%s\n' "${profile}"
    printf 'VEILKEY_INSTALLER_STAGE_ROOT=%s\n' "${root}"
    printf 'VEILKEY_INSTALLER_MANIFEST=%s\n' "${manifests_dir}/components.toml"
  } > "${release_env}"

  echo "Staged installer layout at ${root}"
  echo "  manifest: ${manifests_dir}/components.toml"
  echo "  plan:     ${install_plan}"
  echo "  state:    ${release_env}"
}

cmd_bundle() {
  local profile="${1:-veilkey-allinone}"
  local root="${2:-${BUNDLE_DEFAULT_ROOT}/${profile}}"
  local downloads_dir="${root}/downloads"

  cmd_stage "${profile}" "${root}"
  cmd_download "${profile}" "${downloads_dir}"
}

component_source_dir() {
  local component="$1"
  local extract_root="$2"
  local base="${extract_root}/${component}"
  local entries=()

  [[ -d "${base}" ]] || {
    echo "Error: missing extracted component directory: ${base}" >&2
    exit 1
  }

  if [[ "${component}" != "proxy" ]]; then
    printf '%s\n' "${base}"
    return 0
  fi

  mapfile -t entries < <(find "${base}" -mindepth 1 -maxdepth 1 -type d | sort)
  if [[ "${#entries[@]}" -eq 1 ]]; then
    printf '%s\n' "${entries[0]}"
    return 0
  fi

  printf '%s\n' "${base}"
}

copy_tree() {
  local src="$1"
  local dst="$2"
  mkdir -p "${dst}"
  if [[ -d "${src}" ]]; then
    cp -a "${src}/." "${dst}/"
  fi
}

extract_component() {
  local component="$1"
  local artifact_filename="$2"
  local bundle_root="$3"
  local extract_root="$4"
  local archive

  archive="${bundle_root}/downloads/${artifact_filename}"
  [[ -f "${archive}" ]] || {
    echo "Error: missing component archive: ${archive}" >&2
    exit 1
  }
  mkdir -p "${extract_root}/${component}"
  tar -xzf "${archive}" -C "${extract_root}/${component}"
}

plan_field() {
  local line="$1"
  local key="$2"
  local fields field

  IFS=';' read -r -a fields <<<"$line"
  for field in "${fields[@]}"; do
    if [[ "${field}" == "${key}="* ]]; then
      printf '%s\n' "${field#*=}"
      return 0
    fi
  done
  return 1
}

version_ge() {
  local actual="$1"
  local required="$2"
  [[ -n "${actual}" && -n "${required}" ]] || return 1
  [[ "${actual}" == "${required}" ]] && return 0
  [[ "$(printf '%s\n%s\n' "${required}" "${actual}" | sort -V | head -n1)" == "${required}" ]]
}

binary_glibc_requirement() {
  local binary_path="$1"
  command -v strings >/dev/null 2>&1 || return 1
  strings -a "${binary_path}" 2>/dev/null \
    | grep -Eo 'GLIBC_[0-9]+\.[0-9]+' \
    | sed 's/^GLIBC_//' \
    | sort -V \
    | tail -n1
}

component_glibc_requirement() {
  local component_src="$1"
  local binary_path="$2"
  local metadata_path="${component_src}/artifact-runtime.env"

  if [[ -f "${metadata_path}" ]]; then
    # shellcheck disable=SC1090
    . "${metadata_path}"
    if [[ -n "${VEILKEY_RUNTIME_GLIBC_MIN:-}" ]]; then
      printf '%s\n' "${VEILKEY_RUNTIME_GLIBC_MIN}"
      return 0
    fi
  fi

  binary_glibc_requirement "${binary_path}" || true
}

target_glibc_version() {
  local root="${1:-/}"
  local version=""

  if [[ -n "${VEILKEY_INSTALLER_TARGET_GLIBC_VERSION:-}" ]]; then
    printf '%s\n' "${VEILKEY_INSTALLER_TARGET_GLIBC_VERSION}"
    return 0
  fi

  if [[ "${root}" != "/" ]]; then
    :
  fi

  if command -v getconf >/dev/null 2>&1; then
    version="$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}')"
  fi
  if [[ -z "${version}" ]] && command -v ldd >/dev/null 2>&1; then
    version="$(ldd --version 2>/dev/null | awk 'NR == 1 {for (i = NF; i >= 1; i--) if ($i ~ /^[0-9]+\.[0-9]+$/) {print $i; exit}}')"
  fi
  [[ -n "${version}" ]] && printf '%s\n' "${version}"
}

enforce_component_runtime_compat() {
  local component="$1"
  local extract_root="$2"
  local root="$3"
  local component_src binary_path required_glibc target_glibc

  case "${component}" in
    keycenter)
      component_src="$(component_source_dir "${component}" "${extract_root}")"
      binary_path="${component_src}/veilkey-keycenter"
      ;;
    localvault)
      component_src="$(component_source_dir "${component}" "${extract_root}")"
      binary_path="${component_src}/veilkey-localvault"
      ;;
    *)
      return 0
      ;;
  esac

  [[ -f "${binary_path}" ]] || return 0

  required_glibc="$(component_glibc_requirement "${component_src}" "${binary_path}")"
  [[ -n "${required_glibc}" ]] || return 0

  target_glibc="$(target_glibc_version "${root}")"
  [[ -n "${target_glibc}" ]] || return 0

  if ! version_ge "${target_glibc}" "${required_glibc}"; then
    echo "Error: component ${component} requires glibc ${required_glibc}, but target runtime provides ${target_glibc}." >&2
    echo "Rebuild the artifact against an older glibc baseline or install on a newer runtime." >&2
    exit 1
  fi
}

first_existing_path() {
  local path
  for path in "$@"; do
    if [[ -f "${path}" ]]; then
      printf '%s\n' "${path}"
      return 0
    fi
  done
  return 1
}

install_required_asset() {
  local destination="$1"
  local mode="$2"
  shift 2
  local source
  source="$(first_existing_path "$@")" || {
    echo "Error: missing required asset for ${destination}: $*" >&2
    exit 1
  }
  install -m "${mode}" "${source}" "${destination}"
}

install_optional_asset() {
  local destination="$1"
  local mode="$2"
  shift 2
  local source
  if ! source="$(first_existing_path "$@")"; then
    return 0
  fi
  install -m "${mode}" "${source}" "${destination}"
}

install_component_payload() {
  local component="$1"
  local extract_root="$2"
  local root="$3"
  local component_src
  local component_dst="${root%/}/opt/veilkey/${component}"

  component_src="$(component_source_dir "${component}" "${extract_root}")"
  mkdir -p "${component_dst}" "${VEILKEY_OS_BIN_DIR}"
  copy_tree "${component_src}" "${component_dst}"

  case "${component}" in
    cli)
      mkdir -p "${root%/}/etc/veilkey"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-cli" 0755 \
        "${component_src}/veilkey-cli"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-session-config" 0755 \
        "${component_src}/veilkey-session-config"
      install_optional_asset "${VEILKEY_OS_BIN_DIR}/vk" 0755 \
        "${component_src}/vk"
      install_optional_asset "${root%/}/etc/veilkey/session-tools.toml.example" 0644 \
        "${component_src}/session-tools.toml.example"
      ;;
    keycenter)
      install -m 0755 "${component_src}/veilkey-keycenter" "${VEILKEY_OS_BIN_DIR}/veilkey-keycenter"
      ;;
    localvault)
      install -m 0755 "${component_src}/veilkey-localvault" "${VEILKEY_OS_BIN_DIR}/veilkey-localvault"
      ;;
    proxy)
      local session_config_tmp
      mkdir -p "${root%/}/usr/local/lib/veilkey-proxy" "${root%/}/etc/veilkey" "${root%/}/var/log/veilkey-proxy"
      if [[ -f "${component_src}/go.mod" ]] && [[ -f "${component_src}/cmd/veilkey-session-config/main.go" ]]; then
        require_go
        session_config_tmp="$(mktemp)"
        (cd "${component_src}" && "${GO_BIN}" build -o "${session_config_tmp}" ./cmd/veilkey-session-config)
        install -m 0755 "${session_config_tmp}" "${VEILKEY_OS_BIN_DIR}/veilkey-session-config"
        rm -f "${session_config_tmp}"
      else
        install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-session-config" 0755 \
          "${component_src}/deploy/shared/veilkey-session-config"
      fi
      install -m 0755 "${component_src}/deploy/lxc/veilkey-proxy-launch" \
        "${VEILKEY_OS_BIN_DIR}/veilkey-proxy-launch"
      install -m 0755 "${component_src}/deploy/lxc/verify-proxy-lxc.sh" \
        "${root%/}/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh"
      install -m 0644 "${component_src}/deploy/lxc/veilkey-egress-proxy@.service" \
        "${VEILKEY_OS_SERVICE_DIR}/veilkey-egress-proxy@.service"
      install -m 0644 "${component_src}/deploy/host/session-tools.toml.example" \
        "${root%/}/etc/veilkey/session-tools.toml"
      install_required_asset "${root%/}/usr/local/lib/veilkey-proxy/install-veilroot-boundary.sh" 0755 \
        "${component_src}/deploy/host/install-veilroot-boundary.sh" \
        "${component_src}/deploy/host/install-root-ai-boundary.sh"
      install_optional_asset "${root%/}/usr/local/lib/veilkey-proxy/install-veilroot-codex.sh" 0755 \
        "${component_src}/deploy/host/install-veilroot-codex.sh"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/verify-veilroot-session" 0755 \
        "${component_src}/deploy/host/verify-veilroot-session.sh" \
        "${component_src}/deploy/host/verify-root-ai-session.sh"
      install -m 0755 "${component_src}/deploy/host/veilroot-shell" \
        "${VEILKEY_OS_BIN_DIR}/veilroot-shell"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-session" 0755 \
        "${component_src}/deploy/host/veilkey-veilroot-session" \
        "${component_src}/deploy/host/veilkey-root-ai-session"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-observe" 0755 \
        "${component_src}/deploy/host/veilkey-veilroot-observe" \
        "${component_src}/deploy/host/veilkey-root-ai-observe"
      install_required_asset "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-egress-guard" 0755 \
        "${component_src}/deploy/host/veilkey-veilroot-egress-guard" \
        "${component_src}/deploy/host/veilkey-root-ai-egress-guard"
      install -m 0755 "${component_src}/deploy/host/veilkey-veilroot-curl" \
        "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-curl"
      install -m 0755 "${component_src}/deploy/host/veilkey-veilroot-wget" \
        "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-wget"
      install -m 0755 "${component_src}/deploy/host/veilkey-veilroot-http" \
        "${VEILKEY_OS_BIN_DIR}/veilkey-veilroot-http"
      install_required_asset "${VEILKEY_OS_SERVICE_DIR}/veilkey-veilroot-observe@.service" 0644 \
        "${component_src}/deploy/host/veilkey-veilroot-observe@.service" \
        "${component_src}/deploy/host/veilkey-root-ai-observe@.service"
      install_required_asset "${VEILKEY_OS_SERVICE_DIR}/veilkey-veilroot-egress-guard@.service" 0644 \
        "${component_src}/deploy/host/veilkey-veilroot-egress-guard@.service" \
        "${component_src}/deploy/host/veilkey-root-ai-egress-guard@.service"
      install -d "${root%/}/usr/local/share/veilkey/snippets"
      install_optional_asset "${root%/}/usr/local/share/veilkey/snippets/veilroot-veilkey-shell.sh" 0755 \
        "${component_src}/deploy/host/snippets/veilroot-veilkey-shell.sh"
      ;;
  esac
}

write_component_env_templates() {
  local root="$1"
  local veilkey_etc="${root%/}/etc/veilkey"

  mkdir -p "${veilkey_etc}"

  cat > "${veilkey_etc}/keycenter.env.example" <<'EOF'
# VEILKEY_PASSWORD_FILE: path to a file containing the KEK password (mode 0600).
# Do NOT use VEILKEY_PASSWORD env var — it exposes the password in the process table.
VEILKEY_PASSWORD_FILE=/etc/veilkey/keycenter.password
VEILKEY_ADDR=:10181
VEILKEY_DB_PATH=/opt/veilkey/keycenter/data/veilkey.db
VEILKEY_TLS_CERT=
VEILKEY_TLS_KEY=
VEILKEY_TLS_CA=
EOF

  cat > "${veilkey_etc}/localvault.env.example" <<'EOF'
# VEILKEY_PASSWORD_FILE: path to a file containing the KEK password (mode 0600).
# Do NOT use VEILKEY_PASSWORD env var — it exposes the password in the process table.
VEILKEY_PASSWORD_FILE=/etc/veilkey/localvault.password
VEILKEY_ADDR=:10180
VEILKEY_DB_PATH=/opt/veilkey/localvault/data/veilkey.db
VEILKEY_KEYCENTER_URL=
VEILKEY_TLS_CERT=
VEILKEY_TLS_KEY=
VEILKEY_TLS_CA=
EOF

  cat > "${veilkey_etc}/proxy.env.example" <<'EOF'
VEILKEY_LOCALVAULT_URL=
VEILKEY_KEYCENTER_URL=
VEILKEY_PROXY_ACCESS_LOG_FORMAT=jsonl
EOF
}

profile_file() {
  local profile="$1"
  printf '%s/%s.env.example\n' "${PROFILE_DIR}" "${profile}"
}

require_profile_file() {
  local profile="$1"
  local file
  file="$(profile_file "${profile}")"
  [[ -f "${file}" ]] || {
    echo "Error: missing profile template: ${file}" >&2
    exit 1
  }
}

profile_env_default() {
  local profile="$1"
  local key="$2"
  local file
  file="$(profile_file "${profile}")"
  awk -F= -v key="${key}" '$1 == key {print $2; exit}' "${file}"
}

profile_has_component() {
  local profile="$1"
  local component="$2"
  manifest_cmd plan "${profile}" | awk 'NF >= 2 {print $2}' | grep -Fx "${component}" >/dev/null 2>&1
}

render_profile_envs() {
  local profile="$1"
  local root="$2"
  local veilkey_etc="${root%/}/etc/veilkey"
  local installer_state="${veilkey_etc}/installer-profile.env"
  local enable_keycenter enable_localvault enable_proxy
  local default_enable_keycenter default_enable_localvault default_enable_proxy
  local keycenter_addr keycenter_db localvault_addr localvault_db localvault_trusted_ips
  local keycenter_url proxy_localvault_url proxy_keycenter_url
  local default_keycenter_addr default_localvault_addr default_keycenter_url

  require_profile_file "${profile}"
  mkdir -p "${veilkey_etc}"

  if profile_has_component "${profile}" "keycenter"; then
    default_enable_keycenter=1
  else
    default_enable_keycenter=0
  fi
  if profile_has_component "${profile}" "localvault"; then
    default_enable_localvault=1
  else
    default_enable_localvault=0
  fi
  if profile_has_component "${profile}" "proxy"; then
    default_enable_proxy="$(profile_env_default "${profile}" "VEILKEY_ENABLE_PROXY")"
    default_enable_proxy="${default_enable_proxy:-0}"
  else
    default_enable_proxy=0
  fi
  default_enable_keycenter="${VEILKEY_ENABLE_KEYCENTER_DEFAULT:-$(profile_env_default "${profile}" "VEILKEY_ENABLE_KEYCENTER" || true)}"
  default_enable_keycenter="${default_enable_keycenter:-$(if profile_has_component "${profile}" "keycenter"; then echo 1; else echo 0; fi)}"
  default_enable_localvault="${VEILKEY_ENABLE_LOCALVAULT_DEFAULT:-$(profile_env_default "${profile}" "VEILKEY_ENABLE_LOCALVAULT" || true)}"
  default_enable_localvault="${default_enable_localvault:-$(if profile_has_component "${profile}" "localvault"; then echo 1; else echo 0; fi)}"

  enable_keycenter="${VEILKEY_ENABLE_KEYCENTER:-${default_enable_keycenter}}"
  enable_localvault="${VEILKEY_ENABLE_LOCALVAULT:-${default_enable_localvault}}"
  enable_proxy="${VEILKEY_ENABLE_PROXY:-${default_enable_proxy}}"
  if [[ "${profile}" = "proxmox-lxc-allinone" && "${enable_proxy}" = "1" ]]; then
    echo "Error: proxmox-lxc-allinone does not support enabling proxy runtime inside the LXC." >&2
    echo "Install the host companion boundary separately with proxmox-host-cli." >&2
    exit 1
  fi
  default_keycenter_addr=":10180"
  default_localvault_addr=":10180"
  default_keycenter_url="http://127.0.0.1:10180"
  if [[ "${default_enable_keycenter}" = "1" && "${default_enable_localvault}" = "1" ]]; then
    default_keycenter_addr=":10181"
    default_localvault_addr=":10180"
    default_keycenter_url="http://127.0.0.1:10181"
  fi
  keycenter_addr="${VEILKEY_KEYCENTER_ADDR:-${default_keycenter_addr}}"
  keycenter_db="${VEILKEY_KEYCENTER_DB_PATH:-/opt/veilkey/keycenter/data/veilkey.db}"
  localvault_addr="${VEILKEY_LOCALVAULT_ADDR:-${default_localvault_addr}}"
  localvault_db="${VEILKEY_LOCALVAULT_DB_PATH:-/opt/veilkey/localvault/data/veilkey.db}"
  localvault_trusted_ips="${VEILKEY_LOCALVAULT_TRUSTED_IPS:-}"
  keycenter_url="${VEILKEY_KEYCENTER_URL:-${default_keycenter_url}}"
  proxy_localvault_url="${VEILKEY_PROXY_LOCALVAULT_URL:-https://127.0.0.1:10180}"
  proxy_keycenter_url="${VEILKEY_PROXY_KEYCENTER_URL:-${keycenter_url}}"

  cat > "${installer_state}" <<EOF
VEILKEY_PROFILE=${profile}
VEILKEY_ENABLE_KEYCENTER=${enable_keycenter}
VEILKEY_ENABLE_LOCALVAULT=${enable_localvault}
VEILKEY_ENABLE_PROXY=${enable_proxy}
VEILKEY_KEYCENTER_URL=${keycenter_url}
VEILKEY_PROXY_LOCALVAULT_URL=${proxy_localvault_url}
VEILKEY_PROXY_KEYCENTER_URL=${proxy_keycenter_url}
EOF

  # Write password files with restricted permissions (never store in env files)
  if [[ -n "${VEILKEY_KEYCENTER_PASSWORD:-}" ]]; then
    printf '%s' "${VEILKEY_KEYCENTER_PASSWORD}" > "${veilkey_etc}/keycenter.password"
    chmod 600 "${veilkey_etc}/keycenter.password"
  fi
  if [[ -n "${VEILKEY_LOCALVAULT_PASSWORD:-}" ]]; then
    printf '%s' "${VEILKEY_LOCALVAULT_PASSWORD}" > "${veilkey_etc}/localvault.password"
    chmod 600 "${veilkey_etc}/localvault.password"
  fi

  cat > "${veilkey_etc}/keycenter.env" <<EOF
VEILKEY_PASSWORD_FILE=${veilkey_etc}/keycenter.password
VEILKEY_ADDR=${keycenter_addr}
VEILKEY_DB_PATH=${keycenter_db}
VEILKEY_TLS_CERT=${VEILKEY_TLS_CERT:-}
VEILKEY_TLS_KEY=${VEILKEY_TLS_KEY:-}
VEILKEY_TLS_CA=${VEILKEY_TLS_CA:-}
EOF

  cat > "${veilkey_etc}/localvault.env" <<EOF
VEILKEY_PASSWORD_FILE=${veilkey_etc}/localvault.password
VEILKEY_ADDR=${localvault_addr}
VEILKEY_DB_PATH=${localvault_db}
VEILKEY_KEYCENTER_URL=${keycenter_url}
VEILKEY_TRUSTED_IPS=${localvault_trusted_ips}
VEILKEY_TLS_CERT=${VEILKEY_TLS_CERT:-}
VEILKEY_TLS_KEY=${VEILKEY_TLS_KEY:-}
VEILKEY_TLS_CA=${VEILKEY_TLS_CA:-}
EOF

  cat > "${veilkey_etc}/proxy.env" <<EOF
VEILKEY_LOCALVAULT_URL=${proxy_localvault_url}
VEILKEY_KEYCENTER_URL=${proxy_keycenter_url}
VEILKEY_PROXY_ACCESS_LOG_FORMAT=${VEILKEY_PROXY_ACCESS_LOG_FORMAT:-jsonl}
EOF
}

write_service_targets() {
  local root="$1"
  local veilkey_etc="${root%/}/etc/veilkey"
  local installer_state="${veilkey_etc}/installer-profile.env"
  local services_file="${veilkey_etc}/services.enabled"

  # shellcheck disable=SC1090
  source "${installer_state}"
  : > "${services_file}"
  [[ "${VEILKEY_ENABLE_KEYCENTER}" = "1" ]] && echo "veilkey-keycenter.service" >> "${services_file}"
  [[ "${VEILKEY_ENABLE_LOCALVAULT}" = "1" ]] && echo "veilkey-localvault.service" >> "${services_file}"
  if [[ "${VEILKEY_ENABLE_PROXY:-0}" = "1" ]]; then
    echo "veilkey-egress-proxy@default.service" >> "${services_file}"
    echo "veilkey-egress-proxy@codex.service" >> "${services_file}"
    echo "veilkey-egress-proxy@claude.service" >> "${services_file}"
    echo "veilkey-egress-proxy@opencode.service" >> "${services_file}"
  fi
}

write_systemd_units() {
  local root="$1"

  mkdir -p "${VEILKEY_OS_SERVICE_DIR}"

  cat > "${VEILKEY_OS_SERVICE_DIR}/veilkey-keycenter.service" <<'EOF'
[Unit]
Description=VeilKey KeyCenter
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/veilkey/keycenter
EnvironmentFile=-/etc/veilkey/keycenter.env
ExecStart=/usr/local/bin/veilkey-keycenter
Restart=on-failure
RestartSec=3
# Restrict access to password file
UMask=0077

[Install]
WantedBy=multi-user.target
EOF

  cat > "${VEILKEY_OS_SERVICE_DIR}/veilkey-localvault.service" <<'EOF'
[Unit]
Description=VeilKey LocalVault
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/veilkey/localvault
EnvironmentFile=-/etc/veilkey/localvault.env
ExecStart=/usr/local/bin/veilkey-localvault
Restart=on-failure
RestartSec=3
# Restrict access to password file
UMask=0077

[Install]
WantedBy=multi-user.target
EOF
}

write_profile_script() {
  local root="$1"

  mkdir -p "${VEILKEY_OS_PROFILE_DIR}"
  cat > "${VEILKEY_OS_PROFILE_DIR}/veilkey.sh" <<'EOF'
export PATH="/usr/local/bin:${PATH}"
EOF
}

write_post_install_health_script() {
  local root="$1"
  local bin_dir="${root%/}/opt/veilkey/installer/bin"

  mkdir -p "${bin_dir}"
  cat > "${bin_dir}/veilkey-post-install-health" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-/}"
veilkey_etc="${ROOT%/}/etc/veilkey"
installer_state="${veilkey_etc}/installer-profile.env"
check_cmd() {
  local path="$1"
  [[ -x "${ROOT%/}${path}" ]] || {
    echo "missing executable: ${path}" >&2
    exit 1
  }
}

check_file() {
  local path="$1"
  [[ -f "${ROOT%/}${path}" ]] || {
    echo "missing file: ${path}" >&2
    exit 1
  }
}

if [[ -f "${installer_state}" ]]; then
  # shellcheck disable=SC1090
  source "${installer_state}"
fi

if [[ "${VEILKEY_ENABLE_KEYCENTER:-0}" = "1" ]]; then
  check_cmd /usr/local/bin/veilkey-keycenter
fi
if [[ "${VEILKEY_ENABLE_LOCALVAULT:-0}" = "1" ]]; then
  check_cmd /usr/local/bin/veilkey-localvault
fi
check_file /etc/veilkey/keycenter.env.example
check_file /etc/veilkey/localvault.env.example
if [[ "${VEILKEY_ENABLE_PROXY:-0}" = "1" ]]; then
  check_cmd /usr/local/bin/veilkey-session-config
  check_cmd /usr/local/bin/veilkey-proxy-launch
  check_cmd /usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh
  check_file /etc/veilkey/proxy.env
  check_file /etc/veilkey/proxy.env.example
  check_file /etc/veilkey/session-tools.toml
  check_file /etc/systemd/system/veilkey-egress-proxy@.service
fi
check_file /etc/veilkey/installer-profile.env
check_file /etc/veilkey/services.enabled
check_file /etc/profile.d/veilkey.sh

echo "post-install scaffold ok"
EOF
  chmod +x "${bin_dir}/veilkey-post-install-health"
}

write_activation_script() {
  local root="$1"
  local bin_dir="${root%/}/opt/veilkey/installer/bin"

  mkdir -p "${bin_dir}"
  cat > "${bin_dir}/veilkey-activate-services" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-/}"
services_file="${ROOT%/}/etc/veilkey/services.enabled"
[[ -f "${services_file}" ]] || {
  echo "missing service list: ${services_file}" >&2
  exit 1
}

mapfile -t services < "${services_file}"
printf 'services=%s\n' "${services[*]}"

if [[ "${ROOT}" != "/" ]]; then
  echo "non-rootfs activation is a plan only; run on the live target root to enable/start services"
  exit 0
fi

command -v systemctl >/dev/null 2>&1 || {
  echo "systemctl not found" >&2
  exit 1
}

systemctl daemon-reload
for svc in "${services[@]}"; do
  systemctl enable "${svc}"
  systemctl restart "${svc}"
done
EOF
  chmod +x "${bin_dir}/veilkey-activate-services"
}

write_install_state() {
  local profile="$1"
  local root="$2"
  local bundle_root="$3"

  mkdir -p "${root%/}/opt/veilkey/installer"
  cp "${MANIFEST_FILE}" "${root%/}/opt/veilkey/installer/components.toml"
  cat > "${root%/}/opt/veilkey/installer/install.env" <<EOF
VEILKEY_INSTALLER_PROFILE=${profile}
VEILKEY_INSTALLER_OS_FAMILY=${veilkey_os_family}
VEILKEY_INSTALLER_BUNDLE_ROOT=${bundle_root}
VEILKEY_INSTALLER_ROOT=${root}
EOF
}

cmd_plan_install() {
  local profile="${1:-veilkey-allinone}"
  local root="${2:-/}"

  load_os_module "${VEILKEY_INSTALLER_OS_FAMILY:-}"
  veilkey_os_prepare_layout "${root}"

  echo "profile=${profile}"
  echo "os_family=${veilkey_os_family}"
  echo "root=${root}"
  echo "service_dir=${VEILKEY_OS_SERVICE_DIR}"
  echo "profile_dir=${VEILKEY_OS_PROFILE_DIR}"
  echo "bin_dir=${VEILKEY_OS_BIN_DIR}"
  manifest_cmd plan "${profile}"
}

cmd_install() {
  local profile="${1:-veilkey-allinone}"
  local root="${2:-/}"
  local bundle_root="${3:-${BUNDLE_DEFAULT_ROOT}/${profile}}"
  local extract_root="${bundle_root}/extracted"
  local install_plan="${bundle_root}/state/install-plan.env"
  local line component artifact_filename

  load_os_module "${VEILKEY_INSTALLER_OS_FAMILY:-}"
  veilkey_os_prepare_layout "${root}"

  if [[ ! -f "${install_plan}" ]]; then
    echo "Bundle missing at ${bundle_root}; creating it first"
    cmd_bundle "${profile}" "${bundle_root}"
  fi

  mkdir -p "${extract_root}"
  while IFS= read -r line; do
    [[ "${line}" == component=* ]] || continue
    component="$(plan_field "${line}" component)"
    artifact_filename="$(plan_field "${line}" artifact_filename)"
    extract_component "${component}" "${artifact_filename}" "${bundle_root}" "${extract_root}"
    enforce_component_runtime_compat "${component}" "${extract_root}" "${root}"
    install_component_payload "${component}" "${extract_root}" "${root}"
  done < "${install_plan}"

  write_component_env_templates "${root}"
  render_profile_envs "${profile}" "${root}"
  write_service_targets "${root}"
  write_systemd_units "${root}"
  write_profile_script "${root}"
  write_post_install_health_script "${root}"
  write_activation_script "${root}"
  write_install_state "${profile}" "${root}" "${bundle_root}"
  veilkey_os_finalize_install "${root}"
  echo "Installed profile ${profile} for ${veilkey_os_family} into ${root}"
}

cmd_configure() {
  local profile="${1:-veilkey-allinone}"
  local root="${2:-/}"

  load_os_module "${VEILKEY_INSTALLER_OS_FAMILY:-}"
  veilkey_os_prepare_layout "${root}"
  render_profile_envs "${profile}" "${root}"
  write_service_targets "${root}"
  write_activation_script "${root}"
  echo "Configured profile ${profile} into ${root}"
}

cmd_install_profile() {
  local activate=0
  local health=0
  local profile root bundle_root

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --activate)
        activate=1
        shift
        ;;
      --health)
        health=1
        shift
        ;;
      --)
        shift
        break
        ;;
      -*)
        echo "Error: unknown option for install-profile: $1" >&2
        exit 2
        ;;
      *)
        break
        ;;
    esac
  done

  profile="${1:-veilkey-allinone}"
  root="${2:-/}"
  bundle_root="${3:-${BUNDLE_DEFAULT_ROOT}/${profile}}"

  if [[ -f "${bundle_root}/manifests/components.toml" && -f "${bundle_root}/state/install-plan.env" ]]; then
    echo "Reusing existing bundle at ${bundle_root}"
  else
    cmd_bundle "${profile}" "${bundle_root}"
  fi
  cmd_install "${profile}" "${root}" "${bundle_root}"
  cmd_configure "${profile}" "${root}"

  if [[ "${health}" == "1" ]]; then
    cmd_post_install_health "${root}"
  fi

  if [[ "${activate}" == "1" ]]; then
    cmd_activate "${root}"
  fi
}

cmd_plan_activate() {
  local root="${1:-/}"
  local services_file="${root%/}/etc/veilkey/services.enabled"
  [[ -f "${services_file}" ]] || {
    echo "Error: missing service list: ${services_file}" >&2
    exit 1
  }
  echo "root=${root}"
  echo "services:"
  sed 's/^/- /' "${services_file}"
}

cmd_activate() {
  local root="${1:-/}"
  "${root%/}/opt/veilkey/installer/bin/veilkey-activate-services" "${root}"
}

cmd_post_install_health() {
  local root="${1:-/}"
  "${root%/}/opt/veilkey/installer/bin/veilkey-post-install-health" "${root}"
}

cmd="${1:-help}"
shift || true

case "${cmd}" in
  init)
    init_manifest
    ;;
  validate)
    manifest_cmd validate
    ;;
  doctor)
    cmd_doctor
    ;;
  detect-os)
    cmd_detect_os "${1:-}"
    ;;
  list)
    manifest_cmd list-components
    ;;
  profiles)
    manifest_cmd list-profiles
    ;;
  plan)
    manifest_cmd plan "${1:-veilkey-allinone}"
    ;;
  plan-stage)
    manifest_cmd plan-stage "${1:-veilkey-allinone}"
    ;;
  plan-download)
    manifest_cmd plan-download "${1:-veilkey-allinone}"
    ;;
  plan-install)
    cmd_plan_install "${1:-veilkey-allinone}" "${2:-/}"
    ;;
  download)
    cmd_download "${1:-veilkey-allinone}" "${2:-}"
    ;;
  stage)
    cmd_stage "${1:-veilkey-allinone}" "${2:-}"
    ;;
  bundle)
    cmd_bundle "${1:-veilkey-allinone}" "${2:-}"
    ;;
  install)
    cmd_install "${1:-veilkey-allinone}" "${2:-/}" "${3:-}"
    ;;
  configure)
    cmd_configure "${1:-veilkey-allinone}" "${2:-/}"
    ;;
  install-profile)
    cmd_install_profile "$@"
    ;;
  plan-activate)
    cmd_plan_activate "${1:-/}"
    ;;
  activate)
    cmd_activate "${1:-/}"
    ;;
  post-install-health)
    cmd_post_install_health "${1:-/}"
    ;;
  print-json)
    manifest_cmd print-json
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    echo "Error: unknown command: ${cmd}" >&2
    usage
    exit 2
    ;;
esac
