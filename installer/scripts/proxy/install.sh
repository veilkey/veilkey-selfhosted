#!/usr/bin/env bash
# VeilKey Proxy Runtime — Independent Installer
# Installs veilkey-proxy binary, veilkey-session-config, session-tools.toml,
# and systemd units. Does NOT set up veilroot boundary (use install-veilroot-boundary.sh).
#
# Usage:
#   bash installer/scripts/proxy/install.sh [root]
#
# Environment:
#   VEILKEY_PROXY_SRC        Path to proxy source (default: services/proxy in repo)
#   VEILKEY_SESSION_TOOLS    Path to session-tools.toml (copies example if missing)
#   VEILKEY_LOCALVAULT_URL   LocalVault URL for proxy config
#   VEILKEY_KEYCENTER_URL    KeyCenter URL for proxy hub
set -euo pipefail

SELF_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SELF_DIR/../../.." && pwd)"
PROXY_SRC="${VEILKEY_PROXY_SRC:-${REPO_ROOT}/services/proxy}"
root="${1:-/}"

BIN_DIR="${root%/}/usr/local/bin"
ETC_DIR="${root%/}/etc/veilkey"
SYSTEMD_DIR="${root%/}/etc/systemd/system"
LIB_DIR="${root%/}/usr/local/lib/veilkey-proxy"

info()  { echo "[proxy-install] $*"; }
error() { echo "[proxy-install] ERROR: $*" >&2; exit 1; }

# --- Validate source ---
[[ -f "${PROXY_SRC}/go.mod" ]] || error "proxy source not found at ${PROXY_SRC}"

# --- Ensure target directories exist ---
mkdir -p "${BIN_DIR}" "${ETC_DIR}" "${LIB_DIR}" "${LIB_DIR}/snippets" "${SYSTEMD_DIR}"

# --- Build binaries ---
info "building veilkey-proxy..."
(cd "${PROXY_SRC}" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_DIR}/veilkey-proxy" ./cmd/veilkey-proxy/)

info "building veilkey-session-config..."
(cd "${PROXY_SRC}" && CGO_ENABLED=0 go build -ldflags="-s -w" -o "${BIN_DIR}/veilkey-session-config" ./cmd/veilkey-session-config/)

chmod +x "${BIN_DIR}/veilkey-proxy" "${BIN_DIR}/veilkey-session-config"

# --- Install session-tools.toml ---
if [[ -n "${VEILKEY_SESSION_TOOLS:-}" && -f "${VEILKEY_SESSION_TOOLS}" ]]; then
  install -m 0644 "${VEILKEY_SESSION_TOOLS}" "${ETC_DIR}/session-tools.toml"
elif [[ ! -f "${ETC_DIR}/session-tools.toml" ]]; then
  install -m 0644 "${PROXY_SRC}/deploy/host/session-tools.toml.example" "${ETC_DIR}/session-tools.toml"
  info "installed example session-tools.toml (edit to match your environment)"
fi

# --- Install lib scripts ---
# install-veilroot-boundary.sh lives under deploy/host/
for script in install-veilroot-boundary.sh install-veilroot-codex.sh; do
  src="${PROXY_SRC}/deploy/host/${script}"
  [[ -f "$src" ]] && install -m 0755 "$src" "${LIB_DIR}/${script}"
done

# verify-proxy-lxc.sh lives under deploy/lxc/ (not deploy/host/)
if [[ -f "${PROXY_SRC}/deploy/lxc/verify-proxy-lxc.sh" ]]; then
  install -m 0755 "${PROXY_SRC}/deploy/lxc/verify-proxy-lxc.sh" "${LIB_DIR}/verify-proxy-lxc.sh"
fi

# --- Install host bin scripts ---
for script in veilroot-shell veilkey-veilroot-session veilkey-veilroot-observe \
  veilkey-veilroot-egress-guard veilkey-veilroot-curl veilkey-veilroot-wget \
  veilkey-veilroot-http verify-veilroot-session.sh; do
  src="${PROXY_SRC}/deploy/host/${script}"
  [[ -f "$src" ]] && install -m 0755 "$src" "${BIN_DIR}/${script}"
done

# --- Install shell snippets ---
if [[ -d "${PROXY_SRC}/deploy/host/snippets" ]]; then
  cp -R "${PROXY_SRC}/deploy/host/snippets/." "${LIB_DIR}/snippets/"
fi

# --- Install systemd units ---
for unit in veilkey-veilroot-observe@.service veilkey-veilroot-egress-guard@.service; do
  src="${PROXY_SRC}/deploy/host/${unit}"
  [[ -f "$src" ]] && install -m 0644 "$src" "${SYSTEMD_DIR}/${unit}"
done

# --- Render proxy.env ---
cat > "${ETC_DIR}/proxy.env" <<EOF
VEILKEY_LOCALVAULT_URL=${VEILKEY_LOCALVAULT_URL:-https://127.0.0.1:10180}
VEILKEY_KEYCENTER_URL=${VEILKEY_KEYCENTER_URL:-https://127.0.0.1:10181}
VEILKEY_PROXY_ACCESS_LOG_FORMAT=${VEILKEY_PROXY_ACCESS_LOG_FORMAT:-jsonl}
EOF

info "proxy runtime installed to ${root}"
info ""
info "next steps:"
info "  1. edit ${ETC_DIR}/session-tools.toml for your environment"
info "  2. run install-veilroot-boundary.sh to set up veilroot user (live root only)"
info "  3. systemctl daemon-reload && systemctl enable veilkey-veilroot-observe@default"
