# Hardcoding Rules

This directory holds the operator-facing policy for literals that must not drift into the self-hosted VeilKey repository without review.

The machine-checked source of truth lives in [`../cue/hardcoding_contract.cue`](../cue/hardcoding_contract.cue). This document explains how to apply that contract when reviewing docs, installers, and runtime-facing config.

## Governance Modes

- `blocking`
  - the pipeline must fail when the pattern is matched
- `audit`
  - the pipeline may continue, but the match must still be surfaced and reviewed

## Always Blocking

- plaintext secret material
- embedded credentials in connection strings
- literal `vault_node_uuid`
- literal `vault_hash`
- literal `vault_runtime_hash`

These values must come from runtime injection, provisioning, or secret-manager references. They are never acceptable as committed literals.

## Audit First

- hardcoded IP addresses
- hardcoded deployment ports

These often exist in examples, installer flows, or environment-specific wrappers. The goal is to inventory them first, then reduce and eventually promote the truly unsafe cases into blocking rules.

## Allowlist Principle

Allowlist entries must stay narrow and justified. They are only for:

- loopback-only local development values
- explicit placeholder references such as `VK:REF:*`

Do not use the allowlist to hide production debt.

## Review Workflow

1. update the contract in [`../cue/hardcoding_contract.cue`](../cue/hardcoding_contract.cue)
2. regenerate docs with `bash docs/tools/generate.sh`
3. run `bash docs/tools/doctor.sh`
4. explain any remaining temporary exceptions in the MR
