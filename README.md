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

## Workspace and Session Policy

For live install, deploy, LXC, or Proxmox validation work, separate these roles by default:

- user session
  - primary operator conversation and ad-hoc inspection
- work session
  - claimed live-operation shell used for actual `/` installs, service restarts, and exports
- delegate session
  - separate Claude/Codex tmux session bound to a separate clone or workspace

Do not mix live runtime operations into a dirty primary tree when a separate work workspace is available.

For delegated Claude work, prefer the VeilKey task wrapper instead of raw `tmux send-keys`:

- `shared/scripts/send-veilkey-claude-task.sh`

This wrapper only accepts prompts that follow the VeilKey task section format:

- `VeilKey Task:`
- `Workspace:`
- `Goal:`
- `Scope:`
- `Constraints:`
- `Deliverables:`
- `Reply:`

## GitHub Mirror Notes

When syncing into the `veilkey` GitHub organization, keep these canonical document entrypoints aligned:

- `installer/README.md` and `installer/INSTALL.md` for fresh install and completion flow
- `services/keycenter/README.md` for controlled update flow
- component READMEs for service-specific runtime contracts
