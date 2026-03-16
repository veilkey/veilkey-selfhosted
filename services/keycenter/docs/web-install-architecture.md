# VeilKey Web Install Architecture

This note defines the target architecture for VeilKey web-first installation and centralized runtime management.

It replaces node-local hardcoded install values with KeyCenter-managed install policy and controlled server-side execution.

## Goals

- make KeyCenter the canonical control plane for install policy
- keep browser UI out of direct shell execution
- avoid per-node hardcoded DNS, TLS, and hub URL values
- support repeatable install, re-apply, and re-registration flows
- preserve masking-first and non-plaintext operational boundaries

## Non-Goals

- arbitrary shell access from the browser
- storing plaintext passwords as long-lived UI settings
- direct browser-to-node SSH orchestration
- making every LocalVault independently decide hub and TLS policy

## Design Principles

1. Web UI manages policy, targets, approvals, and progress only.
2. Server-side fixed runners execute install actions.
3. Node runtime config is rendered from centralized policy, not handwritten env drift.
4. Secrets are accepted through short-lived custody/input flows and converted to password files or VeilKey refs.
5. Completion is granted only after health, registration, and final smoke checks pass.

## External Pattern Alignment

VeilKey should follow the same broad pattern used by products such as Rancher and Portainer:

- central UI selects the desired target state
- a server-side runner or pull-agent performs the actual install
- node registration is explicit and auditable
- credentials and certificates are handled centrally and rendered into node-local runtime state only when needed

## Control Plane Model

KeyCenter becomes the canonical source for:

- install profiles
- target groups
- install runners
- KeyCenter and LocalVault public URLs
- TLS certificate, key, and CA paths
- DNS and VIP intent
- install session state
- approval and custody state

The existing `install session` and `install runtime config` APIs are part of this control plane.

## Execution Model

### 1. Install Session

The operator starts a session from the web UI.

The session records:

- flow
- deployment mode
- target scope
- planned stages
- completed stages
- approval mode

This already exists in KeyCenter and remains the gate for operator access.

### 2. Central Runtime Config

KeyCenter stores install runtime config as centrally managed policy, including:

- `install_profile`
- `install_root`
- `install_script`
- `install_workdir`
- `keycenter_url`
- `localvault_url`
- `tls_cert_path`
- `tls_key_path`
- `tls_ca_path`

This config must be treated as the source of truth for install rendering.

### 3. Controlled Install Apply

KeyCenter exposes a controlled install apply endpoint.

Suggested shape:

- `POST /api/install/apply`

Behavior:

- reads central install runtime config from KeyCenter DB
- reads current install session
- verifies required approvals are complete
- invokes a fixed allowlisted install runner
- passes only validated arguments and environment
- records audit events and stage transitions

The browser never submits an arbitrary command.

### 4. Runner Types

Support two execution models.

#### A. Push Runner

KeyCenter runs a server-side script or job runner on a trusted control node.

Use for:

- local host installs
- proxmox-host-localvault
- single-node recovery

#### B. Pull Agent

A lightweight install agent on the target periodically polls KeyCenter for desired state and retrieves a signed install job.

Use for:

- multi-node rollout
- remote sites
- environments where inbound orchestration is undesirable

The long-term preferred model is pull-agent.

## Node Registration Model

Node registration should follow the current LocalVault heartbeat identity model.

Required outcome:

- each installed LocalVault registers with `vault_node_uuid`
- KeyCenter binds it to `vault_hash` and `vault_runtime_hash`
- install apply does not mark success until expected registration appears

This avoids false success when packages install but the node never joins.

## Secrets and Custody

The web UI may request operator input, but plaintext handling must remain short-lived.

Allowed pattern:

- operator provides approval or custody input through the current bootstrap/custody flow
- server-side runner converts approved input into:
  - password file paths
  - one-time install files
  - VeilKey refs for longer-lived policy

Disallowed pattern:

- storing installer passwords as durable UI config fields
- sending plaintext passwords back from server to browser
- copying long-lived secrets into multiple env files

## TLS and DNS Policy

TLS and public endpoint policy should be centralized.

KeyCenter policy should define:

- intended public KeyCenter URL
- intended public LocalVault URL
- certificate material paths or resolver references
- CA distribution path
- optional management VIP or DNS target

Install runners render node-local env from this policy.

Node-local env should be considered generated state, not operator-authored truth.

## Admin UI Scope

The web UI should expose:

- install profile selection
- target group or target node selection
- DNS and URL policy summary
- TLS material references
- approval status
- install apply trigger
- stage progress
- health and registration results

The web UI should not expose:

- raw shell commands
- arbitrary script paths unless already approved in server policy
- plaintext secret review

## Proposed Data Model Additions

The current runtime config fields are enough for the first step, but the target model needs separate entities:

- `install_targets`
  - target id, kind, addressability, environment metadata
- `install_target_groups`
  - group id, profile defaults, rollout policy
- `install_policies`
  - URLs, TLS refs, DNS intent, runner binding
- `install_runs`
  - run id, target id, requested version, status, logs, artifacts

This is preferable to overloading `ui_configs` indefinitely.

## Proposed API Surface

Near-term:

- `GET /api/install/runtime-config`
- `PATCH /api/install/runtime-config`
- `POST /api/install/apply`
- `GET /api/install/runs`
- `GET /api/install/runs/{run_id}`

Later:

- `POST /api/install/targets`
- `GET /api/install/targets`
- `POST /api/install/target-groups`
- `POST /api/install/policies`
- `POST /api/install/agent/register`
- `POST /api/install/agent/heartbeat`

## Completion Gate

Install access should open only when all of the following are true:

- install runner exited successfully
- expected services are healthy
- expected KeyCenter or LocalVault status endpoints return healthy
- expected LocalVault registration appears in inventory
- final smoke stage is recorded

If any of these fail, the install gate remains closed.

## Migration Plan

### Phase 1

- keep installer shell wrappers
- add KeyCenter-owned runtime config
- add controlled install apply API
- record install runs and audit events

### Phase 2

- add admin UI for runtime config and install apply
- render generated env from central policy
- stop treating handwritten `/etc/veilkey/*.env` as canonical

### Phase 3

- introduce pull-agent for remote targets
- add target groups and rollout policy
- make DNS and TLS intent part of install policy objects

## Immediate Implementation Guidance

Implement next in this order:

1. add `POST /api/install/apply` with allowlisted script execution
2. persist install run state and logs
3. update install session stages from runner results
4. add admin UI for runtime config and apply action
5. teach LocalVault and related tooling to prefer KeyCenter-issued policy over stale local env

## Decision

VeilKey should use a web-first centralized install model where:

- KeyCenter owns install policy
- browser UI owns intent and approval
- controlled runners or agents own execution
- generated node config is derived state

This is the simplest path that removes local hardcoding without creating an unsafe browser-driven shell model.
