# Hardcoding Baseline Audit

**Date:** 2026-03-15
**Scope:** Full recursive scan of `/opt/veilkey-selfhosted-repo`

---

## What Was Checked

1. **Private IP addresses** -- patterns `10.*`, `192.168.*`, `172.16-31.*`
2. **Localhost URLs** -- `http://127.0.0.1`, `http://localhost`
3. **Hardcoded credentials** -- `password=`, `token=`, `secret=`, `key=` with literal string values
4. **Hardcoded external domains** -- non-example, non-license URLs (notably `gitlab.ranode.net`)

Files explicitly excluded from findings: `.env.example` placeholder lines, LICENSE files, Go module paths (`go.mod`, import statements), and `validation-logs/` (read-only historical logs).

---

## Findings

### Category 1: Hardcoded Internal Infrastructure IPs

#### NEEDS ATTENTION -- Production source code with lab-specific IPs

| File | Value | Context |
|------|-------|---------|
| `services/proxy/policy/proxy-profiles.toml` | `10.50.2.8:18080-18084` | Proxy URL fields for all four profiles (default, codex, claude, opencode). This is a checked-in config file used at runtime. |
| `services/proxy/deploy/host/doctor-veilkey.sh` | `10.50.2.6:10180`, `10.50.2.7:10180` | Default fallback for keycenter and hostvault health URLs (overridable via env vars). |
| `services/proxy/cmd/veilkey-session-config/main.go` | `10.50.2.6:10180` | Compiled-in fallback for `veilkeyHubURL()`. |
| `client/cli/cmd/veilkey-session-config/main.go` | `10.50.2.6:10180` | Same compiled-in fallback in the CLI variant. |
| `services/proxy/deploy/host/session-tools.toml.example` | `10.50.2.6:10180`, `10.50.2.7:10181` | Example config, but contains lab-specific IPs instead of generic placeholders. |
| `client/cli/deploy/host/session-tools.toml.example` | `10.50.2.6:10180` | Same issue in CLI example. |
| `installer/install.sh` (lines 232-234, 322-324) | `10.60.100.210`, `10.50.100.210` | GitLab mirror IP candidates for credential/package resolution. |
| `services/localvault/.gitlab-ci.yml` (lines 42, 45) | `10.50.100.210` | CI pipeline package publish fallback URL and credential host. |
| `services/proxy/deploy/lxc/create-proxy-lxc.sh` (lines 52-53) | `10.50.0.1` | Default gateway and nameserver for LXC creation. |

#### Acceptable -- RFC 1918 ranges used as policy defaults

| File | Value | Context |
|------|-------|---------|
| `installer/scripts/proxmox-host-localvault/install.sh` | `10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,127.0.0.1` | Trusted-IP CIDR defaults; standard private ranges, not site-specific. |
| `services/keycenter/install/server-macos.sh` | Same CIDR set | Same purpose. |
| `services/keycenter/install/server-linux.sh` | Same CIDR set | Same purpose. |
| `*/veilkey-veilroot-egress-guard`, `*/veilkey-root-ai-egress-guard` | `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` | iptables allow-rules for private ranges. Generic, not site-specific. |

#### Acceptable -- Test fixtures and validation logs

All IPs in `*_test.go`, `tests/test_*.sh`, `installer/scripts/ci/`, and `installer/validation-logs/` are test data. These are not deployed and do not leak infrastructure details beyond what is already in the repo.

---

### Category 2: Localhost URLs (`http://127.0.0.1`)

All localhost references fall into two patterns:

1. **Default fallback in source code** -- e.g., `http://127.0.0.1:10180` as the last-resort LocalVault URL when no env var or config is set (`keycenter/internal/api/hkm_global_function_run.go`, `client/cli/cmd/veilkey-session-config/main.go`, installer scripts). These are acceptable; localhost is the correct default for a local service.
2. **Test fixtures, examples, install scripts** -- health-check probes, example configs, CLI usage output. Acceptable.

No findings that need attention in this category.

---

### Category 3: Hardcoded Credentials / Secrets

No real secrets were found. All matches are:

| Pattern | Files | Verdict |
|---------|-------|---------|
| `VEILKEY_KEYCENTER_PASSWORD='e2e-keycenter'` / `'test-keycenter'` / `'test-localvault'` | `installer/scripts/ci/`, `installer/tests/` | CI/test passwords. Acceptable. |
| `password = "correct-horse-battery-staple"` / `"the-right-password"` | `services/keycenter/tests/integration/` | Test-only constants. Acceptable. |
| `Token: "legacy-token"` | `services/keycenter/internal/api/install_custody_test.go` | Test fixture. Acceptable. |
| `GITLAB_TOKEN = "VK:EXTERNAL:abcd1234"` / `"VK:LOCAL:f33f06f1"` | `client/cli/README.md`, `client/cli/functions/gitlab/current-user.toml` | VK token references (opaque refs, not secrets). Acceptable. |
| `VEILKEY_LOCALVAULT_PASSWORD='replace-me'` etc. | `installer/INSTALL.md` | Documentation placeholders. Acceptable. |

No plaintext production secrets detected.

---

### Category 4: Hardcoded External Domains

| File | Value | Context |
|------|-------|---------|
| `installer/install.sh`, `installer/components.toml.example`, CI scripts | `gitlab.ranode.net` | The organization's self-hosted GitLab instance. Used for artifact URLs, credential probing, and Go module paths. This is expected for an internal-deployment repo. |
| `client/cli/functions/gitlab/current-user.toml` | `gitlab.ranode.net/api/v4/user` | Example function definition referencing the internal GitLab. |
| `client/cli/.goreleaser.yaml` | `gitlab.com` (fallback) | GoReleaser default when CI env vars are absent. Generic, acceptable. |
| Various | `go.dev`, `brew.sh`, `fsf.org`, `github.com`, `example.com`, `example.test` | Standard external references. Acceptable. |

The `gitlab.ranode.net` references are inherent to the project's hosting. They are not secrets but would need updating if the GitLab instance were migrated.

---

## Summary Classification

### Blocking (should be parameterized before broader distribution)

None. No plaintext secrets or credentials exist in the codebase.

### Needs Attention (audit-only, should be tracked for cleanup)

| ID | Issue | Files |
|----|-------|-------|
| HC-1 | Lab-specific IP `10.50.2.8` hardcoded in runtime proxy config | `services/proxy/policy/proxy-profiles.toml` |
| HC-2 | Lab-specific IPs `10.50.2.6`, `10.50.2.7` as compiled-in Go fallbacks | `services/proxy/cmd/veilkey-session-config/main.go`, `client/cli/cmd/veilkey-session-config/main.go` |
| HC-3 | Lab-specific IPs `10.50.2.6`, `10.50.2.7` as shell defaults | `services/proxy/deploy/host/doctor-veilkey.sh` |
| HC-4 | GitLab mirror IPs `10.60.100.210`, `10.50.100.210` in installer and CI | `installer/install.sh`, `services/localvault/.gitlab-ci.yml` |
| HC-5 | Default gateway `10.50.0.1` in LXC creation script | `services/proxy/deploy/lxc/create-proxy-lxc.sh` |
| HC-6 | Example configs contain lab IPs instead of placeholders | `*/session-tools.toml.example` (2 files) |

### Acceptable (no action needed)

- RFC 1918 CIDR ranges used as generic trusted-IP defaults.
- `127.0.0.1` localhost fallbacks in source and scripts.
- All test-file IPs and passwords.
- Documentation placeholder passwords (`replace-me`).
- `gitlab.ranode.net` domain references (inherent to project).
- VK token reference strings (opaque refs, not secrets).
