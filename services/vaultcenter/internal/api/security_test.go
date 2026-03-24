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

func TestDecodeJSON_MaxBodySize(t *testing.T) {
	// Build a body larger than 1 MiB
	bigBody := strings.Repeat("x", 1<<20+1)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))
	var dst map[string]any
	err := decodeJSON(req, &dst)
	if err == nil {
		t.Error("expected error for oversized body, got nil")
	}
}

func TestRemoteIP_LoopbackNotTrusted(t *testing.T) {
	// When direct connection is from private IP and X-Real-IP is loopback,
	// it should NOT trust the header and return the direct address.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-Ip", "127.0.0.1")
	ip := remoteIP(req)
	if ip == "127.0.0.1" {
		t.Errorf("remoteIP should not trust loopback X-Real-IP, got %s", ip)
	}
	if ip != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", ip)
	}
}

func TestRemoteIP_XForwardedForLoopback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1, 10.0.0.2")
	ip := remoteIP(req)
	if ip == "127.0.0.1" {
		t.Errorf("remoteIP should not trust loopback X-Forwarded-For, got %s", ip)
	}
}

func TestMaxJSONBodyConst(t *testing.T) {
	if maxJSONBody != 1<<20 {
		t.Errorf("maxJSONBody = %d, want %d", maxJSONBody, 1<<20)
	}
}
