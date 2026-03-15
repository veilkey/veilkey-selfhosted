package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallBootstrapChallengeRoundTrip(t *testing.T) {
	secretInbox := filepath.Join(t.TempDir(), "mail.txt")
	sendmail := filepath.Join(t.TempDir(), "sendmail")
	script := "#!/bin/sh\ncat >" + secretInbox + "\n"
	if err := os.WriteFile(sendmail, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sendmail: %v", err)
	}
	t.Setenv("VEILKEY_OTP_SENDMAIL", sendmail)
	t.Setenv("VEILKEY_OTP_SMTP_HOST", "")

	_, handler, _ := setupServerWithPassword(t, "install-pass")

	createSession := postJSON(handler, "/api/install/session", map[string]any{
		"language":         "ko",
		"quickstart":       true,
		"flow":             "wizard",
		"deployment_mode":  "docker",
		"install_scope":    "host+keycenter",
		"bootstrap_mode":   "email",
		"mail_transport":   "sendmail-fallback",
		"planned_stages":   []string{"language", "bootstrap", "custody"},
		"completed_stages": []string{"language"},
		"last_stage":       "language",
	})
	if createSession.Code != http.StatusCreated {
		t.Fatalf("create install session: expected 201, got %d: %s", createSession.Code, createSession.Body.String())
	}
	var sessionResp struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(createSession.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode install session response: %v", err)
	}

	request := postJSON(handler, "/api/install/bootstrap/request", map[string]any{
		"session_id": sessionResp.Session.SessionID,
		"email":      "tex02@naver.com",
		"base_url":   "https://veilkey.test.internal",
	})
	if request.Code != http.StatusCreated {
		t.Fatalf("create bootstrap request: expected 201, got %d: %s", request.Code, request.Body.String())
	}
	var requestResp struct {
		Token string `json:"token"`
		Link  string `json:"link"`
	}
	if err := json.Unmarshal(request.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode bootstrap request response: %v", err)
	}
	if requestResp.Token == "" || requestResp.Link != "https://veilkey.test.internal/approve/t/"+requestResp.Token {
		t.Fatalf("unexpected bootstrap request response: %+v", requestResp)
	}
	if raw, err := os.ReadFile(secretInbox); err != nil || !strings.Contains(string(raw), requestResp.Link) {
		t.Fatalf("expected mail body to contain bootstrap link, err=%v body=%q", err, string(raw))
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/approve/install/bootstrap?session_id="+sessionResp.Session.SessionID, nil)
	redirectW := httptest.NewRecorder()
	handler.ServeHTTP(redirectW, redirectReq)
	if redirectW.Code != http.StatusSeeOther {
		t.Fatalf("bootstrap redirect: expected 303, got %d: %s", redirectW.Code, redirectW.Body.String())
	}
	if location := redirectW.Header().Get("Location"); location != "/approve/t/"+requestResp.Token {
		t.Fatalf("bootstrap redirect location = %q", location)
	}

	page := getJSON(handler, "/approve/t/"+requestResp.Token)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Install Bootstrap Confirmation") {
		t.Fatalf("expected bootstrap token page, got %d: %s", page.Code, page.Body.String())
	}

	submit := postForm(handler, "/approve/t/"+requestResp.Token, map[string]string{
		"token": requestResp.Token,
		"value": "otp-confirmed",
	})
	if submit.Code != http.StatusOK {
		t.Fatalf("bootstrap submit: expected 200, got %d: %s", submit.Code, submit.Body.String())
	}

	getLatest := getJSON(handler, "/api/install/state?session_id="+sessionResp.Session.SessionID)
	var latestResp struct {
		Exists  bool                `json:"exists"`
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(getLatest.Body.Bytes(), &latestResp); err != nil {
		t.Fatalf("decode latest install state: %v", err)
	}
	if !latestResp.Exists || latestResp.Session.LastStage != "bootstrap" || len(latestResp.Session.CompletedStages) != 2 {
		t.Fatalf("unexpected install state after bootstrap submit: %+v", latestResp)
	}
}
