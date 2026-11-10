#!/bin/bash
set -euo pipefail

# Retained as a compatibility entry point; password files are unsupported.
echo "Password-file setup is no longer supported, including RAM-disk files." >&2
echo "Unlock VaultCenter through the web UI or TUI after a restart." >&2
echo "LocalVault uses VaultCenter-managed unlock; see docs/setup/vaultcenter/setup.md." >&2
echo "Remove any legacy password files and password-file settings from launchd manually." >&2
exit 1
