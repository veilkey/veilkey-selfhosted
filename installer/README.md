# VeilKey Installer

`installer` is the self-hosted VeilKey installation and packaging component.

It assembles tested component versions, stages runtime assets, renders host-specific scaffolding, and verifies that the installed node is healthy.

## Product Position

VeilKey is split into:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - `installer`
  - `keycenter`
  - `localvault`
  - `cli`
  - `proxy`

## Responsibilities

This component owns:

- install profiles
- component bundle layout
- Proxmox wrapper commands
- activation and health verification
- bootstrap export flows for initial installs

It does not own long-lived runtime business logic for KeyCenter, LocalVault, CLI, or Proxy.

## Canonical Targets

The active install targets are:

- `proxmox-host`
- `proxmox-host-cli`
- `proxmox-host-localvault`
- `proxmox-lxc-allinone`
- `proxmox-lxc-runtime`

## Main Documents

- `INSTALL.md`
  - operator install flow
- `docs/deployment-readiness.md`
  - deploy-time guardrails and preflight expectations
- `profiles/`
  - install target inputs
- `scripts/`
  - wrapper commands
- `validation-logs/`
  - command-object validation records
- post-install validation

This repository is not the source of truth for:

- KeyCenter runtime code
- LocalVault runtime code
- long-term mirrors of component repositories

## Canonical Profiles

Only the following profiles are active:

- `proxmox-host`
- `proxmox-host-cli`
- `proxmox-host-localvault`
- `proxmox-lxc-allinone`
- `proxmox-lxc-runtime`

## Canonical Runtime Shapes

### `proxmox-host-localvault`

Purpose:

- install a host-side LocalVault node
- connect it to KeyCenter
- support install, health, and purge with explicit wrappers

Validated flow:

- install
- activate
- KeyCenter registration
- purge
- KeyCenter unregister

### `proxmox-lxc-allinone`

Purpose:

- install an all-in-one LXC with both KeyCenter and LocalVault
- expose both services by IP
- generate bootstrap SSH material during first install
- export bootstrap public and encrypted artifacts to the host
- stage boundary assets needed for a host companion install

Important contract:

- `proxmox-lxc-allinone` is the LXC runtime/control-plane profile
- proxy runtime is not supported inside the all-in-one LXC
- if you need the Proxmox host boundary, install `proxmox-host-cli` separately on the host

Validated flow:

- fresh one-shot install
- IP access to KeyCenter and LocalVault
- LocalVault registration inside KeyCenter
- bootstrap export
- health
- purge

## Architecture Summary

```text
Operator / CLI / UI
        |
        v
   VeilKey KeyCenter
        |
        +---- LocalVault (node A)
        +---- LocalVault (node B)
        +---- LocalVault (host)
```

Installer provisions the runtime shape. KeyCenter and LocalVault carry the runtime state.

## Command Model

Top-level commands:

```bash
./install.sh init
./install.sh validate
./install.sh doctor
./install.sh detect-os
./install.sh profiles
./install.sh plan-install <profile> <root>
./install.sh download <profile>
./install.sh stage <profile>
./install.sh bundle <profile>
./install.sh install <profile> <root>
./install.sh configure <profile> <root>
./install.sh install-profile <profile> <root>
./install.sh plan-activate <root>
./install.sh activate
./install.sh post-install-health <root>
```

`doctor` warns when the active manifest still contains placeholder artifact URLs and `VEILKEY_INSTALLER_GITLAB_API_BASE` is unset.

Operator-facing wrappers:

```bash
./scripts/proxmox-host-install.sh /
./scripts/proxmox-host-localvault/install.sh --activate /
./scripts/proxmox-host-localvault/health.sh /
./scripts/proxmox-host-localvault/purge.sh /

./scripts/proxmox-lxc-allinone-install.sh --activate /
./scripts/proxmox-lxc-allinone-health.sh /
./scripts/proxmox-lxc-runtime-install.sh --activate /
./scripts/proxmox-lxc-runtime-health.sh /
./scripts/proxmox-lxc-allinone-purge.sh /
./scripts/proxmox-lxc-allinone-export-bootstrap.sh <vmid> [dest_root]
```

## Bundle and Install Model

The installer works in four stages:

1. plan
2. download
3. stage / bundle
4. install / configure / activate

Important behavior:

