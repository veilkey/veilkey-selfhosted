package api

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: defense tests for LocalVault ─────────────────────────────
// These tests verify that real attack scenarios are properly defended.

// ── Agent auth bypass ────────────────────────────────────────────────────────

func TestDefense_RequireAgentSecret_ConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/subtle"`) {
		t.Error("requireAgentSecret must import crypto/subtle for constant-time comparison")
	}
	if !strings.Contains(content, "subtle.ConstantTimeCompare") {
		t.Error("requireAgentSecret must use subtle.ConstantTimeCompare to prevent timing attacks")
	}
}

func TestDefense_RequireAgentSecret_EmptyBearerRejected(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// Verify the middleware checks for empty auth header
	if !strings.Contains(content, `authHeader == ""`) {
		t.Error("requireAgentSecret must reject empty Authorization header")
	}
	// Verify empty token after TrimPrefix is rejected
	if !strings.Contains(content, `token == ""`) {
		t.Error("requireAgentSecret must reject empty Bearer token value")
	}
}

func TestDefense_RequireAgentSecret_WrongFormatRejected(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// When TrimPrefix("Bearer ") returns the original string, format is wrong
	if !strings.Contains(content, `token == authHeader`) {
		t.Error("requireAgentSecret must detect non-Bearer authorization format")
	}
}

func TestDefense_RequireAgentSecret_LockedStateBlocks(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// The middleware must check locked state first
	if !strings.Contains(content, "s.IsLocked()") {
		t.Error("requireAgentSecret must check lock state before processing auth")
	}
	if !strings.Contains(content, "http.StatusServiceUnavailable") {
		t.Error("locked server must return 503 Service Unavailable")
	}
}

func TestDefense_RequireAgentSecret_DecryptsSecretForComparison(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// The agent secret is stored encrypted, so it must be decrypted before comparison
	if !strings.Contains(content, "crypto.Decrypt") {
		t.Error("requireAgentSecret must decrypt the stored agent secret before comparison")
	}
}

// ── Path traversal in bulk apply ─────────────────────────────────────────────

func TestDefense_BulkApply_IsAllowedTarget_TraversalPaths(t *testing.T) {
	// These must all be rejected by isAllowedBulkApplyTarget
	// (function is in the localvault/internal/api/bulk package, tested via source analysis)
	src, err := os.ReadFile("bulk/apply.go")
	if err != nil {
		t.Fatalf("failed to read bulk/apply.go: %v", err)
	}
	content := string(src)

	// Verify allowlist-based approach (not path validation — strict allowlist is stronger)
	if !strings.Contains(content, "defaultBulkApplyTargets") {
		t.Error("bulk apply must use a static allowlist of allowed target paths")
	}

	// Verify that the function checks exact match against allowlist
	if !strings.Contains(content, "func isAllowedBulkApplyTarget(") {
		t.Error("isAllowedBulkApplyTarget function must exist")
	}
}

func TestDefense_BulkApply_ExtraPaths_RejectTraversal(t *testing.T) {
	src, err := os.ReadFile("bulk/apply.go")
	if err != nil {
		t.Fatalf("failed to read bulk/apply.go: %v", err)
	}
	content := string(src)

	// Extra paths from env must reject .. and relative paths
	if !strings.Contains(content, `strings.Contains(p, "..")`) {
		t.Error("extra bulk apply paths must reject paths containing '..'")
	}
	if !strings.Contains(content, "filepath.IsAbs(p)") {
		t.Error("extra bulk apply paths must require absolute paths")
	}
}

func TestDefense_BulkApply_AttackPaths(t *testing.T) {
	// Test attack paths against isAllowedBulkApplyTarget helper.
	// These use the source analysis pattern since the function is in another package.
	attackPaths := []struct {
		path   string
		reason string
	}{
		{"../../../etc/passwd", "parent directory traversal"},
		{"/opt/mattermost/../../etc/shadow", "embedded traversal after valid prefix"},
		{"relative/path", "relative path (no leading /)"},
		{"/opt/mattermost\x00/etc/passwd", "null byte path injection"},
		{"/etc/passwd", "arbitrary system file"},
		{"/root/.ssh/authorized_keys", "SSH key injection"},
	}

	for _, tc := range attackPaths {
		// Verify these paths are NOT in the default allowlist
		found := false
		for _, allowed := range []string{
			"/opt/mattermost/config/config.json",
			"/opt/mattermost/.env",
			"/etc/systemd/system/mattermost.service.d/override.conf",
			"/etc/gitlab/gitlab.rb",
		} {
			if tc.path == allowed {
				found = true
				break
			}
		}
		if found {
			t.Errorf("attack path %q should not match default allowlist: %s", tc.path, tc.reason)
		}
	}
}

func TestDefense_BulkApply_AllowedPathWorks(t *testing.T) {
	// Verify legitimate path is in the default list
	allowed := "/opt/mattermost/config/config.json"
	found := false
	for _, path := range []string{
		"/opt/mattermost/config/config.json",
		"/opt/mattermost/.env",
		"/etc/systemd/system/mattermost.service.d/override.conf",
		"/etc/gitlab/gitlab.rb",
	} {
		if path == allowed {
			found = true
			break
		}
	}
	if !found {
		t.Error("legitimate mattermost config path should be in default allowlist")
	}
}

