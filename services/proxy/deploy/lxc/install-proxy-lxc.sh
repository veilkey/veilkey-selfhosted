#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
vmid="${1:-}"

if [[ -z "$vmid" ]]; then
  echo "usage: $(basename "$0") <vmid>" >&2
  exit 2
fi

bin_src="${VEILKEY_PROXY_CLI_SRC:-/opt/veilkey/artifacts/hostvault/veilkey-cli}"
config_src="${VEILKEY_PROXY_POLICY_SRC:?VEILKEY_PROXY_POLICY_SRC must be set (see policy/proxy-profiles.toml.example)}"
access_log_format="${VEILKEY_PROXY_ACCESS_LOG_FORMAT:-jsonl}"

if [[ ! -f "$bin_src" ]]; then
  echo "missing veilkey-cli binary: $bin_src" >&2
  exit 1
fi

if [[ ! -f "$config_src" ]]; then
  echo "missing proxy policy: $config_src" >&2
  exit 1
fi

ensure_go() {
  if command -v go >/dev/null 2>&1; then
    return 0
  fi
  if command -v apt-get >/dev/null 2>&1; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update >/dev/null
    apt-get install -y golang-go >/dev/null
    return 0
  fi
  echo "go is missing and no supported package manager (apt-get) was found" >&2
  exit 1
}

bundle_dir="$(mktemp -d /tmp/veilkey-proxy-lxc.XXXXXX)"
cleanup() {
  rm -rf "$bundle_dir"
}
trap cleanup EXIT

mkdir -p \
  "$bundle_dir/usr/local/bin" \
  "$bundle_dir/etc/systemd/system" \
  "$bundle_dir/etc/veilkey" \
  "$bundle_dir/usr/local/lib/veilkey-proxy"

ensure_go
install -m 0755 "$bin_src" "$bundle_dir/usr/local/bin/veilkey-cli"
go build -o "$bundle_dir/usr/local/bin/veilkey-session-config" "$repo_root/cmd/veilkey-session-config"
install -m 0755 "$repo_root/deploy/lxc/veilkey-proxy-launch" "$bundle_dir/usr/local/bin/veilkey-proxy-launch"
install -m 0644 "$repo_root/deploy/lxc/veilkey-egress-proxy@.service" "$bundle_dir/etc/systemd/system/veilkey-egress-proxy@.service"
install -m 0644 "$config_src" "$bundle_dir/etc/veilkey/session-tools.toml"
install -m 0755 "$repo_root/deploy/lxc/verify-proxy-lxc.sh" "$bundle_dir/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh"
localvault_url="${VEILKEY_LOCALVAULT_URL:-$("$bundle_dir/usr/local/bin/veilkey-session-config" --config "$bundle_dir/etc/veilkey/session-tools.toml" veilkey-localvault-url)}"
hub_url="${VEILKEY_HUB_URL:-$("$bundle_dir/usr/local/bin/veilkey-session-config" --config "$bundle_dir/etc/veilkey/session-tools.toml" veilkey-hub-url)}"
hostvault_url="${VEILKEY_HOSTVAULT_URL:-$("$bundle_dir/usr/local/bin/veilkey-session-config" --config "$bundle_dir/etc/veilkey/session-tools.toml" veilkey-hostvault-url)}"
cat >"$bundle_dir/etc/veilkey/proxy.env" <<EOF
VEILKEY_LOCALVAULT_URL=$localvault_url
VEILKEY_HUB_URL=$hub_url
VEILKEY_HOSTVAULT_URL=$hostvault_url
VEILKEY_PROXY_ACCESS_LOG_FORMAT=$access_log_format
EOF

vibe_lxc_ops "$vmid" "apt-get update && apt-get install -y ca-certificates curl jq tmux"
vibe_lxc_ops "$vmid" "mkdir -p /etc/veilkey /var/log/veilkey-proxy /usr/local/lib/veilkey-proxy"
vibe_lxc_ops "$vmid" "systemctl stop veilkey-egress-proxy@default.service veilkey-egress-proxy@codex.service veilkey-egress-proxy@claude.service veilkey-egress-proxy@opencode.service 2>/dev/null || true"

mapfile -t bundle_files < <(find "$bundle_dir" -type f | sort)
for file in "${bundle_files[@]}"; do
  rel="${file#"$bundle_dir"/}"
  if ! pct push "$vmid" "$file" "/$rel"; then
    echo "failed to push $rel into LXC $vmid" >&2
    exit 1
  fi
done

vibe_lxc_ops --stdin "$vmid" <<'EOF'
set -euo pipefail
chmod 0755 /usr/local/bin/veilkey-cli /usr/local/bin/veilkey-session-config /usr/local/bin/veilkey-proxy-launch /usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh
systemctl daemon-reload
systemctl enable --now veilkey-egress-proxy@default.service veilkey-egress-proxy@codex.service veilkey-egress-proxy@claude.service veilkey-egress-proxy@opencode.service
EOF

echo "installed veilkey-proxy services into LXC $vmid"
echo "next:"
echo "  vibe_lxc_ops $vmid '/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh'"
