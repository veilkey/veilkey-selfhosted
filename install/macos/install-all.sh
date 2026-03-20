#!/bin/bash
set -euo pipefail

# VeilKey full installer for macOS (server + CLI)
#
# Usage:
#   bash install/macos/install-all.sh
#
# To install separately:
#   bash install/macos/install-server.sh   # Docker Compose only
#   bash install/macos/install-cli.sh      # veil CLI only
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

bash "$SCRIPT_DIR/install-server.sh"
bash "$SCRIPT_DIR/install-cli.sh"
