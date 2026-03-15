# Hardcoding Baseline

Baseline date: **2026-03-15**
Product version at baseline: **0.1.0**

This document records the current hardcoding status of each component in the
VeilKey self-hosted repository. It is the companion to
[`rules.md`](rules.md), which defines what is and is not permitted.

---

## Component Status

| Component | Path | Enforcement Mode | Status | Notes |
|---|---|---|---|---|
| installer | `installer/` | audit | clean | `.env.example` profile files contain default ports only |
| keycenter | `services/keycenter/` | audit | clean | `.env.example` uses placeholder password `changeme` and default port `10180` |
| localvault | `services/localvault/` | audit | clean | No hardcoded secrets detected |
| proxy | `services/proxy/` | audit | known exceptions | See proxy exceptions below |
| cli | `client/cli/` | audit | clean | `patterns.yml` contains detection regexes, not secrets; example config uses placeholder values |

---

## Known Exceptions

### proxy: `services/proxy/policy/proxy-profiles.toml`

**Category:** Production IP addresses and port numbers

The proxy profile configuration contains literal IP addresses (e.g.
`10.50.2.8`) and port numbers (`18080`-`18084`) that represent a specific
deployment topology.

**Justification:** This file currently serves as both the example template
and the active profile definition. Until a config-generation or overlay
mechanism is introduced, these values remain as-is.

**Remediation plan:** Move deployment-specific addresses into an
operator-provided overlay or `.env`-driven template so that the committed
file contains only placeholder or default values.

**Target mode:** blocking (after remediation)

### cli: `client/cli/function_test.go`

**Category:** Test fixture values

The test file contains a hardcoded vault hash value (`56093730`) used as an
expected output assertion.

**Justification:** This is a deterministic test fixture, not a real secret
or production value. Test assertions require known expected values.

**Remediation plan:** None required. Test fixtures with synthetic data are
acceptable.

**Target mode:** N/A (exempt as test fixture)

---

## Installer Profile Defaults

The following `.env.example` files under `installer/profiles/` contain
commented-out default port values. These are allowed under the rules:

- `proxmox-lxc-allinone.env.example` -- ports `10180`, `10181`
- `proxmox-host-localvault.env.example` -- port `10180`
- `proxmox-host.env.example`
- `proxmox-lxc-runtime.env.example`
- `proxmox-host-cli.env.example`

---

## Next Review

The baseline should be re-evaluated when:

- A component transitions from `audit` to `blocking` mode
- New components are added to the repository
- The proxy config overlay mechanism is implemented
- The product version reaches `0.2.0`
