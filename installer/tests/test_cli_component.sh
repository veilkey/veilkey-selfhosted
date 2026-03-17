#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_manifest="$(mktemp)"
tmp_bundle="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_artifact_dir="$(mktemp -d)"
trap 'rm -f "$tmp_manifest"; rm -rf "$tmp_bundle" "$tmp_root" "$tmp_artifact_dir"' EXIT

artifact="${tmp_artifact_dir}/veilkey-cli.tar.gz"
pkg_root="${tmp_artifact_dir}/pkg"
mkdir -p "$pkg_root"
cp ../client/cli/bin/veilkey-cli-linux-amd64 "$pkg_root/veilkey-cli"
cp ../client/cli/bin/veilkey-session-config-linux-amd64 "$pkg_root/veilkey-session-config"
cp ../client/cli/scripts/vk "$pkg_root/vk"
cp ../client/cli/deploy/host/session-tools.toml.example "$pkg_root/session-tools.toml.example"
tar -czf "$artifact" -C "$pkg_root" veilkey-cli veilkey-session-config vk session-tools.toml.example

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
python3 - "$tmp_manifest" "$artifact" <<'PY'
from pathlib import Path
import sys

manifest = Path(sys.argv[1])
artifact = Path(sys.argv[2]).resolve()
text = manifest.read_text()
text = text.replace('ref = "RELEASE_OR_COMMIT"', 'ref = "local-test"', 1)
text = text.replace(
    'artifact_url = "https://your-gitlab-host/api/v4/projects/veilkey%2Fveilkey-selfhosted/packages/generic/veilkey-cli/RELEASE_OR_COMMIT/veilkey-cli.tar.gz"',
    f'artifact_url = "file://{artifact}"',
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

echo "ok: cli component install layout"
