package admin

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: admin_auth.go — session management and TOTP ──────────────

func TestSource_AdminSessionCookieName(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `adminSessionCookieName`) {
		t.Error("admin session cookie name constant must be defined")
	}
	if !strings.Contains(content, `"vk_session"`) {
		t.Error("admin session cookie name must be vk_session")
	}
}

func TestSource_SessionTTLUsesEnvDuration(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func adminSessionTTL()`) {
		t.Error("adminSessionTTL function must exist")
	}
	if !strings.Contains(content, `envDuration("VEILKEY_ADMIN_SESSION_TTL"`) {
		t.Error("session TTL must use envDuration with VEILKEY_ADMIN_SESSION_TTL")
	}
}

func TestSource_SessionIdleTimeoutUsesEnvDuration(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func adminSessionIdleTimeout()`) {
		t.Error("adminSessionIdleTimeout function must exist")
	}
	if !strings.Contains(content, `envDuration("VEILKEY_ADMIN_SESSION_IDLE_TIMEOUT"`) {
		t.Error("idle timeout must use envDuration with VEILKEY_ADMIN_SESSION_IDLE_TIMEOUT")
	}
}

func TestSource_TOTPUsesConstantTimeCompare(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/subtle"`) {
		t.Error("admin_auth.go must import crypto/subtle for TOTP comparison")
	}
	if !strings.Contains(content, `subtle.ConstantTimeCompare`) {
		t.Error("TOTP verification must use subtle.ConstantTimeCompare")
	}
}

func TestSource_TOTPWindowTolerance(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	// verifyTOTP must check offsets -1, 0, +1
	if !strings.Contains(content, `func verifyTOTP(`) {
		t.Error("verifyTOTP function must exist")
	}
	if !strings.Contains(content, `for offset := -1; offset <= 1; offset++`) {
		t.Error("TOTP verification must check window offsets -1, 0, +1")
	}
}

func TestSource_RevealWindowDurationConstant(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `adminRevealWindow`) {
		t.Error("adminRevealWindow constant must be defined")
	}
	if !strings.Contains(content, `5 * time.Minute`) {
		t.Error("reveal window must be 5 minutes")
	}
}

func TestSource_CookieFlags_HttpOnly_Secure_SameSiteStrict(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func setAdminSessionCookie(`) {
		t.Fatal("setAdminSessionCookie function must exist")
	}

	fnBody := extractFnBody(content, "func setAdminSessionCookie(")
	if !strings.Contains(fnBody, "HttpOnly: true") {
		t.Error("session cookie must have HttpOnly: true")
	}
	if !strings.Contains(fnBody, "Secure:   true") && !strings.Contains(fnBody, "Secure: true") {
		t.Error("session cookie must have Secure: true")
	}
	if !strings.Contains(fnBody, "SameSite: http.SameSiteStrictMode") {
		t.Error("session cookie must have SameSite: http.SameSiteStrictMode")
	}
}

func TestSource_TokenHashUsesSHA256(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func hashAdminSessionToken(`) {
		t.Error("hashAdminSessionToken function must exist")
	}
	fnBody := extractFnBody(content, "func hashAdminSessionToken(")
	if !strings.Contains(fnBody, "sha256.Sum256") {
		t.Error("token hash must use SHA256")
	}
}

// ── Source analysis: passkey.go — COSE key validation ─────────────────────────

