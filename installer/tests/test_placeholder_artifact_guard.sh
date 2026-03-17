#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_manifest="$(mktemp)"
tmp_err="$(mktemp)"
trap 'rm -rf "$tmp_bundle" "$tmp_root"; rm -f "$tmp_manifest" "$tmp_err"' EXIT

env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null

if env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  -u VEILKEY_INSTALLER_CLI_RELEASE_TAG \
  VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install-profile proxmox-host "$tmp_root" "$tmp_bundle" >/dev/null 2>"$tmp_err"; then
  echo "expected install-profile to fail without VEILKEY_INSTALLER_GITLAB_API_BASE when manifest still has placeholder URLs" >&2
  exit 1
fi

grep -E "placeholder artifact_url requires VEILKEY_INSTALLER_GITLAB_API_BASE or a rewritten manifest URL|placeholder artifact_url still contains an unresolved release/tag token" "$tmp_err" >/dev/null

cli_only_manifest="$(mktemp)"
cli_only_root="$(mktemp -d)"
cli_only_bundle="$(mktemp -d)"
trap 'rm -rf "$tmp_bundle" "$tmp_root" "$cli_only_root" "$cli_only_bundle"; rm -f "$tmp_manifest" "$tmp_err" "$cli_only_manifest"' EXIT

cat > "$cli_only_manifest" <<'EOF'
[release]
name = "veilkey-core"
version = "2026.03.08"
channel = "stable"

[components.cli]
source = "github"
project = "veilkey/veilkey-selfhosted"
ref = "RELEASE_OR_TAG"
type = "binary"
install_order = 25
artifact_url = "https://github.com/veilkey/veilkey-selfhosted/releases/download/RELEASE_OR_TAG/veilkey-cli_RELEASE_OR_TAG_linux_amd64.tar.gz"
artifact_filename = "veilkey-cli_RELEASE_OR_TAG_linux_amd64.tar.gz"
sha256 = ""

[profiles.proxmox-host-cli]
description = "Proxmox host boundary CLI package install"
components = ["cli"]
EOF

if env -u VEILKEY_INSTALLER_GITLAB_API_BASE \
  -u VEILKEY_INSTALLER_CLI_RELEASE_TAG \
  VEILKEY_INSTALLER_MANIFEST="$cli_only_manifest" ./install.sh install-profile proxmox-host-cli "$cli_only_root" "$cli_only_bundle" >/dev/null 2>"$tmp_err"; then
  echo "expected cli-only install-profile to fail without VEILKEY_INSTALLER_CLI_RELEASE_TAG" >&2
  exit 1
fi

grep -F "placeholder artifact_url still contains an unresolved release/tag token" "$tmp_err" >/dev/null

echo "ok: placeholder artifact_url guard"
