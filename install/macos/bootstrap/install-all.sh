#!/bin/bash
set -euo pipefail

# VeilKey full installer for macOS (VaultCenter + LocalVault + veil-cli)
#
# Usage:
#   bash install/macos/bootstrap/install-all.sh
#
# To install separately:
#   bash install/macos/vaultcenter/install.sh
#   bash install/macos/localvault/install.sh
#   bash install/macos/veil-cli/install.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

bash "$REPO_ROOT/install/macos/vaultcenter/install.sh"
bash "$REPO_ROOT/install/macos/localvault/install.sh"
bash "$REPO_ROOT/install/macos/veil-cli/install.sh"
