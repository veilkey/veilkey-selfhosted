# VeilKey Self-Hosted

`veilkey-selfhosted` is the self-hosted VeilKey product tree for secret lifecycle control, node-local runtime enforcement, and operator-managed infrastructure.

It packages the runtime services, installer, and operator CLI needed to run VeilKey on your own infrastructure instead of relying on a hosted control plane.

Korean summary: [`README.ko.md`](./README.ko.md)

## What VeilKey Is

VeilKey is a self-hosted secret and execution-boundary system for local AI and operator workflows.

The active runtime model is:

- `services/keycenter`
  - central control plane
- `services/localvault`
  - node-local runtime
- `client/cli`
  - operator entrypoint
- `services/proxy`
  - outbound enforcement layer
- `installer`
  - installation and verification layer

## Core Logic

The shortest mental model is:

1. `KeyCenter` owns central policy and catalog decisions.
2. multiple `LocalVault` nodes run close to workloads, often inside separate hosts or containers.
3. operators use the CLI and install flows to register, inspect, and update those nodes.
4. runtime changes are pushed outward by policy, heartbeat, tracked-ref sync, and bulk-apply flows.

In practice, the shape looks like this:

```text
operator / cli
      |
      v
  KeyCenter
      |
      +---- LocalVault (container A)
      +---- LocalVault (container B)
      +---- LocalVault (host node)
```

The important split is:

- `KeyCenter`
  - central control plane
  - global catalog and policy decisions
  - central view of nodes, bindings, audit, and bulk operations
- `LocalVault`
  - node-local runtime
  - ciphertext and config storage
  - heartbeat and runtime identity
  - local execution under KeyCenter policy

## Central Management Model

VeilKey is designed so that keys and runtime state can be managed centrally while execution still happens at the node edge.

That includes:

- central registration of LocalVault nodes into KeyCenter
- central visibility into vault identity and runtime binding
- bulk-apply and workflow-style changes pushed from KeyCenter toward multiple LocalVault nodes
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
  - current KeyCenter runtime binding hash
- `managed_paths`
  - reported runtime ownership context, not the identity itself

Operationally, the flow is:

1. a LocalVault node registers and heartbeats to KeyCenter
2. KeyCenter can require rotation or rebind
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
- how Proxmox hosts and LXCs are provisioned
- how secrets are handled inside your own trust boundary

If you need a hosted SaaS secret manager, this repository is not that.
If you need VeilKey to live on your own host, LXC, and network boundary, this repository is the right surface.

## Quick Start

The fastest operator path is the installer.

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted/installer
./install.sh validate
```

Then choose one of the validated install paths:

- all-in-one LXC

```bash
./scripts/proxmox-lxc-allinone-install.sh --activate /
./scripts/proxmox-lxc-allinone-health.sh /
```

- host-side LocalVault

```bash
./scripts/proxmox-host-localvault/install.sh --activate /
./scripts/proxmox-host-localvault/health.sh /
```

The minimum success check should look like this:

```bash
curl http://127.0.0.1:10181/health
curl http://127.0.0.1:10180/health
```

Expected result:

- KeyCenter health responds
- LocalVault health responds
- the node can heartbeat and appear in the central view after registration

### What Success Looks Like

KeyCenter starts in a locked state and becomes usable after unlock:

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
  KeyCenter API
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

- `KeyCenter`
  - central policy, registration, audit, bulk operations
- `LocalVault`
  - node-local runtime, ciphertext/context, execution boundary
- `proxy`
  - outbound enforcement surface when runtime traffic must be mediated

Additional operating notes live in [`docs/OPERATING-MODEL.md`](./docs/OPERATING-MODEL.md).

## Main Use Cases

- run KeyCenter and LocalVault inside your own Proxmox environment
- keep node-local runtime state under your own control
- use LocalVault as the node-local runtime paired with a central KeyCenter
- stage boundary and bootstrap assets for host companion setups

## How To Read This Repository

- `installer/`
  - install profiles, wrappers, health checks, and packaging
- `services/keycenter/`
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
- explicit Proxmox and LXC install paths
- local runtime components such as LocalVault instead of a cloud-only model
- tighter install-to-runtime contract inside one source tree
- central KeyCenter + multiple LocalVault runtime topology instead of a single hosted vault model

### What It Is Not

- not a hosted SaaS secret manager
- not a generic password vault for personal use
- not a single-binary local secret toy
- not a cloud-only control plane detached from node runtime

### Tradeoffs

- higher operational complexity than a hosted secret service
- stronger dependency on your own host, LXC, and network setup
- installer and runtime verification matter more because the value is in the full self-hosted path

### Rough Comparison

| Tool shape | Main model | VeilKey difference |
|---|---|---|
| hosted secret SaaS | central hosted control plane | VeilKey keeps runtime and state under your infrastructure |
| generic password manager | store/retrieve secrets | VeilKey focuses on node registration, runtime identity, and policy-driven execution |
| file-encryption workflow | encrypt files in repos | VeilKey adds KeyCenter + LocalVault runtime topology and heartbeat/rebind flows |

### Current Gaps

Compared with more productized operator stacks such as OpenClaw, the current weak points are still visible.

- there is no single unified gateway surface in front of all operator and agent traffic
- session-level context compaction is not a first-class runtime feature yet
- health exposure is clearer than before, but not fully standardized across every service
- the proxy role exists, but its boundary is still less immediately legible than the KeyCenter and LocalVault split

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

This repository is licensed under the MIT License.

See [`LICENSE`](./LICENSE).
