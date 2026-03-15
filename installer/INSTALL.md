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
git clone git@10.60.100.210:veilkey/veilkey-installer.git
cd veilkey-installer
./install.sh init
```

If `components.toml` already exists, the wrappers reuse it.

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
export VEILKEY_LOCALVAULT_PASSWORD='replace-me'  # only for installer; written to file, not kept in env
export VEILKEY_KEYCENTER_URL='https://keycenter.60.internal.kr'

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

## 4. Verify IP Access for All-in-One

After installation, verify both services by IP:

```bash
curl http://<lxc-ip>:10181/health
curl http://<lxc-ip>:10180/health
```

Expected result:

- both endpoints return an `ok` health response

## 5. Export Bootstrap SSH Artifacts

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

## 6. Remove an All-in-One Node

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
