# VeilKey Self-Hosted

VeilKey is an open-source secret lifecycle manager for self-hosted infrastructure.

It encrypts secrets at the point of creation, replaces plaintext with scoped references (`VK:{SCOPE}:{REF}`), and enforces policy across distributed nodes — so secrets never sit in config files, env vars, or CI logs in the clear.

## How It Works

```
                    ┌──────────────────────────────┐
                    │       Operator / CLI          │
                    │   scan · filter · wrap · exec │
                    └──────────────┬───────────────┘
                                   │
                    ┌──────────────▼───────────────┐
                    │         KeyCenter             │
                    │   control plane · policy      │
                    │   identity · key lifecycle    │
                    │          :10181               │
                    └──┬───────────┬───────────┬───┘
                       │           │           │
               ┌───────▼──┐ ┌─────▼────┐ ┌────▼─────┐
               │LocalVault│ │LocalVault│ │LocalVault│
               │  node A  │ │  node B  │ │  host    │
               │  :10180  │ │  :10180  │ │  :10180  │
               └──────────┘ └──────────┘ └──────────┘
                       │           │           │
                    ┌──▼───────────▼───────────▼──┐
                    │        Proxy (optional)       │
                    │   egress enforcement · audit  │
                    └──────────────────────────────┘
```

**KeyCenter** is the control plane — it manages node identity, encryption keys, rotation policy, and the canonical secret registry.

**LocalVault** is a node-local agent — it stores encrypted ciphertext, reports heartbeats, and executes lifecycle actions under KeyCenter policy. It never handles plaintext directly.

**CLI** is the operator interface — scan files for leaked secrets, replace them with `VK:` references, wrap commands with automatic secret masking.

**Proxy** is the optional outbound enforcement layer — intercepts network egress from wrapped workloads and blocks or rewrites leaked secrets.

## Quick Start

### macOS

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted
./installer/scripts/install-mac.sh install
./installer/scripts/install-mac.sh start
```

### Proxmox LXC (All-in-One)

```bash
git clone https://github.com/veilkey/veilkey-selfhosted.git
cd veilkey-selfhosted/installer

export VEILKEY_INSTALLER_GITLAB_API_BASE="https://gitlab.60.internal.kr/api/v4"
./install.sh init

echo -n 'your-keycenter-password' > /etc/veilkey/keycenter.password
chmod 600 /etc/veilkey/keycenter.password
echo -n 'your-localvault-password' > /etc/veilkey/localvault.password
chmod 600 /etc/veilkey/localvault.password

./scripts/proxmox-lxc-allinone-install.sh --activate /
./scripts/proxmox-lxc-allinone-health.sh /
```

See [docs/installation.md](docs/installation.md) for all install targets.

## Documentation

| Document | Description |
|----------|-------------|
| [docs/architecture.md](docs/architecture.md) | System architecture and component responsibilities |
| [docs/installation.md](docs/installation.md) | Installation guide for all platforms |
| [docs/cli.md](docs/cli.md) | CLI usage and commands |
| [docs/security-model.md](docs/security-model.md) | Threat model and security design |
| [docs/contributing.md](docs/contributing.md) | Development workflow and PR process |

## Repository Layout

```
veilkey-selfhosted/
├── services/
│   ├── keycenter/      # control plane (Go)
│   ├── localvault/     # node agent (Go)
│   └── proxy/          # egress enforcement (Go + eBPF)
├── client/
│   └── cli/            # operator CLI (Go)
├── installer/          # packaging, profiles, health checks
├── shared/             # deploy scripts, shell hooks, tests
└── docs/               # architecture, guides, security model
```

## License

[AGPL-3.0](LICENSE)
