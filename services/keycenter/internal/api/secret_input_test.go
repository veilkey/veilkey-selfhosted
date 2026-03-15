package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecretInputChallengeRoundTrip(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents/veilkey-hostvault/secrets" {
			http.NotFound(w, r)
			return
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["name"] != "INSTALL_PASSWORD__TEST" || payload["value"] != "super-secret-password" {
			t.Fatalf("unexpected payload: %+v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer agent.Close()

	srv, handler, _ := setupServerWithPassword(t, "install-pass")

	request := postJSON(handler, "/api/approvals/secret-input/request", map[string]any{
		"email":       "tex02@naver.com",
		"endpoint":    agent.URL + "/api/agents/veilkey-hostvault",
		"vault":       "hostvault",
		"secret_name": "INSTALL_PASSWORD__TEST",
		"reason":      "install password custody",
		"base_url":    "https://keycenter.test",
	})
	if request.Code != 201 {
		t.Fatalf("create secret input challenge: expected 201, got %d: %s", request.Code, request.Body.String())
	}
	var requestResp map[string]any
	if err := json.Unmarshal(request.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode secret input request response: %v", err)
	}
	token, _ := requestResp["token"].(string)
	link, _ := requestResp["link"].(string)
	if token == "" || !strings.Contains(link, token) {
		t.Fatalf("expected tokenized link, got token=%q link=%q", token, link)
	}

	page := getJSON(handler, "/ui/approvals/secret-input?token="+token)
	if page.Code != 200 {
		t.Fatalf("secret input page: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	for _, needle := range []string{"Secure Secret Input", "INSTALL_PASSWORD__TEST", "hostvault"} {
		if !strings.Contains(page.Body.String(), needle) {
			t.Fatalf("expected secret input page to contain %q", needle)
		}
	}

	submit := postForm(handler, "/ui/approvals/secret-input", map[string]string{
		"token":   token,
		"value":   "super-secret-password",
		"confirm": "super-secret-password",
	})
	if submit.Code != 200 {
		t.Fatalf("secret input submit: expected 200, got %d: %s", submit.Code, submit.Body.String())
	}

	challenge, err := srv.db.GetSecretInputChallenge(token)
	if err != nil {
		t.Fatalf("reload secret input challenge: %v", err)
	}
	if challenge.Status != "submitted" {
		t.Fatalf("expected submitted challenge, got %q", challenge.Status)
	}
	if challenge.UsedAt == nil {
		t.Fatal("expected used_at to be set")
	}
}

func TestSecretInputChallengeAllowsMissingEmail(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agents/veilkey-hostvault/secrets" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer agent.Close()

	srv, handler, _ := setupServerWithPassword(t, "install-pass")

	request := postJSON(handler, "/api/approvals/secret-input/request", map[string]any{
		"endpoint":    agent.URL + "/api/agents/veilkey-hostvault",
		"vault":       "hostvault",
		"secret_name": "INSTALL_PASSWORD__TEST",
		"reason":      "install password custody",
		"base_url":    "https://keycenter.test",
	})
	if request.Code != 201 {
		t.Fatalf("create secret input challenge without email: expected 201, got %d: %s", request.Code, request.Body.String())
	}
	var requestResp map[string]any
	if err := json.Unmarshal(request.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode secret input request response: %v", err)
	}
	token, _ := requestResp["token"].(string)
	if token == "" {
		t.Fatal("expected token")
	}

	challenge, err := srv.db.GetSecretInputChallenge(token)
	if err != nil {
		t.Fatalf("reload secret input challenge: %v", err)
	}
	if challenge.Email != "" {
		t.Fatalf("expected empty email, got %q", challenge.Email)
	}

	page := getJSON(handler, "/ui/approvals/secret-input?token="+token)
	if page.Code != 200 {
		t.Fatalf("secret input page: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "Target email: <strong>-</strong>") {
		t.Fatalf("expected placeholder email on page, got %q", page.Body.String())
	}

	_ = srv
}

func TestSecretInputRequestRespectsTrustedIP(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.0.0.100"})
	_ = srv

	blocked := postJSONFromIP(handler, "/api/approvals/secret-input/request", "192.168.1.50:9999", map[string]any{
		"email":       "tex02@naver.com",
		"endpoint":    "http://127.0.0.1:10180/api/agents/veilkey-hostvault",
		"vault":       "hostvault",
		"secret_name": "INSTALL_PASSWORD__TEST",
	})
	if blocked.Code != 403 {
		t.Fatalf("expected 403 for blocked IP, got %d: %s", blocked.Code, blocked.Body.String())
	}

	allowed := postJSONFromIP(handler, "/api/approvals/secret-input/request", "10.0.0.100:9999", map[string]any{
		"email":       "tex02@naver.com",
		"endpoint":    "http://127.0.0.1:10180/api/agents/veilkey-hostvault",
		"vault":       "hostvault",
		"secret_name": "INSTALL_PASSWORD__TEST",
		"base_url":    "https://keycenter.test",
	})
	if allowed.Code != 201 {
		t.Fatalf("expected 201 for allowed IP, got %d: %s", allowed.Code, allowed.Body.String())
	}
}
