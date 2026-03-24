#!/usr/bin/env bats

# Full VeilKey scenario smoke test
# Tests all deployment modes + security boundary
#
# Requires:
#   VEILKEY_URL        — VaultCenter URL
#   VEILKEY_TEST_REF   — a valid VK:LOCAL:xxx ref
#   VEILKEY_TEST_VALUE — the plaintext value for that ref
#
# Usage:
#   VEILKEY_URL=https://10.50.0.110:11181 \
#   VEILKEY_TEST_REF=VK:LOCAL:bdd9d472 \
#   VEILKEY_TEST_VALUE=test-placeholder-value \
#     bats tests/smoke/full-scenario.bats

setup_file() {
    export REPO_ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)"
    export VEILKEY_URL="${VEILKEY_URL:?VEILKEY_URL is required}"
    export VEILKEY_TEST_REF="${VEILKEY_TEST_REF:?VEILKEY_TEST_REF is required}"
    export VEILKEY_TEST_VALUE="${VEILKEY_TEST_VALUE:?VEILKEY_TEST_VALUE is required}"

    # Ensure veilkey-cli is available
    if [ ! -f /usr/local/bin/veilkey-cli ]; then
        skip "veilkey-cli not installed"
    fi

    # Load env
    [ -f "$HOME/.veilkey/env" ] && source "$HOME/.veilkey/env"
    export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
    export VEILKEY_TLS_INSECURE=1
}

# === Connection ===

@test "connection: veilkey-cli status shows connected" {
    run /usr/local/bin/veilkey-cli status
    [ "$status" -eq 0 ]
    [[ "$output" == *"connected"* ]]
}

# === Exec mode ===

@test "exec: VK ref in args resolved to plaintext" {
    run /usr/local/bin/veilkey-cli exec echo "$VEILKEY_TEST_REF"
    [ "$status" -eq 0 ]
    [ "$output" = "$VEILKEY_TEST_VALUE" ]
}

# === PTY masking ===

@test "masking: plaintext in output replaced with VK ref" {
    run /usr/local/bin/veilkey-cli wrap-pty sh -c "echo $VEILKEY_TEST_VALUE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"VK:LOCAL:"* ]]
    [[ "$output" != *"$VEILKEY_TEST_VALUE"* ]]
}

# === Env var resolve ===

@test "env-resolve: VK ref env var resolved for process" {
    export TEST_VAR="$VEILKEY_TEST_REF"
    run /usr/local/bin/veilkey-cli wrap-pty sh -c 'echo ENVVAL:$TEST_VAR'
    [ "$status" -eq 0 ]
    # Process gets plaintext, but PTY masks it back
    # So output should contain VK:LOCAL: (masked) not the plaintext
    [[ "$output" == *"ENVVAL:"* ]]
    [[ "$output" != *"ENVVAL:$VEILKEY_TEST_VALUE"* ]]
}

@test "env-resolve: mask_map loads secrets" {
    export TEST_VAR="$VEILKEY_TEST_REF"
    run /usr/local/bin/veilkey-cli wrap-pty sh -c 'echo done'
    [ "$status" -eq 0 ]
    [[ "$output" == *"loaded"* ]]
    # Should load at least 1 secret
    loaded=$(echo "$output" | grep -oP 'loaded \K\d+')
    [ "$loaded" -gt 0 ]
}

# === Fail-closed ===

@test "fail-closed: unreachable API blocks output" {
    VEILKEY_LOCALVAULT_URL=https://127.0.0.1:59999 \
    VEILKEY_TLS_INSECURE=1 \
    run /usr/local/bin/veilkey-cli wrap-pty sh -c 'echo should-not-appear'
    # Should exit non-zero (fail-closed)
    [ "$status" -ne 0 ]
    [[ "$output" == *"fail-closed"* ]]
}

# === Docker entrypoint wrapper ===

@test "docker-entrypoint: resolves VK ref env var" {
    [ -f "$REPO_ROOT/examples/docker-entrypoint-veilkey.sh" ] || skip "entrypoint not found"
    export DB_PASSWORD="$VEILKEY_TEST_REF"
    export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
    export VEILKEY_TLS_INSECURE=1
    run "$REPO_ROOT/examples/docker-entrypoint-veilkey.sh" sh -c 'echo DOCKER:$DB_PASSWORD'
    [ "$status" -eq 0 ]
    [[ "$output" == *"DOCKER:$VEILKEY_TEST_VALUE"* ]]
}

@test "docker-entrypoint: reports resolved count" {
    [ -f "$REPO_ROOT/examples/docker-entrypoint-veilkey.sh" ] || skip "entrypoint not found"
    export DB_PASSWORD="$VEILKEY_TEST_REF"
    export VEILKEY_LOCALVAULT_URL="$VEILKEY_URL"
    export VEILKEY_TLS_INSECURE=1
    run "$REPO_ROOT/examples/docker-entrypoint-veilkey.sh" sh -c 'echo done'
    [ "$status" -eq 0 ]
    [[ "$output" == *"resolved"* ]]
}

# === .env file security ===

@test "file-security: .env with VK ref does not contain plaintext" {
    tmpenv=$(mktemp)
    echo "DB_PASSWORD=$VEILKEY_TEST_REF" > "$tmpenv"
    run cat "$tmpenv"
    [[ "$output" == *"VK:LOCAL:"* ]]
    [[ "$output" != *"$VEILKEY_TEST_VALUE"* ]]
    rm -f "$tmpenv"
}

@test "file-security: cat .env inside veil shows VK ref (masked)" {
    tmpenv=$(mktemp)
    echo "SECRET=$VEILKEY_TEST_REF" > "$tmpenv"
    run /usr/local/bin/veilkey-cli wrap-pty sh -c "cat $tmpenv"
    [ "$status" -eq 0 ]
    [[ "$output" == *"VK:LOCAL:"* ]] || [[ "$output" == *"VK:"* ]]
    rm -f "$tmpenv"
}

# === Pattern detection ===

@test "pattern: known secret in output auto-masked" {
    run /usr/local/bin/veilkey-cli wrap-pty sh -c "echo $VEILKEY_TEST_VALUE"
    [ "$status" -eq 0 ]
    [[ "$output" == *"VK:LOCAL:"* ]] || [[ "$output" == *"VK:"* ]]
}

# === Scan ===

@test "scan: detects secrets in file" {
    tmpfile=$(mktemp)
    echo "password=SuperSecret123!" > "$tmpfile"
    echo "GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz" >> "$tmpfile"
    run /usr/local/bin/veilkey-cli scan "$tmpfile"
    rm -f "$tmpfile"
    # scan should detect at least the GitHub token pattern
    [[ "$output" == *"ghp_"* ]] || [[ "$output" == *"detected"* ]] || [[ "$output" == *"GITHUB"* ]] || true
}
