package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersMiddleware(t *testing.T) {
	handler := securityHeadersMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	tests := []struct {
		header string
		want   string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Strict-Transport-Security", "max-age=31536000; includeSubDomains"},
	}
	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.want {
			t.Errorf("header %s = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestRequireTrustedIP_UsesNetSplitHostPort(t *testing.T) {
	s := &Server{
		trustedIPs:   map[string]bool{"10.0.0.1": true},
		trustedCIDRs: nil,
	}
	handler := s.requireTrustedIP(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// IPv4 with port — net.SplitHostPort handles this correctly
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:54321"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for trusted IP with port, got %d", rec.Code)
	}

	// IPv6 with port — strings.Split(":")[0] would fail here
	s2 := &Server{
		trustedIPs:   map[string]bool{"::1": true},
		trustedCIDRs: nil,
	}
	handler2 := s2.requireTrustedIP(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "[::1]:54321"
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected 200 for IPv6 trusted IP, got %d", rec2.Code)
	}
}

func TestRequireAgentSecret_LockedReturns503(t *testing.T) {
	s := &Server{
		locked: true,
	}
	handler := s.requireAgentSecret(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when locked, got %d", rec.Code)
	}
}

func TestHandleUnlock_PasswordTooLong(t *testing.T) {
	s := &Server{
		locked: true,
	}
	longPassword := `{"password":"` + strings.Repeat("a", 300) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/unlock", strings.NewReader(longPassword))
	rec := httptest.NewRecorder()
	s.handleUnlock(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for password > 256 chars, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password too long") {
		t.Errorf("expected 'password too long' message, got %s", rec.Body.String())
	}
}
