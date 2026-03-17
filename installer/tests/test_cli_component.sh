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
if [[ ! -x ../client/cli/bin/veilkey-cli-linux-amd64 ]]; then
  (
    cd ../client/cli
    make build-linux-amd64
  )
fi
if [[ ! -x ../client/cli/bin/veilkey-session-config-linux-amd64 ]]; then
  (
    cd ../client/cli
    make build-session-config-linux-amd64
  )
fi
cp ../client/cli/bin/veilkey-cli-linux-amd64 "$cli_root/veilkey-cli"
cp ../client/cli/bin/veilkey-session-config-linux-amd64 "$cli_root/veilkey-session-config"
cp ../client/cli/deploy/host/veil "$cli_root/veil"
cp ../client/cli/scripts/vk "$cli_root/vk"
cp ../client/cli/deploy/host/session-tools.toml.example "$cli_root/session-tools.toml.example"
tar -czf "$cli_artifact" -C "$cli_root" veilkey-cli veilkey-session-config veil vk session-tools.toml.example

cp -a ../services/proxy/. "$proxy_root/"
tar -czf "$proxy_artifact" -C "$tmp_artifact_dir" veilkey-proxy-local

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh init >/dev/null
go run ./tests/rewrite_manifest_urls.go "$tmp_manifest" "$cli_artifact" "$proxy_artifact"

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh validate >/dev/null
plan="$(VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh plan proxmox-host-cli)"
printf '%s\n' "$plan" | grep -F "cli" >/dev/null
printf '%s\n' "$plan" | grep -F "proxy" >/dev/null

VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh bundle proxmox-host-cli "$tmp_bundle" >/dev/null
VEILKEY_INSTALLER_MANIFEST="$tmp_manifest" ./install.sh install proxmox-host-cli "$tmp_root" "$tmp_bundle" >/dev/null

test -x "$tmp_root/usr/local/bin/veilkey-cli"
test -x "$tmp_root/usr/local/bin/veilkey-session-config"
test -x "$tmp_root/usr/local/bin/veil"
test -x "$tmp_root/usr/local/bin/vk"
test -f "$tmp_root/etc/veilkey/session-tools.toml.example"
test -x "$tmp_root/usr/local/bin/veilroot-shell"

echo "ok: cli component install layout"
