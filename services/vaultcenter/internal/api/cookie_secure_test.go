package api

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"veilkey-vaultcenter/internal/db"
)

// ══════════════════════════════════════════════════════════════════
// Cookie Secure flag tests
//
// These tests verify that the session cookie Secure flag is derived
// from the request's TLS state, not hardcoded. A hardcoded Secure: true
// breaks HTTP clients (e.g. vaultcenter-tui) because Go's cookie jar
// refuses to send Secure cookies over plain HTTP.
// ══════════════════════════════════════════════════════════════════

const testAdminPassword = "test-admin-pw-2026!"

func newTestServer(t *testing.T) (*Server, *http.ServeMux) {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := database.SetAdminPassword(testAdminPassword); err != nil {
		t.Fatalf("failed to set admin password: %v", err)
	}

	kek := make([]byte, 32)
	for i := range kek {
		kek[i] = byte(i)
	}
	srv := NewServer(database, kek, []string{"127.0.0.1", "192.168.1.0/24"})

	mux := http.NewServeMux()
	srv.SetupAPIRoutes(mux)
	return srv, mux
}

func loginRequest(password string) *http.Request {
	body := `{"password":"` + password + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}

func parseCookieSecure(header string) (found bool, secure bool) {
	if !strings.Contains(header, "vk_session") {
		return false, false
	}
	return true, strings.Contains(header, "Secure")
}

// ── Login over HTTP ─────────────────────────────────────────────

func TestCookie_Login_HTTP_NoSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	// Plain HTTP: no TLS, no forwarded header
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: status %d, body: %s", rec.Code, rec.Body.String())
	}

	cookie := rec.Header().Get("Set-Cookie")
	found, secure := parseCookieSecure(cookie)
	if !found {
		t.Fatal("vk_session cookie not set on successful login")
	}
	if secure {
		t.Error("HTTP login must NOT set Secure flag — Go cookie jar will refuse to send it back over HTTP")
	}
}

// ── Login over HTTPS (TLS) ──────────────────────────────────────

func TestCookie_Login_HTTPS_HasSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: status %d, body: %s", rec.Code, rec.Body.String())
	}

	cookie := rec.Header().Get("Set-Cookie")
	found, secure := parseCookieSecure(cookie)
	if !found {
		t.Fatal("vk_session cookie not set on successful login")
	}
	if !secure {
		t.Error("HTTPS login MUST set Secure flag to prevent cookie leakage over HTTP")
	}
}

// ── Login behind reverse proxy (X-Forwarded-Proto) ──────────────

func TestCookie_Login_ForwardedHTTPS_HasSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: status %d, body: %s", rec.Code, rec.Body.String())
	}

	cookie := rec.Header().Get("Set-Cookie")
	_, secure := parseCookieSecure(cookie)
	if !secure {
		t.Error("request with X-Forwarded-Proto: https must set Secure flag")
	}
}

func TestCookie_Login_ForwardedHTTP_NoSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest(testAdminPassword)
	req.Header.Set("X-Forwarded-Proto", "http")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login failed: status %d, body: %s", rec.Code, rec.Body.String())
	}

	cookie := rec.Header().Get("Set-Cookie")
	_, secure := parseCookieSecure(cookie)
	if secure {
		t.Error("request with X-Forwarded-Proto: http must NOT set Secure flag")
	}
}

// ── Logout cookie matches request TLS state ─────────────────────

func TestCookie_Logout_HTTP_NoSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	found, secure := parseCookieSecure(cookie)
	if !found {
		t.Fatal("logout should always clear the session cookie")
	}
	if secure {
		t.Error("HTTP logout must NOT set Secure flag")
	}
}

func TestCookie_Logout_HTTPS_HasSecureFlag(t *testing.T) {
	_, mux := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.TLS = &tls.ConnectionState{}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	cookie := rec.Header().Get("Set-Cookie")
	_, secure := parseCookieSecure(cookie)
	if !secure {
		t.Error("HTTPS logout MUST set Secure flag")
	}
}

// ── Full E2E: login → access protected endpoint → success ───────

func TestCookie_E2E_HTTP_LoginThenAccessProtected(t *testing.T) {
	_, mux := newTestServer(t)

	// Step 1: Login over HTTP
	loginReq := loginRequest(testAdminPassword)
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Fatalf("login failed: %d", loginRec.Code)
	}

	// Extract session cookie
	cookieHeader := loginRec.Header().Get("Set-Cookie")
	if !strings.Contains(cookieHeader, "vk_session=") {
		t.Fatal("no session cookie in login response")
	}

	// Parse cookie value
	parts := strings.SplitN(cookieHeader, ";", 2)
	cookiePair := strings.TrimSpace(parts[0]) // "vk_session=xxx"

	// Step 2: Access protected endpoint with cookie
	protectedReq := httptest.NewRequest(http.MethodGet, "/api/keycenter/temp-refs", nil)
	protectedReq.RemoteAddr = "127.0.0.1:12345"
	protectedReq.Header.Set("Cookie", cookiePair)
	protectedRec := httptest.NewRecorder()
	mux.ServeHTTP(protectedRec, protectedReq)

	if protectedRec.Code == http.StatusUnauthorized {
		t.Error("protected endpoint returned 401 after login — session cookie not accepted")
	}
	if protectedRec.Code != http.StatusOK {
		t.Logf("status %d (may be expected if no temp-refs exist), body: %s",
			protectedRec.Code, protectedRec.Body.String())
	}
}

// ── Cookie always has HttpOnly and SameSite ─────────────────────

func TestCookie_Login_AlwaysHttpOnlyAndSameSiteStrict(t *testing.T) {
	_, mux := newTestServer(t)

	cases := []struct {
		name string
		tls  bool
	}{
		{"HTTP", false},
		{"HTTPS", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := loginRequest(testAdminPassword)
			if tc.tls {
				req.TLS = &tls.ConnectionState{}
			}
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			cookie := rec.Header().Get("Set-Cookie")
			if !strings.Contains(cookie, "HttpOnly") {
				t.Error("session cookie must always have HttpOnly flag")
			}
			if !strings.Contains(cookie, "SameSite=Strict") {
				t.Error("session cookie must always have SameSite=Strict")
			}
		})
	}
}

// ── Wrong password must not set cookie ──────────────────────────

func TestCookie_Login_WrongPassword_NoCookie(t *testing.T) {
	_, mux := newTestServer(t)

	req := loginRequest("wrong-password")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	cookie := rec.Header().Get("Set-Cookie")
	if strings.Contains(cookie, "vk_session") {
		t.Error("failed login must NOT set session cookie")
	}
}
