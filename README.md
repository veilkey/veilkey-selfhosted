# VeilKey Self-Hosted

`veilkey-selfhosted` is a unified source tree for the self-hosted VeilKey product surface.

It groups the active self-hosted components under one workspace while keeping installation, runtime services, and operator-facing clients separated by responsibility.

## Product Split

VeilKey is organized in two product domains:

- `managed`
  - `veilkey-docs`
  - `veilkey-homepage`
- `self-hosted`
  - `installer`
  - `keycenter`
  - `localvault`
  - `cli`
  - `proxy`

This repository contains only the `self-hosted` domain.

## Layout

- `installer/`
  - packaging, install profiles, Proxmox wrappers, health checks
  - proxmox-lxc-allinone: LXC runtime (KeyCenter + LocalVault)
  - proxmox-host-cli: Proxmox host companion boundary
  - proxmox-allinone-stack-install.sh: combines both in one operator step
- `services/`
  - runtime services
  - `keycenter/`
  - `localvault/`
  - `proxy/`
- `client/`
  - operator-facing surfaces
  - `cli/`

## Runtime Model

The active runtime model is:

- `services/keycenter`
  - central control plane
- `services/localvault`
  - node-local agent
- `client/cli`
  - operator-facing entrypoint
- `services/proxy`
  - outbound enforcement layer
- `installer`
  - installation and verification layer

## Scope

This repository is intended to keep the self-hosted VeilKey surface in one place without flattening component responsibilities.

Each top-level area remains responsible for its own source, tests, and operational contracts.
