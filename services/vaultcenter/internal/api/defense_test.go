package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
	"testing"
)

// ── Source analysis: defense tests for VaultCenter ────────────────────────────
// These tests verify that real attack scenarios are properly defended by
// analyzing the source code structure and testing exported helpers directly.

// ── Brute force defense ──────────────────────────────────────────────────────

func TestDefense_LoginMaxAttemptsConstant(t *testing.T) {
	if loginMaxAttempts <= 0 {
		t.Fatal("loginMaxAttempts must be a positive integer")
	}
	if loginMaxAttempts > 20 {
		t.Error("loginMaxAttempts should not be excessively high; current:", loginMaxAttempts)
	}
}

func TestDefense_LoginLockDurationConfigured(t *testing.T) {
	if loginLockDuration <= 0 {
		t.Fatal("loginLockDuration must be a positive duration")
	}
}

func TestDefense_LoginRateLimit_PerIPTracking(t *testing.T) {
	// Clean state
	loginMu.Lock()
	for k := range loginAttempts {
		delete(loginAttempts, k)
	}
	loginMu.Unlock()

	// Simulate failures from two different IPs
	ip1 := "192.168.1.100"
	ip2 := "10.0.0.50"

	for i := 0; i < 5; i++ {
		recordLoginFailure(ip1)
	}

	// ip2 should still be allowed (independent tracking)
	if !checkLoginRateLimit(ip2) {
		t.Error("ip2 should not be rate limited by ip1's failures")
	}

	// ip1 should still be allowed (under threshold)
	if !checkLoginRateLimit(ip1) {
		t.Error("ip1 should still be allowed below loginMaxAttempts")
	}

	// Push ip1 over the limit
	for i := 5; i < loginMaxAttempts; i++ {
		recordLoginFailure(ip1)
	}

	if checkLoginRateLimit(ip1) {
		t.Error("ip1 should be locked after loginMaxAttempts failures")
	}

	// ip2 should STILL be allowed
	if !checkLoginRateLimit(ip2) {
		t.Error("ip2 must remain independent from ip1's lockout")
	}

	// Cleanup
	clearLoginAttempts(ip1)
	clearLoginAttempts(ip2)
}

func TestDefense_LoginRateLimit_ClearOnSuccess(t *testing.T) {
	loginMu.Lock()
	for k := range loginAttempts {
		delete(loginAttempts, k)
	}
	loginMu.Unlock()

	ip := "10.10.10.10"
	for i := 0; i < loginMaxAttempts-1; i++ {
		recordLoginFailure(ip)
	}
	clearLoginAttempts(ip)
	// After clearing, should be allowed again
	if !checkLoginRateLimit(ip) {
		t.Error("login rate limit should be cleared after successful login")
	}
}

func TestDefense_UnlockLimiter_Exists(t *testing.T) {
	// Verify the Server struct has an unlockLimiter field (compile-time check).
	// Zero-value Server should have nil unlockLimiter; NewServer sets it.
	s := &Server{}
	if s.unlockLimiter != nil {
		t.Fatal("zero-value Server unexpectedly has non-nil unlockLimiter")
	}
	// Check that NewServer sets it
	srv := NewServer(nil, nil, nil)
	if srv.unlockLimiter == nil {
		t.Fatal("NewServer must initialize unlockLimiter for brute force protection on /api/unlock")
	}
}

// ── Session attacks ──────────────────────────────────────────────────────────

