#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
README="$ROOT_DIR/README.md"

require_line() {
  local pattern="$1"
  if ! grep -Fq "$pattern" "$README"; then
    echo "missing README pattern: $pattern" >&2
    exit 1
  fi
}

require_line "## Cross-Repo Rollout Tracker"
require_line "cross-repo tracker는 이미 결정된 runtime 철학을 다시 토론하는 자리가 아니라, 남은 follow-up만 가시화하는 용도로 유지합니다."
require_line "KeyCenter public ref grammar는 \`VK:{SCOPE}:{REF}\` / \`VE:{SCOPE}:{KEY}\` 로 고정"
require_line "KeyCenter family/scope 정책 registry는 canonical 조합만 허용"
require_line "KeyCenter token ref storage는 \`ref_canonical\`과 \`ref_family/ref_scope/ref_id\` component를 함께 유지"
require_line "HostVault는 operator/execution-boundary metadata layer로 정리됨"
require_line "KeyCenter: stale cleanup 이후의 운영 automation 고도화"
require_line "HostVault: live mail delivery verification 같은 운영 검증성 이슈"
require_line "LocalVault: child repo에서 identity/scoped model 잔여 정리"
require_line "이미 merge된 runtime 계약을 speculative language로 되돌리지 않습니다."
require_line "identity compatibility alias(\`node_id\`, \`agent_hash\`)와 ref grammar cleanup은 별도 항목으로 추적합니다."