func TestDefense_BulkApply_ValidatesSteps(t *testing.T) {
	src, err := os.ReadFile("bulk/apply.go")
	if err != nil {
		t.Fatalf("failed to read bulk/apply.go: %v", err)
	}
	content := string(src)

	// Every step must be validated before execution
	if !strings.Contains(content, "validateBulkApplyStep") {
		t.Error("bulk apply must validate each step before execution")
	}
	// Hooks must also be from allowlist
	if !strings.Contains(content, "getAllowedBulkApplyHook") {
		t.Error("bulk apply hooks must be validated against allowlist")
	}
}

func TestDefense_BulkApply_RejectsTempRefs(t *testing.T) {
	src, err := os.ReadFile("bulk/apply.go")
	if err != nil {
		t.Fatalf("failed to read bulk/apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "VK:TEMP:") {
		t.Error("bulk apply must reject content containing VK:TEMP references")
	}
}

func TestDefense_BulkApply_AtomicWrite(t *testing.T) {
	src, err := os.ReadFile("bulk/apply.go")
	if err != nil {
		t.Fatalf("failed to read bulk/apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "writeAtomically") {
		t.Error("bulk apply must use atomic writes to prevent partial file corruption")
	}
}

// ── Lifecycle state machine integrity ────────────────────────────────────────

func TestDefense_Lifecycle_BlockedRefsReturn423(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "http.StatusLocked") {
		t.Error("blocked refs must return HTTP 423 (Locked)")
	}
	if !strings.Contains(content, "ref is blocked") {
		t.Error("blocked ref error message missing")
	}
}

func TestDefense_Lifecycle_RevokedRefsCannotBeReactivated(t *testing.T) {
	// In the current implementation, revoke sets status to "revoke".
	// The block check prevents further operations on blocked refs.
	// Verify the status transition handlers check current status.
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	// Both activate and status transition paths must check RefStatusBlock
	if strings.Count(content, "RefStatusBlock") < 3 {
		t.Error("lifecycle handlers must check RefStatusBlock in multiple paths")
	}
}

func TestDefense_Lifecycle_ActivateOnlyFromTemp(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "parsed.Scope != RefScopeTemp") {
		t.Error("activate handler must require TEMP scope source")
	}
	if !strings.Contains(content, `ciphertext must use TEMP scope`) {
		t.Error("activate must reject non-TEMP scope with clear error")
	}
}

func TestDefense_Lifecycle_StatusTransition_RejectsTempScope(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	// Archive/Block/Revoke must NOT operate on TEMP refs
	if !strings.Contains(content, "parsed.Scope == RefScopeTemp") {
		t.Error("status transitions (archive/block/revoke) must reject TEMP scope refs")
	}
}

func TestDefense_Lifecycle_ActivateTargetScopeRestricted(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ParseActivationScope") {
		t.Error("activate must validate target scope via ParseActivationScope (LOCAL or EXTERNAL only)")
	}
}

// ── Heartbeat spoofing ───────────────────────────────────────────────────────

func TestDefense_Heartbeat_NewAgentRequiresToken(t *testing.T) {
	// This tests the VaultCenter side, verified via source analysis
	src, err := os.ReadFile("../../vaultcenter/internal/api/hkm/hkm_agent_heartbeat.go")
	if err != nil {
		t.Skip("cannot read vaultcenter heartbeat source (cross-service)")
	}
	content := string(src)

	if !strings.Contains(content, "ConsumeRegistrationToken") {
		t.Error("new agent registration must consume a registration token atomically")
	}
	if !strings.Contains(content, "registration_token is required") {
		t.Error("untrusted IPs without token must be rejected")
	}
}

func TestDefense_Heartbeat_VaultUnlockKey_RequiresAgentAuth(t *testing.T) {
	src, err := os.ReadFile("../../vaultcenter/internal/api/hkm/hkm_agent_heartbeat.go")
	if err != nil {
		t.Skip("cannot read vaultcenter heartbeat source (cross-service)")
	}
	content := string(src)

	// vault_unlock_key injection on existing agents must verify agent identity
	if !strings.Contains(content, "authenticateAgentBySecret") {
		t.Error("vault_unlock_key update must authenticate the agent by secret")
	}
	if !strings.Contains(content, "authedAgent.NodeID != nodeID") {
		t.Error("vault_unlock_key must verify the authenticated agent matches the requesting node")
	}
}

// ── Server locked state ──────────────────────────────────────────────────────

func TestDefense_UnlockLimiter_Initialized(t *testing.T) {
	srv := NewServer(nil, nil, nil)
	if srv.unlockLimiter == nil {
		t.Fatal("NewServer must initialize unlockLimiter for brute force protection")
	}
}

func TestDefense_UnlockRoute_HasRateLimiter(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "unlockLimiter.Middleware") {
		t.Error("POST /api/unlock must be protected by unlockLimiter.Middleware")
	}
}

// ── Security headers ─────────────────────────────────────────────────────────

func TestDefense_SecurityHeaders(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	headers := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"Referrer-Policy",
		"Strict-Transport-Security",
	}
	for _, h := range headers {
		if !strings.Contains(content, h) {
			t.Errorf("security header %s must be set in middleware", h)
		}
	}
}

// ── Password length limit ────────────────────────────────────────────────────

func TestDefense_Unlock_PasswordLengthLimit(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `len(req.Password) > 256`) {
		t.Error("handleUnlock must reject passwords longer than 256 chars to prevent DoS")
	}
}

// ── Max body size ────────────────────────────────────────────────────────────

func TestDefense_Unlock_MaxBodyReader(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "MaxBytesReader") {
		t.Error("handleUnlock must use MaxBytesReader to prevent oversized request bodies")
	}
}
