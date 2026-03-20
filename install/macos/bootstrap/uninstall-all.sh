#!/bin/bash
set -euo pipefail

# VeilKey full uninstaller for macOS (veil-cli + LocalVault + VaultCenter)
#
# Usage:
#   bash install/macos/bootstrap/uninstall-all.sh
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

bash "$REPO_ROOT/install/macos/veil-cli/uninstall.sh"
bash "$REPO_ROOT/install/macos/localvault/uninstall.sh"
bash "$REPO_ROOT/install/macos/vaultcenter/uninstall.sh"