func TestSource_Passkey_COSEKeyValidation(t *testing.T) {
	src, err := os.ReadFile("passkey.go")
	if err != nil {
		t.Fatalf("failed to read passkey.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func parseCOSEES256Key(`) {
		t.Error("parseCOSEES256Key function must exist for COSE key validation")
	}
	// Must validate key type EC2
	if !strings.Contains(content, `kty != 2`) {
		t.Error("COSE key validation must check kty == 2 (EC2)")
	}
	// Must validate algorithm ES256
	if !strings.Contains(content, `alg != -7`) {
		t.Error("COSE key validation must check alg == -7 (ES256)")
	}
	// Must validate curve P-256
	if !strings.Contains(content, `crv != 1`) {
		t.Error("COSE key validation must check crv == 1 (P-256)")
	}
	// Must validate point is on curve
	if !strings.Contains(content, `IsOnCurve`) {
		t.Error("COSE key validation must check IsOnCurve")
	}
}

func TestSource_Passkey_ChallengeUsesCryptoRand(t *testing.T) {
	src, err := os.ReadFile("passkey.go")
	if err != nil {
		t.Fatalf("failed to read passkey.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"crypto/rand"`) {
		t.Error("passkey.go must import crypto/rand for challenge generation")
	}
	if !strings.Contains(content, `rand.Read(challenge)`) {
		t.Error("passkey challenge must use crypto/rand.Read")
	}
}

// ── Source analysis: handler.go — route registration ──────────────────────────

func TestSource_Handler_RequireAdminSessionMiddleware(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func (h *Handler) RequireAdminSession(`) {
		t.Error("RequireAdminSession middleware must exist")
	}
	if !strings.Contains(content, `"admin session required"`) {
		t.Error("RequireAdminSession must return 'admin session required' error")
	}
}

func TestSource_Handler_ProtectedRoutesUseAdminSession(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	protectedRoutes := []string{
		"GET /api/admin/approval-challenges",
		"GET /api/admin/audit/recent",
		"POST /api/admin/reveal-authorize",
		"POST /api/admin/reveal",
	}
	for _, route := range protectedRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "RequireAdminSession") {
					t.Errorf("route %s must be wrapped with RequireAdminSession", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("protected route not registered: %s", route)
		}
	}
}

// ── Source analysis: preview handlers ─────────────────────────────────────────

func TestSource_StaticPreviewHandlersExist(t *testing.T) {
	files := map[string][]string{
		"admin_vue_preview.go": {
			"func (h *Handler) HandleAdminVuePreview(",
		},
		"admin_html_only_preview.go": {
			"func (h *Handler) HandleAdminHTMLOneShotPreview(",
		},
		"admin_mockup_previews.go": {
			"func (h *Handler) HandleAdminMockupDark(",
			"func (h *Handler) HandleAdminMockupAmber(",
			"func (h *Handler) HandleAdminMockupMono(",
		},
	}

	for file, sigs := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("failed to read %s: %v", file, err)
		}
		content := string(src)
		for _, sig := range sigs {
			if !strings.Contains(content, sig) {
				t.Errorf("preview handler must exist in %s: %s", file, sig)
			}
		}
	}
}

// ── Source analysis: admin_auth.go — TOTP algorithm uses SHA256 ───────────────

func TestSource_TOTPAlgorithmUsesSHA256(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	fnBody := extractFnBody(content, "func totpCode(")
	if !strings.Contains(fnBody, "hmac.New(sha256.New") {
		t.Error("TOTP code generation must use HMAC-SHA256")
	}
}

// ── Bug 2: Reveal window must NOT be extended if already active ───────────────

func TestRevealWindowNotExtendable(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	fnBody := extractFnBody(content, "func (h *Handler) handleAdminRevealAuthorize(")
	if fnBody == "" {
		t.Fatal("handleAdminRevealAuthorize must exist")
	}

	// Must check if session already has an active reveal window before extending
	if !strings.Contains(fnBody, "session.RevealUntil") {
		t.Error("handleAdminRevealAuthorize must check session.RevealUntil")
	}

	// Must check if existing window is still active (not expired)
	if !strings.Contains(fnBody, "RevealUntil != nil") || (!strings.Contains(fnBody, "Before(") && !strings.Contains(fnBody, "After(")) {
		t.Error("handleAdminRevealAuthorize must check if existing reveal window is still active before extending")
	}
}

func TestSource_TOTPURIIncludesAlgorithmSHA256(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	fnBody := extractFnBody(content, "func buildTOTPURI(")
	if !strings.Contains(fnBody, `"SHA256"`) {
		t.Error("TOTP URI must specify algorithm=SHA256")
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

func extractFnBody(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 {
		return ""
	}
	rest := code[i:]
	next := strings.Index(rest[1:], "\nfunc ")
	if next < 0 {
		return rest
	}
	return rest[:next+1]
}