- `install-profile` reuses an existing `bundle_root` when present
- fresh installs can download artifacts directly from the URL already encoded in the active manifest
- the example manifest still uses placeholder URLs until release refs are filled in:
  - `keycenter` / `localvault` normalize through `VEILKEY_INSTALLER_GITLAB_API_BASE`
  - `cli` uses a GitHub release asset placeholder with `RELEASE_OR_TAG`
- `post-install-health` validates the installed scaffold
- wrapper commands add target-specific runtime checks on top
- `proxmox-lxc-allinone` stages boundary assets for host export and follow-up setup
- `proxmox-lxc-allinone` does not support `VEILKEY_ENABLE_PROXY=1` inside the LXC; use `proxmox-host-cli` on the Proxmox host for companion boundary/proxy runtime

For the `cli` component, the intended published path is:

```text
https://github.com/veilkey/veilkey-selfhosted/releases/download/<tag>/veilkey-cli_<tag>_linux_amd64.tar.gz
```

Until a real tag is published, `components.toml` must be rewritten or the focused local artifact test path should be used.

## Bootstrap SSH Export

`proxmox-lxc-allinone` generates bootstrap SSH material on first install:

- private key
- public key
- encrypted export

The runtime stores bootstrap material inside the LXC, and the export wrapper copies the public and encrypted artifacts to the host.

Validated host export layout:

- `/opt/veilkey/bootstrap-exports/<vmid>-<hostname>/veilkey-admin.pub`
- `/opt/veilkey/bootstrap-exports/<vmid>-<hostname>/veilkey-admin.enc`
- `/opt/veilkey/bootstrap-exports/<vmid>-<hostname>/manifest.json`

## Validation

Validation logs are tracked as command objects under:

- [`validation-logs/validated`](./validation-logs/validated)
- [`validation-logs/pending`](./validation-logs/pending)

Each validated object log records:

- command
- target
- expected result
- observed result
- observed time
- proof
- artifacts
- exit code

## Related Repositories

- `veilkey-keycenter`
  - control plane
- `veilkey-localvault`
  - node runtime

## Installation Guide

For copy-paste installation steps, see [`INSTALL.md`](./INSTALL.md).

## Fresh Proxmox LXC Smoke Path

This is the currently validated live install path on a Proxmox host:

```bash
cd installer
export VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4"
./install.sh init
./install.sh bundle proxmox-lxc-allinone /tmp/veilkey-allinone-bundle
./install.sh bundle proxmox-lxc-runtime /tmp/veilkey-runtime-bundle
```

Create a fresh Debian LXC, copy the installer tree and bundle into the container, then run:

```bash
export VEILKEY_ALLINONE_BUNDLE_ROOT=/root/veilkey-allinone-bundle

echo -n 'replace-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
echo -n 'replace-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
./scripts/proxmox-lxc-allinone-install.sh --activate / "${VEILKEY_ALLINONE_BUNDLE_ROOT}"

./scripts/proxmox-lxc-allinone-health.sh /
```

For a second LocalVault-only runtime LXC that registers into the all-in-one KeyCenter:

```bash
export VEILKEY_RUNTIME_BUNDLE_ROOT=/root/veilkey-runtime-bundle

echo -n 'replace-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
VEILKEY_KEYCENTER_URL='http://<allinone-ip>:10181' \
./scripts/proxmox-lxc-runtime-install.sh --activate / "${VEILKEY_RUNTIME_BUNDLE_ROOT}"

./scripts/proxmox-lxc-runtime-health.sh /
```

If you also want the Proxmox host companion boundary on the same operator path, bundle `proxmox-host-cli` and run the stack wrapper:

```bash
./install.sh bundle proxmox-host-cli /tmp/veilkey-host-cli-bundle
export VEILKEY_ALLINONE_BUNDLE_ROOT=/root/veilkey-allinone-bundle
export VEILKEY_HOST_CLI_BUNDLE_ROOT=/root/veilkey-host-cli-bundle

echo -n 'replace-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
echo -n 'replace-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
./scripts/proxmox-allinone-stack-install.sh / / "${VEILKEY_ALLINONE_BUNDLE_ROOT}" "${VEILKEY_HOST_CLI_BUNDLE_ROOT}"
```

This keeps the runtime split explicit:

- LXC runtime/control plane: `proxmox-lxc-allinone`
- Proxmox host companion boundary: `proxmox-host-cli`

## Status

Current canonical state:

- active installer profiles are Proxmox-only
- the active remote branch surface is `main`
- the active host path uses the installer and localvault source repositories only

## License

See [`LICENSE`](./LICENSE).
