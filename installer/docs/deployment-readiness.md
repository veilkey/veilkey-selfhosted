# Installer Deployment Readiness

This note summarizes the guardrails added around the installer deployment path.

## What Changed

- `shared/**/*` changes now trigger every child pipeline from the monorepo root CI.
- operator-facing install examples now prefer password files over exported password env vars.
- placeholder artifact URLs such as `https://your-gitlab-host/...` are now treated as an explicit operator setup requirement:
  - `./install.sh doctor` warns when the active manifest still contains placeholder URLs and `VEILKEY_INSTALLER_GITLAB_API_BASE` is unset
  - `download`, `bundle`, and `install-profile` fail fast when placeholder URLs would otherwise be used directly
  - unresolved GitHub release placeholders such as `RELEASE_OR_TAG` also fail fast

## Operator Expectation

Before bundle or install commands, do one of the following:

```bash
export VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4"
```

Or rewrite `components.toml` so `artifact_url` values already point at the real package host.

For the `cli` package, the current intended published path is a GitHub release asset:

```text
https://github.com/veilkey/veilkey-selfhosted/releases/download/<tag>/veilkey-cli_<tag>_linux_amd64.tar.gz
```

The example manifest leaves this as `RELEASE_OR_TAG` until a real tagged release exists.

Password handling for operator installs should use password files:

```bash
echo -n 'replace-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
echo -n 'replace-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password
```

The installer still accepts password env vars for CI or other non-interactive wrapper usage, but that is not the preferred operator path.

## Recommended Preflight

Run this before a fresh bundle/install flow:

```bash
cd installer
./install.sh init
./install.sh validate
./install.sh doctor
```

Expected behavior:

- `validate` succeeds
- `doctor` prints a warning if placeholder artifact URLs are still active without `VEILKEY_INSTALLER_GITLAB_API_BASE`

## Regression Coverage

The installer regression path now covers:

- placeholder manifest warning path
- placeholder manifest fail-fast path
- unresolved release-tag placeholder fail-fast path
- install-profile success path with API base override
- proxy component staging path
- proxmox wrapper install paths
