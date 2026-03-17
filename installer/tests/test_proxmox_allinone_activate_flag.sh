#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
tmp_dir="$(mktemp -d)"
tmp_root="$(mktemp -d)"
tmp_bundle="$(mktemp -d)"
trap 'rm -rf "$tmp_dir" "$tmp_root" "$tmp_bundle"' EXIT

mkdir -p "$tmp_dir/scripts"
cp "$repo_root/scripts/proxmox-lxc-allinone-install.sh" "$tmp_dir/scripts/"
cp "$repo_root/components.toml.example" "$tmp_dir/components.toml"

cat > "$tmp_dir/install.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >> "${VEILKEY_TEST_LOG}"
EOF
chmod +x "$tmp_dir/install.sh"

log_file="$tmp_dir/install.log"
VEILKEY_TEST_LOG="$log_file" \
VEILKEY_KEYCENTER_PASSWORD='test-keycenter' \
VEILKEY_LOCALVAULT_PASSWORD='test-localvault' \
  "$tmp_dir/scripts/proxmox-lxc-allinone-install.sh" --activate "$tmp_root" "$tmp_bundle" >/dev/null

test -f "$log_file"
line1="$(sed -n '1p' "$log_file")"
line2="$(sed -n '2p' "$log_file")"

printf '%s\n' "$line1" | grep -F "install-profile proxmox-lxc-allinone $tmp_root $tmp_bundle" >/dev/null
if printf '%s\n' "$line1" | grep -F -- '--activate' >/dev/null; then
  echo "unexpected --activate forwarded to install-profile" >&2
  exit 1
fi
printf '%s\n' "$line2" | grep -F "activate $tmp_root" >/dev/null
test "$(wc -l < "$log_file")" -eq 2

echo "ok: proxmox all-in-one wrapper keeps --activate out of install-profile"
