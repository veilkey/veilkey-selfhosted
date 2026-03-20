#!/usr/bin/env bats

# Smoke tests for install/common/install-veil-cli.sh
# Requires: VEILKEY_URL (VaultCenter URL), VEILKEY_TEST_REF (a valid VK:LOCAL:xxx ref)
#
# Usage:
#   VEILKEY_URL=https://10.50.0.110:11181 \
#   VEILKEY_TEST_REF=VK:LOCAL:07c52335 \
#   VEILKEY_TEST_VALUE=my-super-secret-value \
#     bats tests/smoke/common-veil-cli.bats

setup_file() {
    export REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)"
    export VEILKEY_URL="${VEILKEY_URL:?VEILKEY_URL is required}"
    export VEILKEY_TEST_REF="${VEILKEY_TEST_REF:?VEILKEY_TEST_REF is required}"
    export VEILKEY_TEST_VALUE="${VEILKEY_TEST_VALUE:?VEILKEY_TEST_VALUE is required}"
}

# --- Install ---

@test "install: script fails without VEILKEY_URL" {
    cd "$REPO_ROOT"
    unset VEILKEY_URL
    run bash install/common/install-veil-cli.sh
    [ "$status" -ne 0 ]
    [[ "$output" == *"VEILKEY_URL is required"* ]]
}

@test "install: script succeeds with VEILKEY_URL" {
    cd "$REPO_ROOT"
    run bash install/common/install-veil-cli.sh
    [ "$status" -eq 0 ]
    [[ "$output" == *"Installation complete"* ]]
}

@test "install: binaries exist in /usr/local/bin" {
    [ -f /usr/local/bin/veil ]
    [ -f /usr/local/bin/veilkey ]
    [ -f /usr/local/bin/veilkey-cli ]
    [ -f /usr/local/bin/veilkey-session-config ]
}

@test "install: config exists" {
    [ -f "$HOME/.veilkey/env" ]
    [ -f "$HOME/.veilkey/config/veilkey.yml" ]
}

@test "install: env contains VEILKEY_URL" {
    source "$HOME/.veilkey/env"
    [[ "$VEILKEY_LOCALVAULT_URL" == "$VEILKEY_URL" ]]
}

# --- Connection ---

@test "connect: veilkey-cli status shows connected" {
    source "$HOME/.veilkey/env"
    run /usr/local/bin/veilkey-cli status
    [ "$status" -eq 0 ]
    [[ "$output" == *"connected"* ]]
}

# --- Resolve ---

@test "resolve: returns expected secret value" {
    source "$HOME/.veilkey/env"
    run /usr/local/bin/veilkey-cli resolve "$VEILKEY_TEST_REF"
    [ "$status" -eq 0 ]
    [ "$output" = "$VEILKEY_TEST_VALUE" ]
}

# --- PTY Masking ---

@test "masking: real value is replaced with VK ref in output" {
    source "$HOME/.veilkey/env"
    run /usr/local/bin/veilkey-cli wrap-pty sh -c "echo $VEILKEY_TEST_VALUE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"VK:LOCAL:"* ]]
}

# --- Uninstall ---

@test "uninstall: script succeeds" {
    cd "$REPO_ROOT"
    run bash install/common/uninstall-veil-cli.sh
    [ "$status" -eq 0 ]
}

@test "uninstall: binaries removed" {
    [ ! -f /usr/local/bin/veil ]
    [ ! -f /usr/local/bin/veilkey-cli ]
}

@test "uninstall: config removed" {
    [ ! -d "$HOME/.veilkey" ]
}
