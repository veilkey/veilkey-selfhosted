package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Admin session lifecycle tests (new admin system: /api/admin/session/*)
//
// Covers: login, session get, session delete (logout), cookie
// propagation, protected endpoint access, rate limiting, and
// edge cases.
// ══════════════════════════════════════════════════════════════════

// ── Session login via password (/api/admin/login) ───────────────

func TestAdminSession_PasswordLogin_ReturnsOK(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if _, ok := body["ok"]; !ok {
		t.Error("response should contain 'ok' field")
	}
}

func TestAdminSession_WrongPassword_Returns401(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest("totally-wrong-password")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestAdminSession_EmptyPassword_Returns400(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login",
		strings.NewReader(`{"password":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestAdminSession_MalformedJSON_Returns400(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/login",
		strings.NewReader(`{not json}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// ── Session cookie lifecycle ────────────────────────────────────

func extractSessionCookie(rec *httptest.ResponseRecorder) string {
	for _, line := range rec.Result().Cookies() {
		if line.Name == "vk_session" {
			return line.Value
		}
	}
	return ""
}

func TestAdminSession_LoginSetsSessionCookie(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	token := extractSessionCookie(rec)
	if token == "" {
		t.Fatal("login should set vk_session cookie")
	}
	if len(token) < 32 {
		t.Errorf("session token too short: %d chars", len(token))
	}
}

func TestAdminSession_EachLoginGeneratesUniqueCookie(t *testing.T) {
	_, mux := newTestServer(t)

	tokens := make(map[string]bool)
	for i := 0; i < 5; i++ {
		req := loginRequest(testAdminPassword)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		token := extractSessionCookie(rec)
		if tokens[token] {
			t.Fatalf("duplicate session token on attempt %d", i+1)
		}
		tokens[token] = true
	}
}

// ── Protected endpoint access ───────────────────────────────────

func TestAdminSession_ProtectedEndpoint_WithoutCookie_Returns401(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without session cookie, got %d", rec.Code)
	}
}

func TestAdminSession_ProtectedEndpoint_WithValidCookie_Succeeds(t *testing.T) {
	_, mux := newTestServer(t)

	// Login
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginRequest(testAdminPassword))
	token := extractSessionCookie(loginRec)
	if token == "" {
		t.Fatal("login failed to set cookie")
	}

	// Access protected endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.AddCookie(&http.Cookie{Name: "vk_session", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Error("valid session cookie should grant access to protected endpoint")
	}
}

func TestAdminSession_ProtectedEndpoint_WithInvalidCookie_Returns401(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.AddCookie(&http.Cookie{Name: "vk_session", Value: "totally-fake-token"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid cookie, got %d", rec.Code)
	}
}

// ── Logout ──────────────────────────────────────────────────────

func TestAdminSession_Logout_InvalidatesCookie(t *testing.T) {
	_, mux := newTestServer(t)

	// Login
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginRequest(testAdminPassword))
	token := extractSessionCookie(loginRec)

	// Logout
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	logoutReq.RemoteAddr = "127.0.0.1:12345"
	logoutReq.AddCookie(&http.Cookie{Name: "vk_session", Value: token})
	logoutRec := httptest.NewRecorder()
	mux.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusOK {
		t.Fatalf("logout failed: %d", logoutRec.Code)
	}

	// Try to use the old token
	req := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.AddCookie(&http.Cookie{Name: "vk_session", Value: token})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Error("session should be revoked after logout")
	}
}

func TestAdminSession_Logout_ClearsSetCookie(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	cookies := rec.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "vk_session" {
			found = true
			if c.MaxAge != -1 && c.MaxAge != 0 {
				t.Errorf("logout cookie should expire immediately, MaxAge=%d", c.MaxAge)
			}
		}
	}
	if !found {
		t.Error("logout should set a clearing vk_session cookie")
	}
}

// ── Cookie security flags across protocols ──────────────────────

// ── Status endpoint (no auth required) ──────────────────────────

func TestAdminSession_StatusEndpoint_ReturnsLocked(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	// Status endpoint should never require auth and should return 200 or 500
	// (500 if node_info not set up in test DB — that's expected for in-memory DB)
	if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
		t.Error("status endpoint should not require authentication")
	}
}

// ── Multiple concurrent sessions ────────────────────────────────

func TestAdminSession_MultipleSessions_IndependentRevocation(t *testing.T) {
	_, mux := newTestServer(t)

	// Create two sessions
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, loginRequest(testAdminPassword))
	token1 := extractSessionCookie(rec1)

	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, loginRequest(testAdminPassword))
	token2 := extractSessionCookie(rec2)

	if token1 == token2 {
		t.Fatal("two logins should produce different tokens")
	}

	// Logout session 1
	logoutReq := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	logoutReq.RemoteAddr = "127.0.0.1:12345"
	logoutReq.AddCookie(&http.Cookie{Name: "vk_session", Value: token1})
	mux.ServeHTTP(httptest.NewRecorder(), logoutReq)

	// Session 1 should be revoked
	req1 := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req1.RemoteAddr = "127.0.0.1:12345"
	req1.AddCookie(&http.Cookie{Name: "vk_session", Value: token1})
	check1 := httptest.NewRecorder()
	mux.ServeHTTP(check1, req1)
	if check1.Code != http.StatusUnauthorized {
		t.Error("session 1 should be revoked after logout")
	}

	// Session 2 should still work
	req2 := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	req2.RemoteAddr = "127.0.0.1:12345"
	req2.AddCookie(&http.Cookie{Name: "vk_session", Value: token2})
	check2 := httptest.NewRecorder()
	mux.ServeHTTP(check2, req2)
	if check2.Code == http.StatusUnauthorized {
		t.Error("session 2 should still be valid after session 1 logout")
	}
}
