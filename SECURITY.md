# Security Policy

VeilKey is a self-hosted secret manager that keeps secrets away from AI coding tools. We take reports about its security seriously.

## Supported Versions

Only the **latest 0.5.x release** receives security fixes. Older releases, and commits on `main` between releases, are not supported; fixes ship in the next release.

| Version | Supported |
| --- | --- |
| Latest 0.5.x release | Yes |
| Older releases | No |
| Unreleased `main` commits | No |

[veilkey/veilkey-v2](https://github.com/veilkey/veilkey-v2) is a previous copy of this repository and is not supported.

## Reporting a Vulnerability

Please do **not** open a public issue, discussion or pull request for a security problem.

Report it privately in one of these ways:

1. **GitHub private vulnerability reporting** (preferred): open the [Security tab](https://github.com/veilkey/veilkey-selfhosted/security) of this repository and choose **Report a vulnerability**.
2. **Email**: team@dalsoop.com

Please include:

- the affected version or commit,
- the component (VaultCenter, LocalVault, veil CLI, installer, install scripts),
- the steps to reproduce,
- the impact you observed.

## Response Process

- We acknowledge every report within **7 days**.
- We aim to fix and publicly disclose the issue within **90 days** of the report, and we coordinate the disclosure date with you.
- Fixes are published as a new release together with a GitHub security advisory, and we request a CVE for each advisory.
- With your permission we credit you in the advisory. If you prefer not to be named, we will not name you.

## Scope

The [security model](docs/security-model.md) defines what VeilKey protects against. Threats listed there under **"What VeilKey does NOT protect against"** are out of scope, for example:

- VaultCenter and LocalVault both compromised and the KEK extracted,
- a memory dump of a running VaultCenter,
- an operator who holds the master password,
- physical access to the machines.

Bypasses that the security model documents as design limits are also out of scope, but we welcome reports that show a practical way to close them.

## Maintainer

- [dalsoop](https://github.com/dalsoop)
