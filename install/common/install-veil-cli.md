# Linux — veil-cli Installation

Install veil CLI on any Linux machine (Proxmox host, LXC, VM, bare metal).

## Prerequisites

- Rust / cargo: `curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`
- build-essential: `apt install build-essential` (or equivalent)
- Network access to VeilKey server

## Install

```bash
cd veilkey-selfhosted
VEILKEY_URL=https://<HOST>:<VC_PORT> bash install/common/install-veil-cli.sh
```

## After install

```bash
# Load env (or add to ~/.bashrc)
source ~/.veilkey/env

# Check connection
veilkey-cli status

# Enter protected shell
veil
```

## Uninstall

```bash
bash install/common/uninstall-veil-cli.sh
```
