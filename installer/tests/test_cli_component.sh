#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_manifest="$(mktemp)"
tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_artifact_dir="$(mktemp -d)"
trap 'rm -f "$tmp_manifest"; rm -rf "$tmp_bundle" "$tmp_root" "$tmp_artifact_dir"' EXIT

cli_artifact="${tmp_artifact_dir}/veilkey-cli.tar.gz"
proxy_artifact="${tmp_artifact_dir}/veilkey-proxy-local.tar.gz"
cli_root="${tmp_artifact_dir}/cli"
proxy_root="${tmp_artifact_dir}/veilkey-proxy-local"

mkdir -p "$cli_root" "$proxy_root"
cp ../client/cli/bin/veilkey-cli-linux-amd64 "$cli_root/veilkey-cli"
cp ../client/cli/bin/veilkey-session-config-linux-amd64 "$cli_root/veilkey-session-config"
cp ../client/cli/scripts/vk "$cli_root/vk"
cp ../client/cli/deploy/host/session-tools.toml.example "$cli_root/session-tools.toml.example"
tar -czf "$cli_artifact" -C "$cli_root" veilkey-cli veilkey-session-config vk session-tools.toml.example

cp -a ../services/proxy/. "$proxy_root/"
tar -czf "$proxy_artifact" -C "$tmp_artifact_dir" veilkey-proxy-local

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
python3 - "$tmp_manifest" "$cli_artifact" "$proxy_artifact" <<'PY'
from pathlib import Path
import sys

manifest = Path(sys.argv[1])
cli_artifact = Path(sys.argv[2]).resolve()
proxy_artifact = Path(sys.argv[3]).resolve()
text = manifest.read_text()

text = text.replace('ref = "RELEASE_OR_COMMIT"', 'ref = "local-test"', 1)
text = text.replace(
    'artifact_url = "https://your-gitlab-host/api/v4/projects/veilkey%2Fveilkey-selfhosted/packages/generic/veilkey-cli/RELEASE_OR_COMMIT/veilkey-cli.tar.gz"',
    f'artifact_url = "file://{cli_artifact}"',
    1,
)
text = text.replace(
    'artifact_url = "https://your-gitlab-host/api/v4/projects/veilkey%2Fveilkey-proxy/repository/archive.tar.gz?sha=670d1e33736adab35149275428ed3aa75b4e787b"',
    f'artifact_url = "file://{proxy_artifact}"',
    1,
)
text = text.replace(
    'artifact_filename = "veilkey-proxy-670d1e33736adab35149275428ed3aa75b4e787b.tar.gz"',
    'artifact_filename = "veilkey-proxy-local.tar.gz"',
    1,
)
manifest.write_text(text)
PY

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null
plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan proxmox-host-cli)"
printf '%s\n' "$plan" | grep -F "cli" >/dev/null
printf '%s\n' "$plan" | grep -F "proxy" >/dev/null

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh bundle proxmox-host-cli "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install proxmox-host-cli "$tmp_root" "$tmp_bundle" >/dev/null

test -x "$tmp_root/usr/local/bin/veilkey-cli"
test -x "$tmp_root/usr/local/bin/veilkey-session-config"
test -x "$tmp_root/usr/local/bin/vk"
test -f "$tmp_root/etc/veilkey/session-tools.toml.example"
test -x "$tmp_root/usr/local/bin/veilroot-shell"

echo "ok: cli component install layout"
