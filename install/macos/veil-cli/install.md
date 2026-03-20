# macOS — veil-cli Installation

## Prerequisites

- Node.js / npm: `brew install node`
- Rust / cargo: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`

## Install

```bash
bash install/macos/veil-cli/install.sh
```

Builds veil CLI from Rust source, installs via npm, and codesigns for macOS Gatekeeper.

Installed binaries: `veil`, `veilkey`, `veilkey-cli`, `veilkey-session-config`

## After install

```bash
cd veilkey-selfhosted && veil
```

## Uninstall

```bash
bash install/macos/veil-cli/uninstall.sh
```
