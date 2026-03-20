#!/usr/bin/env bats

# Smoke tests for install/proxmox-lxc-debian/install-localvault.sh
# Requires: VEILKEY_CENTER_URL, VEILKEY_PASSWORD
#
# Usage:
#   VEILKEY_CENTER_URL=https://10.50.0.110:11181 \
#   VEILKEY_PASSWORD=xxx \
#     bats tests/smoke/proxmox-lxc-debian-localvault.bats

setup_file() {
    export REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)"
    export VEILKEY_CENTER_URL="${VEILKEY_CENTER_URL:?VEILKEY_CENTER_URL is required}"
    export VEILKEY_PASSWORD="${VEILKEY_PASSWORD:?VEILKEY_PASSWORD is required}"
    export VEILKEY_PORT="${VEILKEY_PORT:-10180}"
}

# --- Pre-check ---

@test "pre: VaultCenter is reachable and ok" {
    run curl -sk "$VEILKEY_CENTER_URL/health"
    [ "$status" -eq 0 ]
    [[ "$output" == *'"status":"ok"'* ]]
}

# --- Install ---

@test "install: script succeeds" {
    cd "$REPO_ROOT"
    run bash install/proxmox-lxc-debian/install-localvault.sh
    [ "$status" -eq 0 ]
    [[ "$output" == *"Installation complete"* ]]
}

@test "install: binary exists" {
    [ -f "$REPO_ROOT/.localvault/veilkey-localvault" ]
}

@test "install: process is running" {
    PID=$(cat "$REPO_ROOT/.localvault/localvault.pid" 2>/dev/null)
    [ -n "$PID" ]
    kill -0 "$PID"
}

@test "install: health check returns ok" {
    run curl -s "http://127.0.0.1:$VEILKEY_PORT/health"
    [ "$status" -eq 0 ]
    [[ "$output" == *'"status":"ok"'* ]]
}

# --- VaultCenter Registration ---

@test "register: vault appears in VaultCenter agents" {
    sleep 5  # wait for heartbeat
    run curl -sk "$VEILKEY_CENTER_URL/api/agents"
    [ "$status" -eq 0 ]
    [[ "$output" == *"$(hostname)"* ]]
}

# --- Uninstall ---

@test "uninstall: script succeeds" {
    cd "$REPO_ROOT"
    echo "y" | bash install/proxmox-lxc-debian/uninstall-localvault.sh
    [ $? -eq 0 ]
}

@test "uninstall: process stopped" {
    ! lsof -i ":$VEILKEY_PORT" -sTCP:LISTEN >/dev/null 2>&1
}

@test "uninstall: data directory removed" {
    [ ! -d "$REPO_ROOT/.localvault" ]
}
