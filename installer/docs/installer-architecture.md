# Installer Architecture

## Scope

`veilkey-installer` is the installation and packaging layer for the active VeilKey runtime.

It assembles component versions, stages artifacts, installs runtime files, renders environment scaffolding, and validates the resulting target.

## Active Runtime Components

- `veilkey-keycenter`
  - central control plane
- `veilkey-localvault`
  - node-local runtime
- `veilkey-proxy`
  - boundary and proxy asset bundle

## Active Profiles

- `proxmox-host`
  - proxy boundary assets on a Proxmox host
- `proxmox-host-cli`
  - host-side boundary CLI and proxy assets
- `proxmox-host-localvault`
  - LocalVault on a Proxmox host
- `proxmox-lxc-runtime`
  - LocalVault runtime in a Proxmox LXC
- `proxmox-lxc-allinone`
  - KeyCenter + LocalVault in one Proxmox LXC, with staged boundary assets for host companion setup

## Install Model

The installer operates in this order:

1. validate manifest
2. plan
3. download
4. stage or bundle
5. install
6. configure
7. activate
8. post-install health

## Runtime Layout

Typical install destinations:

- `/opt/veilkey/<component>/`
- `/usr/local/bin/`
- `/etc/veilkey/`
- `/etc/systemd/system/`

## Wrapper Commands

Active operator wrappers:

- `scripts/proxmox-host-install.sh`
- `scripts/proxmox-allinone-stack-install.sh`
- `scripts/proxmox-host-cli-install.sh`
- `scripts/proxmox-host-localvault/install.sh`
- `scripts/proxmox-host-localvault/health.sh`
- `scripts/proxmox-host-localvault/purge.sh`
- `scripts/proxmox-lxc-runtime-install.sh`
- `scripts/proxmox-lxc-allinone-install.sh`
- `scripts/proxmox-lxc-allinone-health.sh`
- `scripts/proxmox-lxc-allinone-export-bootstrap.sh`
- `scripts/proxmox-lxc-allinone-purge.sh`

## Validation

The installer keeps command-object validation logs under:

- `validation-logs/validated`
- `validation-logs/pending`

Each validated object captures:

- command
- target
- expected result
- observed result
- proof
- artifacts
- exit code
