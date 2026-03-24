#!/usr/bin/env bats

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$BATS_TEST_FILENAME")/../.." && pwd)}"

@test "no PASSWORD_FILE references in Go server code" {
  count=$(grep -r 'PASSWORD_FILE\|ReadPasswordFromFileEnv\|PasswordFile' \
    "$REPO_ROOT/services/vaultcenter/internal/commands/" \
    "$REPO_ROOT/services/localvault/internal/commands/" \
    --include='*.go' | grep -v '_test.go' | wc -l)
  [ "$count" -eq 0 ]
}

@test "no VEILKEY_PASSWORD auto-unlock in Go server code" {
  count=$(grep -r 'VEILKEY_PASSWORD[^_]' \
    "$REPO_ROOT/services/vaultcenter/internal/commands/" \
    "$REPO_ROOT/services/localvault/internal/commands/" \
    --include='*.go' | grep -v '_test.go' | grep -v '// ' | wc -l)
  [ "$count" -eq 0 ]
}

@test "no PASSWORD_FILE in docker entrypoints" {
  count=$(grep -rl 'PASSWORD_FILE' \
    "$REPO_ROOT/services/vaultcenter/docker-entrypoint.sh" \
    "$REPO_ROOT/services/localvault/docker-entrypoint.sh" 2>/dev/null | wc -l)
  [ "$count" -eq 0 ]
}

@test "no ReadPasswordFromFileEnv calls in server code" {
  count=$(grep -r 'ReadPasswordFromFileEnv' \
    "$REPO_ROOT/services/vaultcenter/internal/commands/" \
    "$REPO_ROOT/services/localvault/internal/commands/" \
    --include='*.go' | grep -v '_test.go' | wc -l)
  [ "$count" -eq 0 ]
}

@test "DB key derived from KEK in VaultCenter api.go" {
  grep -q 'deriveDBKeyFromKEK' "$REPO_ROOT/services/vaultcenter/internal/api/api.go"
}

@test "DB key derived from KEK in LocalVault api.go" {
  grep -q 'deriveDBKeyFromKEK' "$REPO_ROOT/services/localvault/internal/api/api.go"
}

@test "no legacy deriveDBKey(salt) in VaultCenter server.go" {
  ! grep -q 'deriveDBKey(salt)' "$REPO_ROOT/services/vaultcenter/internal/commands/server.go"
}

@test "no legacy deriveDBKey(salt) in LocalVault server.go" {
  ! grep -q 'deriveDBKey(salt)' "$REPO_ROOT/services/localvault/internal/commands/server.go"
}

@test "LocalVault has autoUnlock function" {
  grep -q 'func autoUnlock' "$REPO_ROOT/services/localvault/internal/commands/server.go"
}

@test "VaultCenter has unlock-key endpoint" {
  grep -q 'unlock-key' "$REPO_ROOT/services/vaultcenter/internal/api/hkm/handler.go"
}
