#!/usr/bin/env bash
# vk-bulk-apply-sync.sh — VeilKey bulk-apply 자동 동기화 파이프라인
#
# 동작:
#   1. LocalVault에 등록된 시크릿 이름 목록을 기준으로 동작 (= 키명이 없으면 대상 아님)
#   2. VaultCenter에서 해당 키 resolve — 같은 이름이 다른 vault에 있으면 재사용 (통일성)
#   3. 현재 .env와 비교하여 변경 사항이 있을 때만 bulk-apply 실행
#   4. 변경 시 post-deploy hook 실행 (서비스 재시작 등)
#
# 환경변수:
#   VEILKEY_VAULTCENTER_URL   VaultCenter URL (필수)
#   VEILKEY_AGENT_HASH        이 vault의 agent hash (필수)
#   VEILKEY_LOCALVAULT_URL    LocalVault URL (필수)
#   VEILKEY_TEMPLATE_NAME     bulk-apply 템플릿 이름 (기본: soulflow-env)
#   VEILKEY_TARGET_PATH       .env 배포 경로 (기본: /root/workspace/.env)
#   VEILKEY_POST_HOOK         변경 후 실행할 명령 (선택)
#   VEILKEY_DRY_RUN           1이면 실제 적용 안 함

set -euo pipefail

VC_URL="${VEILKEY_VAULTCENTER_URL:?VEILKEY_VAULTCENTER_URL is required}"
AGENT="${VEILKEY_AGENT_HASH:?VEILKEY_AGENT_HASH is required}"
LV_URL="${VEILKEY_LOCALVAULT_URL:?VEILKEY_LOCALVAULT_URL is required}"
TEMPLATE="${VEILKEY_TEMPLATE_NAME:-soulflow-env}"
TARGET="${VEILKEY_TARGET_PATH:-/root/workspace/.env}"
POST_HOOK="${VEILKEY_POST_HOOK:-}"
DRY_RUN="${VEILKEY_DRY_RUN:-0}"
LOG_TAG="vk-sync"

log() { echo "[$(date '+%Y-%m-%d %H:%M:%S')] [$LOG_TAG] $*"; }
die() { log "ERROR: $*" >&2; exit 1; }

CURL_OPTS="${VEILKEY_CURL_OPTS:--sk}"
CURL="curl $CURL_OPTS --max-time 10"

# ── 1. LocalVault에 등록된 시크릿 이름 목록 = sync 대상 ──
log "Fetching secret names from LocalVault"
SECRET_NAMES=$($CURL "$LV_URL/api/secrets" | \
  python3 -c "
import json, sys
d = json.load(sys.stdin)
for s in d.get('secrets', []):
    if s.get('status') == 'active':
        print(s['name'])
" 2>/dev/null) || die "Failed to fetch secrets from LocalVault"

if [ -z "$SECRET_NAMES" ]; then
  log "No active secrets in LocalVault — nothing to sync"
  exit 0
fi

SECRET_COUNT=$(echo "$SECRET_NAMES" | wc -l)
log "Found $SECRET_COUNT active secrets in LocalVault"

# ── 2. 전체 vault 목록 가져오기 (같은 키 재사용 위해) ──
log "Fetching agents for cross-vault key lookup"
ALL_AGENTS=$($CURL "$VC_URL/api/agents" | \
  python3 -c "
import json, sys
agents = json.load(sys.stdin)
for a in agents:
    h = a.get('agent_hash', '')
    if h: print(h)
" 2>/dev/null) || ALL_AGENTS=""

# ── 3. 시크릿 resolve (이 vault 우선 → 다른 vault fallback) ──
log "Resolving secrets..."
declare -A RESOLVED

resolve_from_agent() {
  local agent_hash="$1" name="$2"
  $CURL "$VC_URL/api/agents/$agent_hash/secrets/$name" 2>/dev/null | \
    python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    v = d.get('value', '')
    if v: print(v)
except: pass
" 2>/dev/null
}

while IFS= read -r name; do
  [ -z "$name" ] && continue

  # 이 vault에서 먼저 resolve
  val=$(resolve_from_agent "$AGENT" "$name")

  # 없으면 다른 vault에서 같은 이름 찾기 (통일성)
  if [ -z "$val" ]; then
    for other_agent in $ALL_AGENTS; do
      [ "$other_agent" = "$AGENT" ] && continue
      val=$(resolve_from_agent "$other_agent" "$name")
      if [ -n "$val" ]; then
        log "  $name: reused from agent $other_agent"
        break
      fi
    done
  fi

  if [ -n "$val" ]; then
    RESOLVED["$name"]="$val"
    log "  $name: resolved (${#val} chars)"
  else
    log "  $name: MISSING — no value in any vault"
  fi
done <<< "$SECRET_NAMES"

# ── 4. .env 내용 생성 ──
ENV_CONTENT="# VeilKey managed — auto-synced $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
while IFS= read -r name; do
  [ -z "$name" ] && continue
  ENV_CONTENT="$ENV_CONTENT
$name=${RESOLVED[$name]:-}"
done <<< "$SECRET_NAMES"
ENV_CONTENT="$ENV_CONTENT
"

# ── 5. 변경 감지 (체크섬 기반, 타임스탬프 무시) ──
CHECKSUM_FILE="${TARGET}.vk-checksum"
NEW_CHECKSUM=$(echo "$ENV_CONTENT" | grep -v '^# VeilKey managed' | sort | md5sum | cut -d' ' -f1)
if [ -f "$CHECKSUM_FILE" ]; then
  OLD_CHECKSUM=$(cat "$CHECKSUM_FILE" 2>/dev/null)
  if [ "$NEW_CHECKSUM" = "$OLD_CHECKSUM" ]; then
    log "No changes detected — skipping"
    exit 0
  fi
fi

log "Changes detected — applying"

# ── 6. bulk-apply 실행 ──
if [ "$DRY_RUN" = "1" ]; then
  log "[DRY RUN] Would write to $TARGET:"
  echo "$ENV_CONTENT" | sed 's/=.\{8\}/=****/g'
  exit 0
fi

PAYLOAD=$(python3 -c "
import json, sys
content = sys.stdin.read()
print(json.dumps({
    'name': '$TEMPLATE-deploy',
    'steps': [{
        'name': '$TEMPLATE',
        'format': 'env',
        'target_path': '$TARGET',
        'content': content
    }]
}))
" <<< "$ENV_CONTENT")

RESULT=$($CURL "$LV_URL/api/bulk-apply/execute" \
  -X POST \
  -H 'Content-Type: application/json' \
  -d "$PAYLOAD" 2>/dev/null)

STATUS=$(echo "$RESULT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null)

if [ "$STATUS" != "applied" ]; then
  die "bulk-apply failed: $RESULT"
fi

# 변경 감지 기준용 체크섬 저장
echo "$NEW_CHECKSUM" > "$CHECKSUM_FILE"

# fallback: LocalVault bulk-apply가 파일을 쓰지만, 독립 실행 시에도 보장
[ -f "$TARGET" ] || echo "$ENV_CONTENT" > "$TARGET"

log "bulk-apply applied to $TARGET"

# ── 7. post-deploy hook ──
if [ -n "$POST_HOOK" ]; then
  log "Running post-hook: $POST_HOOK"
  eval "$POST_HOOK" || log "WARNING: post-hook failed (exit $?)"
fi

log "Done"
