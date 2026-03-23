#!/usr/bin/env bats
# vk-bulk-apply-sync.sh 테스트 스위트 (17 케이스)
#
# 실행: bats tests/bulk-apply-sync/vk-bulk-apply-sync.bats
# 필요: bats >= 1.5, python3, curl

SCRIPT_DIR="$(cd "$(dirname "$BATS_TEST_FILENAME")" && pwd)"
REPO_ROOT="${VK_REPO_ROOT:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SCRIPT="${VK_SCRIPT:-$REPO_ROOT/scripts/vk-bulk-apply-sync.sh}"
MOCK="$SCRIPT_DIR/mock_server.py"
FIXTURES="$SCRIPT_DIR/fixtures"
LV_PORT=18900
VC_PORT=18901

# ── 헬퍼 ──

load_fixture() {
  local fixture="$1"
  curl -s "http://127.0.0.1:$LV_PORT/test/load-fixture" -X POST -d @"$fixture" >/dev/null
  curl -s "http://127.0.0.1:$VC_PORT/test/load-fixture" -X POST -d @"$fixture" >/dev/null
}

reset_mocks() {
  curl -s "http://127.0.0.1:$LV_PORT/test/reset" -X POST >/dev/null
  curl -s "http://127.0.0.1:$VC_PORT/test/reset" -X POST >/dev/null
}

get_bulk_log() {
  curl -s "http://127.0.0.1:$LV_PORT/test/bulk-log"
}

run_sync() {
  VEILKEY_VAULTCENTER_URL="http://127.0.0.1:$VC_PORT" \
  VEILKEY_AGENT_HASH="agent-1" \
  VEILKEY_LOCALVAULT_URL="http://127.0.0.1:$LV_PORT" \
  VEILKEY_TEMPLATE_NAME="test-tpl" \
  VEILKEY_TARGET_PATH="$TEST_ENV" \
  VEILKEY_POST_HOOK="${VK_POST_HOOK:-}" \
  VEILKEY_DRY_RUN="${VK_DRY_RUN:-0}" \
  bash "$SCRIPT"
}

# ── setup/teardown ──

setup_file() {
  python3 "$MOCK" "$LV_PORT" &
  echo $! > /tmp/bats_mock_lv.pid
  python3 "$MOCK" "$VC_PORT" &
  echo $! > /tmp/bats_mock_vc.pid
  for port in $LV_PORT $VC_PORT; do
    for _ in $(seq 1 50); do
      curl -s "http://127.0.0.1:$port/test/ping" >/dev/null 2>&1 && break
      sleep 0.1
    done
  done
}

teardown_file() {
  kill "$(cat /tmp/bats_mock_lv.pid 2>/dev/null)" 2>/dev/null || true
  kill "$(cat /tmp/bats_mock_vc.pid 2>/dev/null)" 2>/dev/null || true
  rm -f /tmp/bats_mock_lv.pid /tmp/bats_mock_vc.pid
}

setup() {
  TEST_ENV="$(mktemp)"
  rm -f "$TEST_ENV"
  reset_mocks
  VK_POST_HOOK=""
  VK_DRY_RUN="0"
}

teardown() {
  rm -f "$TEST_ENV" "${TEST_ENV}.hook" "${TEST_ENV}.bak" "${TEST_ENV}.vk-checksum"
}

# ══════════════════════════════════════════════════════════════
# 기본 동작
# ══════════════════════════════════════════════════════════════

@test "기본: 3개 시크릿 resolve → .env 생성" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  [ -f "$TEST_ENV" ]
  grep -q "API_KEY=sk-primary-api-key-123" "$TEST_ENV"
  grep -q "DB_PASSWORD=super-secret-db-pw" "$TEST_ENV"
  grep -q "ADMIN_TOKEN=admin-tok-456" "$TEST_ENV"
  grep -q "# VeilKey managed" "$TEST_ENV"
}

@test "bulk-apply 페이로드에 template/target/content 포함" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  log=$(get_bulk_log)
  echo "$log" | python3 -c "
import json, sys
entries = json.load(sys.stdin)
assert len(entries) == 1, f'expected 1 call, got {len(entries)}'
s = entries[0]['steps'][0]
assert s['name'] == 'test-tpl'
assert s['format'] == 'env'
assert 'API_KEY=sk-primary-api-key-123' in s['content']
assert 'DB_PASSWORD=super-secret-db-pw' in s['content']
"
}

@test ".env 헤더에 타임스탬프 포함" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  head -1 "$TEST_ENV" | grep -qE "# VeilKey managed — auto-synced [0-9]{4}-"
}

# ══════════════════════════════════════════════════════════════
# cross-vault 키 재사용
# ══════════════════════════════════════════════════════════════

@test "cross-vault: 이 vault에 없으면 다른 vault에서 재사용" {
  load_fixture "$FIXTURES/cross_vault.json"
  run run_sync
  [ "$status" -eq 0 ]
  grep -q "SHARED_KEY=shared-from-secondary" "$TEST_ENV"
  grep -q "LOCAL_ONLY=local-only-value" "$TEST_ENV"
  [[ "$output" == *"reused from agent agent-2"* ]]
}

