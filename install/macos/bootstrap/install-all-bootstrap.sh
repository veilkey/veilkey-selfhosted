#!/bin/bash
set -euo pipefail

# VeilKey bootstrap installer for macOS
# Clones the repo (or updates) then runs install-all.sh
# Usage: curl -sL .../install-all-bootstrap.sh | bash
#
# ⚠️  이 스크립트의 실행으로 발생하는 모든 결과에 대한
#     귀책사유는 실행자 본인에게 있습니다.

REPO_URL="https://github.com/veilkey/veilkey-selfhosted.git"
TARGET_DIR="${VEILKEY_DIR:-veilkey-selfhosted}"

if [ -d "$TARGET_DIR/.git" ]; then
    echo "기존 설치 감지: $TARGET_DIR"
    echo "업데이트합니다..."
    cd "$TARGET_DIR"
    git pull --quiet
else
    echo "VeilKey를 $TARGET_DIR 에 설치합니다..."
    git clone --quiet "$REPO_URL" "$TARGET_DIR"
    cd "$TARGET_DIR"
fi

exec bash install/macos/bootstrap/install-all.sh
