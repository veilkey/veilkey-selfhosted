# VeilKey Installer

Cross-platform installer CLI for VeilKey Self-Hosted.

## Usage

```bash
# Detect platform
veilkey-installer detect

# macOS (Docker + CLI)
veilkey-installer macos

# Proxmox LXC (Debian)
veilkey-installer proxmox-lxc-debian --ip <IP>/<MASK> --gateway <GATEWAY>

# veil-cli on any Linux
veilkey-installer veil-cli --url <VEILKEY_URL>
```

## Build

```bash
cargo build --release -p veilkey-installer
```

## Architecture

The installer delegates to platform-specific bash scripts in `install/`. It provides:

- Platform detection
- CLI argument parsing with defaults
- Prerequisite checking
- Unified entry point across platforms

Bash scripts remain the source of truth for install logic.
