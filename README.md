<div align="center">
  <h1>VeilKey Self-Hosted</h1>
  <p><strong>Self-hosted secret lifecycle control and node-local execution boundary for operator-managed AI infrastructure.</strong></p>
  <p>
    <a href="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml"><img src="https://github.com/veilkey/veilkey-selfhosted/actions/workflows/ci.yml/badge.svg" alt="CI status"></a>
    <a href="https://github.com/veilkey/veilkey-selfhosted/releases"><img src="https://img.shields.io/github/v/release/veilkey/veilkey-selfhosted?display_name=tag" alt="GitHub release"></a>
    <a href="https://veil-key.com"><img src="https://img.shields.io/badge/home-veil--key.com-0f766e?logo=googlechrome&logoColor=white" alt="Website"></a>
    <a href="./LICENSE"><img src="https://img.shields.io/badge/license-AGPL--3.0-2563eb.svg" alt="AGPL-3.0"></a>
  </p>
</div>

`veilkey-selfhosted` is the self-hosted VeilKey product tree for secret lifecycle control, node-local runtime enforcement, and operator-managed infrastructure.

It packages the runtime services, installer, and operator CLI needed to run VeilKey on your own infrastructure instead of relying on a hosted control plane.

Korean summary: [`README.ko.md`](./README.ko.md)

Canonical domain: `https://veil-key.com`

## What VeilKey Is

VeilKey is a self-hosted secret and execution-boundary system for local AI and operator workflows.

The active runtime model is:

- `services/vaultcenter`
  - central control plane
- `services/localvault`
  - node-local runtime
- `client/cli`
  - operator entrypoint
- `services/proxy`
  - outbound enforcement layer
- `installer`
  - installation and verification layer

For operator entrypoints, the current split is:

- `veil`
  - protected session entrypoint
- `veilkey`
  - state, crypto, and policy control surface inside or alongside that session
- `veilkey-cli`
  - lower-level implementation binary used by wrappers and install/runtime scripts

## Core Logic

The shortest mental model is:

1. `VaultCenter` owns central policy and catalog decisions.
2. multiple `LocalVault` nodes run close to workloads, often inside separate hosts or containers.
3. operators use the CLI and install flows to register, inspect, and update those nodes.
4. runtime changes are pushed outward by policy, heartbeat, tracked-ref sync, and bulk-apply flows.

In practice, the shape looks like this:

```text
operator / cli
      |
      v
  VaultCenter
      |
      +---- LocalVault (container A)
      +---- LocalVault (container B)
      +---- LocalVault (host node)
```

The important split is:

- `VaultCenter`
  - central control plane
  - global catalog and policy decisions
  - central view of nodes, bindings, audit, and bulk operations
- `LocalVault`
  - node-local runtime
  - ciphertext and config storage
  - heartbeat and runtime identity
  - local execution under VaultCenter policy

## Central Management Model

VeilKey is designed so that keys and runtime state can be managed centrally while execution still happens at the node edge.

That includes:

- central registration of LocalVault nodes into VaultCenter
- central visibility into vault identity and runtime binding
- bulk-apply and workflow-style changes pushed from VaultCenter toward multiple LocalVault nodes
- planned rotation and rebind flows instead of ad-hoc per-node drift

This is the reason the repository contains both central and node-local components in one self-hosted tree.

## Key Version And Rotation Model

The key-management story is not just “store a secret once”.

The important runtime concepts are:

- `key_version`
  - the current cryptographic/runtime version tracked by the node
- `vault_hash`
  - stable vault identifier
- `vault_runtime_hash`
  - current VaultCenter runtime binding hash
- `managed_paths`
  - reported runtime ownership context, not the identity itself

Operationally, the flow is:

1. a LocalVault node registers and heartbeats to VaultCenter
2. VaultCenter can require rotation or rebind
3. LocalVault applies the new `key_version`
4. LocalVault retries heartbeat and reports the updated runtime binding

So the model is:

- central control over version and policy
- local execution and state ownership at each LocalVault node
- explicit rebind and rotation instead of silent key drift

## Why Self-Hosted

VeilKey is self-hosted because the main value is control over:

- where ciphertext and runtime state live
- how node identity and policy are enforced
- how containers are provisioned and managed
- how secrets are handled inside your own trust boundary

If you need a hosted SaaS secret manager, this repository is not that.
If you need VeilKey to live on your own host and network boundary, this repository is the right surface.

## Quick Start

