# Architecture

This document summarizes the self-hosted runtime shape for `veilkey-selfhosted`.

## Runtime Shape

The active runtime is split across the following component groups:

- `services/keycenter`
  - control plane, inventory, lifecycle, orchestration
- `services/localvault`
  - node-local runtime, ciphertext store, bulk-apply execution
- `client/cli`
  - operator-facing CLI and host session boundary
- `services/proxy`
  - outbound enforcement layer behind wrapped execution
- `installer`
  - install, bundle, activation, and health verification

## Boundary Model

The repository is intentionally kept in one place without flattening runtime responsibilities.

- `keycenter`
  - owns policy, runtime identity tracking, and orchestration
- `localvault`
  - owns node-local execution and ciphertext/context material
- `cli`
  - owns operator-facing invocation and host-boundary UX
- `proxy`
  - owns egress enforcement and outbound audit surfaces
- `installer`
  - owns packaging and deployment verification

## Canonical References

- repository hub: [`../README.md`](../README.md)
- docs hub: [`README.md`](./README.md)
- CUE contracts: [`cue/repository.cue`](./cue/repository.cue)
- generated summary: [`generated/summary.md`](./generated/summary.md)
