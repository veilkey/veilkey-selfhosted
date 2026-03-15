package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestIntegration_UnlockRateLimit_BlocksAfterMaxFailures(t *testing.T) {
	const password = "rate-limit-test-pw"
	_, handler, _ := setupServerWithPassword(t, password)

	// Send 5 wrong-password attempts (default maxAttempts = 5)
	for i := 0; i < 5; i++ {
		w := postJSON(handler, "/api/unlock", map[string]string{"password": "wrong"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// 6th attempt should be rate-limited (429)
	w := postJSON(handler, "/api/unlock", map[string]string{"password": "wrong"})
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("after max failures: expected 429, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == "" {
		t.Error("expected error message in 429 response")
	}

	// Even correct password should be blocked during cooldown
	w = postJSON(handler, "/api/unlock", map[string]string{"password": password})
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("correct password during cooldown: expected 429, got %d", w.Code)
	}
}

func TestIntegration_UnlockRateLimit_RetryAfterHeader(t *testing.T) {
	const password = "retry-header-test"
	_, handler, _ := setupServerWithPassword(t, password)

	for i := 0; i < 5; i++ {
		postJSON(handler, "/api/unlock", map[string]string{"password": "wrong"})
	}

	w := postJSON(handler, "/api/unlock", map[string]string{"password": "wrong"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header in 429 response")
	}
}

func TestIntegration_UnlockRateLimit_SuccessBeforeLimit(t *testing.T) {
	const password = "success-before-limit"
	_, handler, _ := setupServerWithPassword(t, password)

	// 4 failures (under the limit of 5)
	for i := 0; i < 4; i++ {
		w := postJSON(handler, "/api/unlock", map[string]string{"password": "wrong"})
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, w.Code)
		}
	}

	// Correct password should still work
	w := postJSON(handler, "/api/unlock", map[string]string{"password": password})
	if w.Code != http.StatusOK {
		t.Errorf("correct password before limit: expected 200, got %d", w.Code)
	}
}

func TestIntegration_UnlockRateLimit_PerIPIsolation(t *testing.T) {
	const password = "per-ip-test"
	_, handler, _ := setupServerWithPassword(t, password)

	// Exhaust attempts from IP 10.0.0.1
	for i := 0; i < 5; i++ {
		postJSONFromIP(handler, "/api/unlock", "10.0.0.1:12345", map[string]string{"password": "wrong"})
	}

	// 10.0.0.1 should be blocked
	w := postJSONFromIP(handler, "/api/unlock", "10.0.0.1:12345", map[string]string{"password": "wrong"})
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("blocked IP: expected 429, got %d", w.Code)
	}

	// 10.0.0.2 should still be allowed
	w = postJSONFromIP(handler, "/api/unlock", "10.0.0.2:12345", map[string]string{"password": password})
	if w.Code != http.StatusOK {
		t.Errorf("different IP: expected 200, got %d", w.Code)
	}
}