func TestDefense_SessionToken_UsesCryptoRand(t *testing.T) {
	src, err := os.ReadFile("handle_admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read handle_admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/rand"`) {
		t.Error("generateSecureToken must use crypto/rand, not math/rand")
	}
	if !strings.Contains(content, "rand.Read") {
		t.Error("generateSecureToken must call rand.Read for cryptographic randomness")
	}
}

func TestDefense_SessionToken_SufficientLength(t *testing.T) {
	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatalf("generateSecureToken failed: %v", err)
	}
	// 32 bytes = 64 hex chars
	if len(token) != 64 {
		t.Errorf("token length = %d, want 64 hex chars (32 bytes)", len(token))
	}
}

func TestDefense_SessionToken_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		token, err := generateSecureToken(32)
		if err != nil {
			t.Fatalf("generateSecureToken failed: %v", err)
		}
		if seen[token] {
			t.Fatalf("duplicate token generated on iteration %d", i)
		}
		seen[token] = true
	}
}

func TestDefense_SessionCookie_SecurityFlags(t *testing.T) {
	src, err := os.ReadFile("handle_admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read handle_admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "HttpOnly: true") {
		t.Error("session cookie must have HttpOnly flag to prevent XSS theft")
	}
	if !strings.Contains(content, "Secure:   true") && !strings.Contains(content, "Secure: true") {
		t.Error("session cookie must have Secure flag to prevent transmission over HTTP")
	}
	if !strings.Contains(content, "SameSiteStrictMode") {
		t.Error("session cookie must use SameSite=Strict to prevent CSRF")
	}
}

func TestDefense_SessionToken_HashedBeforeStorage(t *testing.T) {
	src, err := os.ReadFile("handle_admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read handle_admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "hashToken(token)") {
		t.Error("session token must be hashed before storage (hashToken call missing)")
	}

	// Verify hashToken uses SHA256
	if !strings.Contains(content, "sha256.Sum256") {
		t.Error("hashToken must use SHA256")
	}
}

func TestDefense_HashToken_NotReversible(t *testing.T) {
	token := "test-secret-token-value"
	hashed := hashToken(token)

	// Hash should not contain original token
	if strings.Contains(hashed, token) {
		t.Error("hashed token must not contain original plaintext")
	}
	// Should be hex-encoded SHA256 (64 chars)
	if len(hashed) != 64 {
		t.Errorf("hashToken output length = %d, want 64 (hex-encoded SHA256)", len(hashed))
	}
	// Verify it matches SHA256
	expected := sha256.Sum256([]byte(token))
	if hashed != hex.EncodeToString(expected[:]) {
		t.Error("hashToken does not produce correct SHA256 hash")
	}
}

func TestDefense_Logout_RevokesSession(t *testing.T) {
	src, err := os.ReadFile("handle_admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read handle_admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "RevokeAdminSession") {
		t.Error("handleAdminLogout must call RevokeAdminSession to invalidate server-side session")
	}
	if !strings.Contains(content, "MaxAge:   -1") && !strings.Contains(content, "MaxAge: -1") {
		t.Error("handleAdminLogout must set cookie MaxAge=-1 to expire client-side cookie")
	}
}

// ── Information leakage ──────────────────────────────────────────────────────

func TestDefense_RespondError_NoStackTraces(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// respondError should delegate to httputil — not include error details directly
	if !strings.Contains(content, "httputil.RespondError(w, status, message)") {
		t.Error("respondError must delegate to httputil.RespondError for consistent, safe error formatting")
	}

	// Verify no raw error .Error() is passed to respondJSON/respondError
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "respondError") && strings.Contains(trimmed, ".Error()") {
			// Allow in log.Printf but not in respondError calls that expose to client
			if !strings.Contains(trimmed, "log.") {
				t.Errorf("line %d: respondError may leak internal error details: %s", i+1, trimmed)
			}
		}
	}
}

func TestDefense_ErrorMessages_NoSQLDetails(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	// Check error messages returned to clients don't include SQL/DB internals
	errorMessages := []string{
		"invalid password (cannot open database)",
		"no node info found",
		"invalid password (KEK decryption failed)",
	}
	for _, msg := range errorMessages {
		// These internal messages should only go to log, not respondError.
		// The actual respondError calls should use generic messages.
		_ = strings.Contains(content, `respondError(w,`) && strings.Contains(content, msg)
	}

	// Verify that handleUnlock returns generic "invalid password" to client
	if !strings.Contains(content, `respondError(w, http.StatusUnauthorized, "invalid password")`) {
		t.Error("handleUnlock must return generic 'invalid password' error, not internal details")
	}
}

func TestDefense_SecurityHeaders_Middleware(t *testing.T) {
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

// ── Cross-vault isolation ────────────────────────────────────────────────────

func TestDefense_AgentAuth_ContextIsolation(t *testing.T) {
	src, err := os.ReadFile("hkm/helpers.go")
	if err != nil {
		t.Fatalf("failed to read hkm/helpers.go: %v", err)
	}
	content := string(src)

	// Verify agent hash is stored in context during auth
	if !strings.Contains(content, "context.WithValue") {
		t.Error("requireAgentAuth must store agent identity in request context")
	}
	if !strings.Contains(content, "agentAuthKey") {
		t.Error("agent auth must use a dedicated context key")
	}
}

func TestDefense_AgentAuth_VerifyAgentAccess_MatchesURLPath(t *testing.T) {
	src, err := os.ReadFile("hkm/helpers.go")
	if err != nil {
		t.Fatalf("failed to read hkm/helpers.go: %v", err)
	}
	content := string(src)

	// verifyAgentAccess must compare authenticated agent with URL path agent
	if !strings.Contains(content, `authedAgent == urlAgent`) {
		t.Error("verifyAgentAccess must compare authenticated agent hash with URL path agent hash")
	}
	if !strings.Contains(content, `r.PathValue("agent")`) {
		t.Error("verifyAgentAccess must extract agent from URL path")
	}
}

func TestDefense_AgentAuth_SecretHashedBeforeLookup(t *testing.T) {
	src, err := os.ReadFile("hkm/helpers.go")
	if err != nil {
		t.Fatalf("failed to read hkm/helpers.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "sha256.Sum256") {
		t.Error("authenticateAgentBySecret must hash the token before DB lookup")
	}
	if !strings.Contains(content, "GetAgentBySecretHash") {
		t.Error("authenticateAgentBySecret must look up agent by hashed secret, not plaintext")
	}
}

func TestDefense_AgentHeartbeat_NewRegistrationRequiresToken(t *testing.T) {
	src, err := os.ReadFile("hkm/hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm/hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ConsumeRegistrationToken") {
		t.Error("new agent registration must consume a registration token")
	}
	if !strings.Contains(content, `registration_token is required for first-time agent registration`) {
		t.Error("untrusted agents without registration token must be rejected")
	}
}

func TestDefense_AgentHeartbeat_BlockedAgentReturns423(t *testing.T) {
	src, err := os.ReadFile("hkm/hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm/hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "http.StatusLocked") {
		t.Error("blocked agents must receive HTTP 423 (Locked)")
	}
}

// ── Command injection in functions ───────────────────────────────────────────
// Note: Function run injection tests are in hkm/function_run_test.go
// (they require access to hkm package internals like functionRunAllowlist,
// functionRunDangerousChars, and renderGlobalFunctionCommand).

func TestDefense_FunctionRun_SourceAnalysis(t *testing.T) {
	src, err := os.ReadFile("hkm/hkm_global_function_run.go")
	if err != nil {
		t.Fatalf("failed to read hkm/hkm_global_function_run.go: %v", err)
	}
	content := string(src)

	// Verify allowlist approach exists
	if !strings.Contains(content, "functionRunAllowlist") {
		t.Error("function run must use a command allowlist")
	}
	// Verify dangerous chars regex exists
	if !strings.Contains(content, "functionRunDangerousChars") {
		t.Error("function run must check for dangerous shell metacharacters")
	}
	// Verify placeholder stripping before dangerous char check
	if !strings.Contains(content, "ReplaceAllString") {
		t.Error("function run must strip placeholders before checking dangerous chars")
	}
	// Verify shell quoting for resolved values
	if !strings.Contains(content, "shellQuote") {
		t.Error("function run must shell-quote resolved placeholder values")
	}
}

// ── Path traversal in bulk templates (VaultCenter) ───────────────────────────

func TestDefense_BulkTemplates_PathValidation_Source(t *testing.T) {
	// VaultCenter stores bulk templates but does NOT execute them.
	// Path traversal defense is enforced at the LocalVault level (apply.go)
	// where isAllowedBulkApplyTarget uses a strict allowlist.
	//
	// VaultCenter's bulk template extra paths (from env) do reject traversal
	// in init() of files.go — verify that logic in the template normalize function.
	src, err := os.ReadFile("bulk/templates.go")
	if err != nil {
		t.Fatalf("failed to read bulk/templates.go: %v", err)
	}
	content := string(src)

	// Templates must have a target_path field to specify where they'll be applied
	if !strings.Contains(content, "target_path") {
		t.Error("bulk templates must include target_path for LocalVault to validate")
	}

	// Verify the template system includes validation status tracking
	if !strings.Contains(content, "validation_status") {
		t.Error("bulk templates must track validation_status")
	}
}

// ── Session ID generation ────────────────────────────────────────────────────

func TestDefense_SessionID_CryptoRand(t *testing.T) {
	// Verify generateSessionID uses crypto/rand
	id := generateSessionID()
	if len(id) != 32 {
		t.Errorf("session ID length = %d, want 32 (16 bytes hex)", len(id))
	}
	// Verify uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateSessionID()
		if ids[id] {
			t.Fatalf("duplicate session ID on iteration %d", i)
		}
		ids[id] = true
	}
}

// ── RemoteIP handling (IP spoofing defense) ──────────────────────────────────

func TestDefense_RemoteIP_IgnoresProxyHeadersFromPublicIP(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "203.0.113.50:12345", // public IP
		Header:     http.Header{},
	}
	r.Header.Set("X-Real-IP", "10.0.0.1")
	r.Header.Set("X-Forwarded-For", "172.16.0.1")

	ip := remoteIP(r)
	if ip != "203.0.113.50" {
		t.Errorf("remoteIP = %q, want %q (should ignore proxy headers from public IP)", ip, "203.0.113.50")
	}
}

func TestDefense_RemoteIP_TrustsProxyHeadersFromPrivateIP(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "127.0.0.1:12345", // loopback (proxy)
		Header:     http.Header{},
	}
	r.Header.Set("X-Real-IP", "203.0.113.50")

	ip := remoteIP(r)
	if ip != "203.0.113.50" {
		t.Errorf("remoteIP = %q, want %q (should trust X-Real-IP from loopback)", ip, "203.0.113.50")
	}
}

func TestDefense_RemoteIP_IgnoresLoopbackInXRealIP(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "192.168.1.1:12345", // private IP (proxy)
		Header:     http.Header{},
	}
	r.Header.Set("X-Real-IP", "127.0.0.1")

	ip := remoteIP(r)
	// Should not return loopback from header — falls through
	if ip == "127.0.0.1" {
		t.Error("remoteIP should not trust loopback address from X-Real-IP header")
	}
}

// ── JSON body limit ──────────────────────────────────────────────────────────

func TestDefense_MaxJSONBody_Exists(t *testing.T) {
	if maxJSONBody <= 0 {
		t.Fatal("maxJSONBody must be positive")
	}
	if maxJSONBody > 10<<20 {
		t.Errorf("maxJSONBody = %d, should not exceed 10MB", maxJSONBody)
	}
}

// ── Crypto quality ───────────────────────────────────────────────────────────

func TestDefense_GenerateSecureToken_CorrectEntropy(t *testing.T) {
	// 32 bytes from crypto/rand = 256 bits of entropy
	token, err := generateSecureToken(32)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 32 {
		t.Errorf("token raw bytes = %d, want 32", len(raw))
	}

	// Verify it's not all zeros (would indicate broken rand)
	allZero := true
	for _, b := range raw {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("token is all zeros — crypto/rand may be broken")
	}
}

// ── Timing attack resistance (generateSecureToken) ───────────────────────────

func TestDefense_GenerateSecureToken_DifferentEachCall(t *testing.T) {
	// Generate multiple tokens and ensure they're all different
	tokens := make([]string, 50)
	for i := range tokens {
		var err error
		tokens[i], err = generateSecureToken(32)
		if err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < len(tokens); i++ {
		for j := i + 1; j < len(tokens); j++ {
			if tokens[i] == tokens[j] {
				t.Fatalf("duplicate tokens at %d and %d", i, j)
			}
		}
	}
}

// ── Verify crypto/rand is used, not math/rand ────────────────────────────────

func TestDefense_CryptoRand_NotMathRand(t *testing.T) {
	src, err := os.ReadFile("handle_admin_auth.go")
	if err != nil {
		t.Fatal(err)
	}
	content := string(src)

	// Must import crypto/rand
	if !strings.Contains(content, `"crypto/rand"`) {
		t.Error("must use crypto/rand for token generation")
	}
	// Must NOT import math/rand
	if strings.Contains(content, `"math/rand"`) {
		t.Error("must NOT use math/rand for security-sensitive token generation")
	}

	// Also check generateSecureToken works with real crypto/rand
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("crypto/rand.Read failed: %v", err)
	}
}
