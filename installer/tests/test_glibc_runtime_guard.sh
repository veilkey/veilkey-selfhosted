#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

tmp_root="$(mktemp -d)"
tmp_bundle="$(mktemp -d)"
tmp_bundle_metadata="$(mktemp -d)"
tmp_manifest="$(mktemp)"
tmp_component_src="$(mktemp -d)"
tmp_metadata_src="$(mktemp -d)"
tmp_err="$(mktemp)"
trap 'rm -rf "$tmp_root" "$tmp_bundle" "$tmp_bundle_metadata" "$tmp_component_src" "$tmp_metadata_src"; rm -f "$tmp_manifest" "$tmp_err"' EXIT

cat > "${tmp_component_src}/veilkey-localvault" <<'EOF'
#!/usr/bin/env bash
# GLIBC_2.38
exit 0
EOF
chmod +x "${tmp_component_src}/veilkey-localvault"
tar -C "${tmp_component_src}" -czf "${tmp_bundle}/localvault-compat.tar.gz" .

artifact_url="file://${tmp_bundle}/localvault-compat.tar.gz"
artifact_sha256="$(sha256sum "${tmp_bundle}/localvault-compat.tar.gz" | awk '{print $1}')"

cat > "${tmp_manifest}" <<EOF
[release]
name = "glibc-compat-test"
version = "test"
channel = "dev"

[components.localvault]
source = "file"
project = "local/localvault"
ref = "test"
type = "binary"
install_order = 10
artifact_url = "${artifact_url}"
artifact_filename = "localvault-compat.tar.gz"
sha256 = "${artifact_sha256}"

[profiles.proxmox-host-localvault]
description = "glibc compatibility guard test"
components = ["localvault"]
EOF

if VEILKEY_INSTALLER_TARGET_GLIBC_VERSION=2.36 \
  VEILKEY_INSTALLER_MANIFEST="${tmp_manifest}" \
  ./install.sh install-profile proxmox-host-localvault "${tmp_root}" "${tmp_bundle}" > /dev/null 2>"${tmp_err}"; then
  echo "expected glibc compatibility guard to reject install" >&2
  exit 1
fi

grep -F "component localvault requires glibc 2.38, but target runtime provides 2.36" "${tmp_err}" >/dev/null

VEILKEY_INSTALLER_TARGET_GLIBC_VERSION=2.38 \
  VEILKEY_INSTALLER_MANIFEST="${tmp_manifest}" \
  ./install.sh install-profile proxmox-host-localvault "${tmp_root}" "${tmp_bundle}" >/dev/null

test -x "${tmp_root}/usr/local/bin/veilkey-localvault"

cat > "${tmp_metadata_src}/veilkey-localvault" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "${tmp_metadata_src}/veilkey-localvault"
cat > "${tmp_metadata_src}/artifact-runtime.env" <<'EOF'
VEILKEY_RUNTIME_GLIBC_MIN=2.39
EOF
tar -C "${tmp_metadata_src}" -czf "${tmp_bundle}/localvault-compat-metadata.tar.gz" .

artifact_url="file://${tmp_bundle}/localvault-compat-metadata.tar.gz"
artifact_sha256="$(sha256sum "${tmp_bundle}/localvault-compat-metadata.tar.gz" | awk '{print $1}')"

cat > "${tmp_manifest}" <<EOF
[release]
name = "glibc-compat-test"
version = "test"
channel = "dev"

[components.localvault]
source = "file"
project = "local/localvault"
ref = "test"
type = "binary"
install_order = 10
artifact_url = "${artifact_url}"
artifact_filename = "localvault-compat-metadata.tar.gz"
sha256 = "${artifact_sha256}"

[profiles.proxmox-host-localvault]
description = "glibc compatibility guard test"
components = ["localvault"]
EOF

if VEILKEY_INSTALLER_TARGET_GLIBC_VERSION=2.38 \
  VEILKEY_INSTALLER_MANIFEST="${tmp_manifest}" \
  ./install.sh install-profile proxmox-host-localvault "${tmp_root}" "${tmp_bundle_metadata}" > /dev/null 2>"${tmp_err}"; then
  echo "expected metadata glibc compatibility guard to reject install" >&2
  exit 1
fi

grep -F "component localvault requires glibc 2.39, but target runtime provides 2.38" "${tmp_err}" >/dev/null

echo "ok: glibc runtime guard"