# ══════════════════════════════════════════════════════════════
# 필터링
# ══════════════════════════════════════════════════════════════

@test "active 시크릿만 sync, block/temp 제외" {
  load_fixture "$FIXTURES/inactive_secrets.json"
  run run_sync
  [ "$status" -eq 0 ]
  grep -q "ACTIVE_KEY=active-value-123" "$TEST_ENV"
  ! grep -q "BLOCKED_KEY" "$TEST_ENV"
  ! grep -q "TEMP_KEY" "$TEST_ENV"
}

# ══════════════════════════════════════════════════════════════
# 빈 vault
# ══════════════════════════════════════════════════════════════

@test "빈 LocalVault → 즉시 종료, .env 미생성" {
  load_fixture "$FIXTURES/empty_vault.json"
  run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"No active secrets"* ]]
  [ ! -f "$TEST_ENV" ]
}

# ══════════════════════════════════════════════════════════════
# resolve 실패
# ══════════════════════════════════════════════════════════════

@test "일부 키 missing → 빈 값으로 배포, MISSING 로그" {
  load_fixture "$FIXTURES/missing_secrets.json"
  run run_sync
  [ "$status" -eq 0 ]
  grep -q "FOUND_KEY=found-value" "$TEST_ENV"
  grep -q "MISSING_KEY=$" "$TEST_ENV"
  grep -q "ALSO_MISSING=$" "$TEST_ENV"
  [[ "$output" == *"MISSING_KEY: MISSING"* ]]
  [[ "$output" == *"ALSO_MISSING: MISSING"* ]]
}

# ══════════════════════════════════════════════════════════════
# 변경 감지
# ══════════════════════════════════════════════════════════════

@test "변경 없으면 bulk-apply 스킵" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"No changes detected"* ]]
}

@test "체크섬 삭제 시 재적용" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  rm -f "${TEST_ENV}.vk-checksum"
  run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"Changes detected"* ]]
  grep -q "API_KEY=sk-primary-api-key-123" "$TEST_ENV"
}

@test "타임스탬프 차이만 있으면 변경 아님 (체크섬 동일)" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  [ "$status" -eq 0 ]
  run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"No changes detected"* ]]
}

# ══════════════════════════════════════════════════════════════
# dry-run
# ══════════════════════════════════════════════════════════════

@test "dry-run: 파일 미생성, 마스킹 출력" {
  load_fixture "$FIXTURES/basic.json"
  VK_DRY_RUN=1 run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"DRY RUN"* ]]
  [[ "$output" == *"=****"* ]]
  [ ! -f "$TEST_ENV" ]
}

# ══════════════════════════════════════════════════════════════
# post-hook
# ══════════════════════════════════════════════════════════════

@test "post-hook: 변경 시 실행" {
  load_fixture "$FIXTURES/basic.json"
  VK_POST_HOOK="touch ${TEST_ENV}.hook" run run_sync
  [ "$status" -eq 0 ]
  [ -f "${TEST_ENV}.hook" ]
}

@test "post-hook: 변경 없으면 미실행" {
  load_fixture "$FIXTURES/basic.json"
  run run_sync
  VK_POST_HOOK="touch ${TEST_ENV}.hook" run run_sync
  [ "$status" -eq 0 ]
  [ ! -f "${TEST_ENV}.hook" ]
}

@test "post-hook 실패해도 스크립트 성공" {
  load_fixture "$FIXTURES/basic.json"
  VK_POST_HOOK="false" run run_sync
  [ "$status" -eq 0 ]
  [[ "$output" == *"WARNING: post-hook failed"* ]]
}

# ══════════════════════════════════════════════════════════════
# 에러 처리
# ══════════════════════════════════════════════════════════════

@test "bulk-apply 실패 → 에러 종료" {
  load_fixture "$FIXTURES/bulk_apply_fail.json"
  run run_sync
  [ "$status" -ne 0 ]
  [[ "$output" == *"bulk-apply failed"* ]]
}

@test "필수 환경변수 VEILKEY_VAULTCENTER_URL 누락 → 에러" {
  run bash -c 'unset VEILKEY_VAULTCENTER_URL; VEILKEY_AGENT_HASH=x VEILKEY_LOCALVAULT_URL=http://x bash '"$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"VEILKEY_VAULTCENTER_URL"* ]]
}

@test "필수 환경변수 VEILKEY_AGENT_HASH 누락 → 에러" {
  run bash -c 'unset VEILKEY_AGENT_HASH; VEILKEY_VAULTCENTER_URL=http://x VEILKEY_LOCALVAULT_URL=http://x bash '"$SCRIPT"
  [ "$status" -ne 0 ]
  [[ "$output" == *"VEILKEY_AGENT_HASH"* ]]
}
