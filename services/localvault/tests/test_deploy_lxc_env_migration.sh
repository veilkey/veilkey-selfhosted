#!/bin/bash
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

env_file="${tmp_dir}/veilkey-env"
cat > "${env_file}" <<EOF
VEILKEY_PASSWORD=secret
VEILKEY_ADDR=:10180
VEILKEY_DB_PATH=/opt/veilkey-localvault/data/veilkey.db
EOF

source_script='
  source "'"${REPO_ROOT}"'/scripts/deploy-lxc.sh"
  ensure_vaultcenter_env 100207 "'"${env_file}"'"
'

fake_bin="${tmp_dir}/bin"
mkdir -p "${fake_bin}"
cat > "${fake_bin}/vibe_lxc_ops" <<EOF
#!/bin/bash
set -euo pipefail
cmd="\${@: -1}"
bash -lc "\$cmd"
EOF
chmod +x "${fake_bin}/vibe_lxc_ops"

PATH="${fake_bin}:$PATH" VEILKEY_SOURCE_ONLY=1 bash -c "${source_script}"

grep -q "^VEILKEY_VAULTCENTER_URL=$" "${env_file}"

count="$(grep -c '^VEILKEY_VAULTCENTER_URL=' "${env_file}")"
[[ "${count}" -eq 1 ]]

echo "ok"
