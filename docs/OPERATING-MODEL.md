# VeilKey Operating Model

This document explains the shortest operator-facing model of VeilKey.

Canonical home: `https://veil-key.com`

## Core Split

- `VaultCenter`
  - central control plane
  - registration, policy, audit, bulk changes
- `LocalVault`
  - node-local runtime
  - ciphertext/context storage
  - heartbeat, rebind, rotation
  - execution boundary close to the workload
- `proxy`
  - outbound enforcement layer when runtime traffic should be mediated

For operator entrypoints, the current split is:

- `veil`
  - protected session entrypoint
- `veilkey`
  - state, policy, and crypto control surface
- `veilkey-cli`
  - lower-level implementation binary used by wrappers and runtime scripts

## End-To-End Shape

```text
operator / CLI
      |
      v
  VaultCenter
      |
      +---- LocalVault (container A)
      +---- LocalVault (container B)
      +---- LocalVault (host node)
```

The practical meaning is:

1. operators act on the central control plane
2. VaultCenter records policy and desired runtime state
3. LocalVault nodes heartbeat and reconcile toward that state
4. wrapped execution stays close to the node instead of moving runtime secrets into a hosted SaaS edge

## Typical Operator Flow

1. install or activate a LocalVault node
2. verify `VaultCenter` and `LocalVault` health
3. unlock or initialize VaultCenter
4. register and inspect node identity
5. apply rotation or rebind from the center
6. verify heartbeat and updated runtime state

For CLI-driven local operator work, the intended flow is:

1. enter `veil`
2. run `claude`, `codex`, or other wrapped tools inside that session
3. use `veilkey-cli status`, `veilkey-cli paste-mode`, or `veilkey-cli resolve` for session and crypto control

## What Success Looks Like

- VaultCenter health responds
- LocalVault health responds
- the node reports heartbeat
- runtime state reflects the current `key_version`
- rebind and rotation are explicit, not silent drift

## Why This Matters

The point of this split is not just secret storage.

It gives you:

- central visibility
- node-local runtime enforcement
- explicit execution boundaries
- controlled rollout and bulk updates across multiple nodes
