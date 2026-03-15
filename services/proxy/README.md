# VeilKey Proxy

`proxy` is the self-hosted VeilKey outbound enforcement component.

It provides the network boundary for wrapped workloads, rewrite auditing, and egress control profiles.

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

- outbound proxy runtime
- rewrite auditing
- egress policy profiles
- LXC proxy deployment assets
- host-side proxy verification tools

## Runtime Role

The proxy is the enforcement layer behind CLI-driven session boundaries. It is not the primary operator entrypoint.

Operator-facing boundary scripts belong to `cli`. Outbound enforcement belongs here.

## Related Components

- `cli`
  - operator entrypoint and `veilroot` host boundary
- `localvault`
  - local secret and config runtime
- `keycenter`
  - control-plane orchestration

## Quick Start

### 1. Build and test

```bash
go test ./...
bash tests/run-shell-tests.sh
```

### 2. Install the host boundary

Veilroot boundary:

```bash
./deploy/host/install-veilroot-boundary.sh
```

Optional locale override:

```bash
VEILKEY_LOCALE=ko veilroot status
VEILKEY_LOCALE=en veilroot status
```

### 3. Install the proxy LXC runtime

```bash
./deploy/lxc/install-proxy-lxc.sh 100208
vibe_lxc_ops 100208 '/usr/local/lib/veilkey-proxy/verify-proxy-lxc.sh'
```

### 4. Verify the end-to-end boundary

```bash
/usr/local/bin/verify-veilroot-session codex
./deploy/host/doctor-veilkey.sh
```

## Usage

### Session / wrap model

Canonical operator entrypoints:

```bash
veilkey session <command> [args...]
veilkey wrap <command> [args...]
```

The host installers in this repository wire user-facing launchers to that model instead of exposing ad hoc proxy environment setup.

### Observe and debug

```bash
go run ./cmd/veilkey-proxy doctor
go run ./cmd/veilkey-proxy observe --once
go run ./cmd/veilkey-proxy observe --uid 1008 --format json
go run ./cmd/veilkey-proxy observe --uid 1008 --format json --only-suspicious
go run ./cmd/veilkey-proxy observe --uid 1008 --format json --only-suspicious --enforce-kill
```

### Veilroot tooling

```bash
/usr/local/bin/veilroot status
/usr/local/bin/veilkey-veilroot-session codex
/usr/local/bin/veilkey-veilroot-observe codex --once
/usr/local/bin/veilkey-veilroot-egress-guard codex apply
systemctl enable --now veilkey-veilroot-observe@codex.service
systemctl enable --now veilkey-veilroot-egress-guard@codex.service
```

`veilroot status` and `veilroot-shell status` support `VEILKEY_LOCALE=ko|en` and show:

- VeilKey connection state
- VeilKey Proxy Guard enabled/disabled
- VeilKey Observer connected/disconnected

## Verification

### Read-only doctor

```bash
./deploy/host/doctor-veilkey.sh
```

Checks:

- LocalVault, KeyCenter, and HostVault health
- veilroot session boundary
- veilroot observer and egress guard unit state
- recent rewrite log sanity for all profiles

### TEMP issuance validation

```bash
./deploy/host/doctor-veilkey.sh --check-temp-issuance
```

This performs a real temporary secret issuance and confirms the returned ref is `VK:TEMP:*`. Run it only when you explicitly want a state-changing check.

## Host Deployment Surface

- Template config: [`deploy/host/session-tools.toml.example`](deploy/host/session-tools.toml.example)
- Veilroot installer: [`deploy/host/install-veilroot-boundary.sh`](deploy/host/install-veilroot-boundary.sh)
- Veilroot launcher: [`deploy/host/veilkey-veilroot-session`](deploy/host/veilkey-veilroot-session)
- Veilroot observer: [`deploy/host/veilkey-veilroot-observe`](deploy/host/veilkey-veilroot-observe)
- Veilroot guard: [`deploy/host/veilkey-veilroot-egress-guard`](deploy/host/veilkey-veilroot-egress-guard)
- Full doctor: [`deploy/host/doctor-veilkey.sh`](deploy/host/doctor-veilkey.sh)

## LXC Deployment Surface

- Installer: [`deploy/lxc/install-proxy-lxc.sh`](deploy/lxc/install-proxy-lxc.sh)
- Launcher: [`deploy/lxc/veilkey-proxy-launch`](deploy/lxc/veilkey-proxy-launch)
- Unit: [`deploy/lxc/veilkey-egress-proxy@.service`](deploy/lxc/veilkey-egress-proxy@.service)
- Verification: [`deploy/lxc/verify-proxy-lxc.sh`](deploy/lxc/verify-proxy-lxc.sh)

## Policy and Logging

### Proxy Profiles

Port mappings per profile:

- `default` -> `18080`
- `codex` -> `18081`
- `opencode` -> `18083`
- `claude` -> `18084`

- Default plaintext action: `issue-temp-and-block`
- Exception hosts are defined per profile in [`policy/proxy-profiles.toml`](policy/proxy-profiles.toml)

### Policy SSOT

- Policy source: [`policy/proxy-profiles.toml`](policy/proxy-profiles.toml)
- Host runtime config is rendered from policy, not treated as SSOT
- LXC runtime config is rendered from the same source
- Proxy runtime env source: `/etc/veilkey/proxy.env`

Key runtime env values:

- `VEILKEY_LOCALVAULT_URL`: resolve endpoint
- `VEILKEY_HUB_URL`: temp issue endpoint
- `VEILKEY_PROXY_ACCESS_LOG_FORMAT`: access log format
- `VEILKEY_PROXY_REWRITE_LOG`: plaintext detection and rewrite audit path

### Rewrite scope

Current request surfaces:

- `headers`
- `query`
- `body`

Current rewriteable body content types:

- `application/json`
- `application/x-www-form-urlencoded`
- `text/plain`

Behavior:

- Detect secret-like plaintext values
- In hardened mode, issue `VK:TEMP:*` and block outbound plaintext
- Allow per-profile resolve-only exceptions with `plaintext_resolve_hosts`
- `HTTPS CONNECT` body inspection is currently out of scope

### Logs

- Access log: `/var/log/veilkey-proxy/<profile>.jsonl`
- Rewrite audit log: `/var/log/veilkey-proxy/<profile>-rewrite.jsonl`
- Stderr log: `/var/log/veilkey-proxy/<profile>.log`
- Cleanup/archive helper: [`deploy/host/cleanup-proxy-logs.sh`](deploy/host/cleanup-proxy-logs.sh)

## Current Status

- Linux collector preflight is implemented
- root requirement / BPF fs / target cgroup validation is implemented
- actual `execve` observation is implemented via an in-repo tracepoint + ringbuf eBPF program
- actual `connect` observation is implemented via a tracepoint + ringbuf BPF program
- user space filtering by uid and cgroup path is implemented
- plaintext secret-like argv detection is implemented
- suspicious `execve` kill enforcement is implemented

## Next

- Stabilize kernel-side `execve` error logging
- Push more UID/cgroup filtering deeper into kernel-side paths where practical
- Expand outbound policy evaluation and enforcement stages

## License

GNU Affero General Public License v3.0 (AGPL-3.0)

See [`AGPL-COMPLIANCE.md`](AGPL-COMPLIANCE.md) for the operator checklist used for deployed network services.
