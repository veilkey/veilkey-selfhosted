package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestTempEncryptEndpoint(t *testing.T) {
	_, handler := setupTestServer(t)

	w := postJSON(handler, "/api/encrypt", map[string]string{"plaintext": "test-secret-value"})
	if w.Code != http.StatusOK {
		t.Errorf("POST /api/encrypt should return 200, got %d: %s", w.Code, w.Body.String())
		return
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if !strings.HasPrefix(resp["ref"], "VK:TEMP:") {
		t.Errorf("ref should start with VK:TEMP:, got %s", resp["ref"])
	}
	if resp["expires_at"] == "" {
		t.Error("expires_at should not be empty")
	}
}

func TestTempEncryptEmptyPlaintext(t *testing.T) {
	_, handler := setupTestServer(t)

	w := postJSON(handler, "/api/encrypt", map[string]string{"plaintext": ""})
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST /api/encrypt with empty plaintext should return 400, got %d", w.Code)
	}
}

func TestTempEncryptWithName(t *testing.T) {
	_, handler := setupTestServer(t)

	w := postJSON(handler, "/api/encrypt", map[string]any{
		"plaintext": "my-secret",
		"name":      "MY_API_KEY",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["name"] != "MY_API_KEY" {
		t.Errorf("expected name MY_API_KEY, got %s", resp["name"])
	}
}

func TestTempEncryptOnlyProducesTempScope(t *testing.T) {
	_, handler := setupTestServer(t)

	w := postJSON(handler, "/api/encrypt", map[string]string{"plaintext": "secret"})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	ref := resp["ref"]

	if !strings.HasPrefix(ref, "VK:TEMP:") {
		t.Fatalf("encrypt must only produce TEMP scope, got %s", ref)
	}

	// Verify it's not LOCAL or any other scope
	parts := strings.SplitN(ref, ":", 3)
	if len(parts) != 3 || parts[1] != "TEMP" {
		t.Fatalf("scope must be TEMP, got %s", parts[1])
	}
}

func TestKeycenterRejectsLocalSecretViaEncrypt(t *testing.T) {
	// Encrypt API must never produce LOCAL refs — only TEMP
	_, handler := setupTestServer(t)

	// Even if someone tries to abuse the API, result must be TEMP
	for i := 0; i < 5; i++ {
		w := postJSON(handler, "/api/encrypt", map[string]string{"plaintext": "attempt"})
		if w.Code != http.StatusOK {
			continue
		}
		var resp map[string]string
		json.Unmarshal(w.Body.Bytes(), &resp)
		if !strings.HasPrefix(resp["ref"], "VK:TEMP:") {
			t.Fatalf("keycenter must never store LOCAL via encrypt, got %s", resp["ref"])
		}
	}
}
