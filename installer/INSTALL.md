# VeilKey Installer Quick Start

This document is the operator-facing install guide for the active VeilKey runtime.

It assumes:

- a Proxmox host
- network access to the internal GitLab artifact source
- root shell access on the target host or LXC

## Supported Install Targets

The currently validated install targets are:

- `proxmox-host-localvault`
- `proxmox-lxc-allinone`

## 1. Prepare the Installer Repository

Clone the installer repository and initialize the local manifest:

```bash
git clone git@<YOUR_GITLAB_HOST>:veilkey/veilkey-installer.git
cd veilkey-installer
export VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4"
./install.sh init
```

If `components.toml` already exists, the wrappers reuse it.
The example manifest keeps placeholder package URLs; `VEILKEY_INSTALLER_GITLAB_API_BASE` is required for fresh `bundle` or `download` commands unless you already rewrote the manifest URLs.

## 2. Install a Host LocalVault

Use this when you want a host-side LocalVault that reports to an existing KeyCenter.

Required input:

- `VEILKEY_LOCALVAULT_PASSWORD`
- `VEILKEY_KEYCENTER_URL`

Example:

```bash
# Write password to a file (never use env vars for passwords)
echo -n 'replace-me' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
export VEILKEY_LOCALVAULT_PASSWORD='replace-me'  # installer input only; store the source secret in a file
export VEILKEY_KEYCENTER_URL='https://<YOUR_KEYCENTER_HOST>'

./scripts/proxmox-host-localvault/install.sh --activate /
./scripts/proxmox-host-localvault/health.sh /
```

What this does:

- installs the `proxmox-host-localvault` profile
- initializes the LocalVault database if missing
- activates the systemd service
- verifies local health and KeyCenter registration

To remove it completely:

```bash
./scripts/proxmox-host-localvault/purge.sh /
```

`purge.sh` performs:

- KeyCenter unregister
- service stop and disable
- runtime data removal

## 3. Install a Proxmox LXC All-in-One Node

Use this when you want a single LXC with:

- KeyCenter
- LocalVault
- bootstrap SSH generation

Required input:

- `VEILKEY_KEYCENTER_PASSWORD`
- `VEILKEY_LOCALVAULT_PASSWORD`

Example:

```bash
echo -n 'replace-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
export VEILKEY_KEYCENTER_PASSWORD='replace-keycenter-password'
echo -n 'replace-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
export VEILKEY_LOCALVAULT_PASSWORD='replace-localvault-password'

./scripts/proxmox-lxc-allinone-install.sh --activate /
./scripts/proxmox-lxc-allinone-health.sh /
```

Validated runtime ports:

- KeyCenter: `10181`
- LocalVault: `10180`

Important behavior:

- the installer writes password files under `/etc/veilkey/*.password` and runtime env files use `VEILKEY_PASSWORD_FILE`
- proxy assets are installed, but proxy units are not enabled unless you set `VEILKEY_ENABLE_PROXY=1`
- a fresh live LXC install needs `curl`, `openssl`, and `ssh-keygen`; the all-in-one wrapper installs them with `apt-get` when missing

## 4. Verify IP Access for All-in-One

After installation, verify both services by IP:

```bash
curl http://<lxc-ip>:10181/health
curl http://<lxc-ip>:10180/health
```

Expected result:

- both endpoints return an `ok` health response

If you need to inspect agent registration immediately, unlock KeyCenter once and restart LocalVault to force a fresh heartbeat:

```bash
curl -X POST http://127.0.0.1:10181/api/unlock \
  -H 'Content-Type: application/json' \
  --data '{"password":"replace-keycenter-password"}'
systemctl restart veilkey-localvault.service
curl http://127.0.0.1:10181/api/agents
```

## 5. Install a Runtime-Only LocalVault LXC

Use this when you want a second LXC with LocalVault only, bound to an existing KeyCenter.

Required input:

- `VEILKEY_LOCALVAULT_PASSWORD`
- `VEILKEY_KEYCENTER_URL`

Example:

```bash
echo -n 'replace-runtime-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
export VEILKEY_LOCALVAULT_PASSWORD='replace-runtime-localvault-password'
export VEILKEY_KEYCENTER_URL='http://<allinone-ip>:10181'

./scripts/proxmox-lxc-runtime-install.sh --activate /
./scripts/proxmox-lxc-runtime-health.sh /
```

## 6. Export Bootstrap SSH Artifacts

The all-in-one install generates bootstrap SSH material on first install.

To export the public and encrypted artifacts to the host:

```bash
./scripts/proxmox-lxc-allinone-export-bootstrap.sh <vmid>
```

Default host output layout:

```text
/opt/veilkey/bootstrap-exports/<vmid>-<hostname>/
  veilkey-admin.pub
  veilkey-admin.enc
  manifest.json
```

The private key remains inside the LXC.

## 7. Remove an All-in-One Node

To remove the all-in-one runtime from inside the LXC:

```bash
./scripts/proxmox-lxc-allinone-purge.sh /
```

This removes:

- KeyCenter runtime data
- LocalVault runtime data
- bootstrap SSH artifacts stored inside the LXC
- active services

It does not automatically remove host-side exported bootstrap artifacts.

## Troubleshooting

### Missing `components.toml`

The wrappers bootstrap `components.toml` from `components.toml.example` if needed.

### Fresh install download path

Fresh installs use the internal GitLab HTTPS source.
If downloads fail, verify:

- GitLab availability
- internal routing to the GitLab artifact source
- `VEILKEY_INSTALLER_GITLAB_API_BASE` is set to the active GitLab API base

### Existing bundle reuse

If a bundle already exists, `install-profile` reuses it instead of forcing a redownload.

## Canonical Wrapper Commands

Host LocalVault:

```bash
./scripts/proxmox-host-localvault/install.sh --activate /
./scripts/proxmox-host-localvault/health.sh /
./scripts/proxmox-host-localvault/purge.sh /
```

All-in-one LXC:

```bash
./scripts/proxmox-lxc-allinone-install.sh --activate /
./scripts/proxmox-lxc-allinone-health.sh /
./scripts/proxmox-lxc-allinone-export-bootstrap.sh <vmid>
./scripts/proxmox-lxc-allinone-purge.sh /
```

Runtime LXC:

```bash
./scripts/proxmox-lxc-runtime-install.sh --activate /
./scripts/proxmox-lxc-runtime-health.sh /
```