The fastest operator path is the installer.

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted/installer
./install.sh validate
```

Then run the installer for your target:

```bash
./install.sh install-profile linux-host /
```

The minimum success check should look like this:

```bash
curl http://127.0.0.1:10181/health
curl http://127.0.0.1:10180/health
```

Expected result:

- VaultCenter health responds
- LocalVault health responds
- the node can heartbeat and appear in the central view after registration

### What Success Looks Like

VaultCenter starts in a locked state and becomes usable after unlock:

```json
GET /health
{"status":"locked"}
```

```json
POST /api/unlock
{"status":"unlocked"}
```

```json
GET /health
{"status":"ok"}
```

For LocalVault, the operational path should produce explicit lifecycle output:

```text
heartbeat sent
rotation applied and heartbeat sent
rebind prepared with key_version=9
```

The full operator guide lives in [`installer/INSTALL.md`](./installer/INSTALL.md).

## Operator Path

The operator path is easiest to understand as one control path and one execution path.

```text
operator / CLI
      |
      v
  VaultCenter API
      |
      +---- policy / catalog / bulk apply
      |
      +---- LocalVault node A
      |         |
      |         +---- ciphertext + runtime context
      |         +---- heartbeat / rebind / rotation
      |         +---- wrapped execution boundary
      |
      +---- LocalVault node B
                |
                +---- same node-local model
```

The outbound enforcement edge is where `services/proxy` belongs. The intended split is:

- `VaultCenter`
  - central policy, registration, audit, bulk operations
- `LocalVault`
  - node-local runtime, ciphertext/context, execution boundary

## Veil Session UX

The intended CLI UX is:

```bash
veil
claude
codex
```

With management commands handled by `veilkey`:

```bash
veilkey status
veilkey paste-mode on
veilkey resolve VK:LOCAL:example
```

This keeps `veil` focused on entering the protected environment and `veilkey` focused on crypto and policy actions.
- `proxy`
  - outbound enforcement surface when runtime traffic must be mediated

Additional operating notes live in [`docs/OPERATING-MODEL.md`](./docs/OPERATING-MODEL.md).

## Main Use Cases

- run VaultCenter and LocalVault inside your own Proxmox environment
- keep node-local runtime state under your own control
- use LocalVault as the node-local runtime paired with a central VaultCenter
- stage boundary and bootstrap assets for host companion setups

## How To Read This Repository

- `installer/`
  - install profiles, wrappers, health checks, and packaging
- `services/vaultcenter/`
  - central control plane
- `services/localvault/`
  - node-local runtime
- `services/proxy/`
  - outbound enforcement layer
- `client/cli/`
  - operator-facing CLI

## Why It Exists As One Repo

This repository keeps the self-hosted VeilKey surface in one place without flattening component responsibilities.

That means:

- install flow changes can ship with runtime changes
- operator docs can stay next to the code they describe
- CI can validate the self-hosted product as one surface

## Comparison Frame

VeilKey is not trying to be a generic password manager or a hosted secret vault.

The practical difference is:

- stronger emphasis on self-hosted runtime identity and node registration
- explicit Docker container install paths
- local runtime components such as LocalVault instead of a cloud-only model
- tighter install-to-runtime contract inside one source tree
- central VaultCenter + multiple LocalVault runtime topology instead of a single hosted vault model

### What It Is Not

- not a hosted SaaS secret manager
- not a generic password vault for personal use
- not a single-binary local secret toy
- not a cloud-only control plane detached from node runtime

### Tradeoffs

- higher operational complexity than a hosted secret service
- stronger dependency on your own host and network setup
- installer and runtime verification matter more because the value is in the full self-hosted path

### Rough Comparison

| Tool shape | Main model | VeilKey difference |
|---|---|---|
| hosted secret SaaS | central hosted control plane | VeilKey keeps runtime and state under your infrastructure |
| generic password manager | store/retrieve secrets | VeilKey focuses on node registration, runtime identity, and policy-driven execution |
| file-encryption workflow | encrypt files in repos | VeilKey adds VaultCenter + LocalVault runtime topology and heartbeat/rebind flows |

### Current Gaps

Compared with more productized operator stacks such as OpenClaw, the current weak points are still visible.

- there is no single unified gateway surface in front of all operator and agent traffic
- session-level context compaction is not a first-class runtime feature yet
- health exposure is clearer than before, but not fully standardized across every service
- the proxy role exists, but its boundary is still less immediately legible than the VaultCenter and LocalVault split

That means the runtime model is strong, but the product shell is still catching up.

### Near-Term Priorities

1. add one shared gateway layer in front of operator and agent-facing entrypoints
2. standardize `/healthz` and deployment healthcheck contracts across services
3. promote context compaction and session reset policy to a first-class runtime feature
4. make the proxy boundary and operator path more explicit in docs and runbooks

## Contributing

Start with [`CONTRIBUTING.md`](./CONTRIBUTING.md).

Short version:

- behavior changes need focused regression tests
- user-facing behavior changes need docs updates in the same change
- installer, runtime, and deploy changes should prove one real operator path

## License

This repository is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

See [`LICENSE`](./LICENSE).

## Security Disclaimer

VeilKey is a security-sensitive tool that handles secrets and cryptographic material.
This software is provided WITHOUT WARRANTY. Before using VeilKey in production,
conduct your own security audit and review. The contributors do not guarantee
the absence of vulnerabilities. Use at your own risk.

If you discover a security issue, please report it privately via GitHub Security Advisories
instead of opening a public issue.
