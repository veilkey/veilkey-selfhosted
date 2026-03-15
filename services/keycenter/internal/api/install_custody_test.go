package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"veilkey-keycenter/internal/db"
)

func TestInstallCustodyChallengeRoundTripOnLockedServer(t *testing.T) {
	secretInbox := filepath.Join(t.TempDir(), "mail.txt")
	sendmail := filepath.Join(t.TempDir(), "sendmail")
	script := "#!/bin/sh\ncat >" + secretInbox + "\n"
	if err := os.WriteFile(sendmail, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake sendmail: %v", err)
	}
	t.Setenv("VEILKEY_OTP_SENDMAIL", sendmail)
	t.Setenv("VEILKEY_OTP_SMTP_HOST", "")

	srv, handler, _ := setupServerWithPassword(t, "install-pass")

	createSession := postJSON(handler, "/api/install/session", map[string]any{
		"language":         "ko",
		"quickstart":       true,
		"flow":             "wizard",
		"deployment_mode":  "docker",
		"install_scope":    "host+keycenter",
		"bootstrap_mode":   "email",
		"mail_transport":   "sendmail-fallback",
		"planned_stages":   []string{"language", "bootstrap", "custody"},
		"completed_stages": []string{"language", "bootstrap"},
		"last_stage":       "bootstrap",
	})
	if createSession.Code != 201 {
		t.Fatalf("create install session: expected 201, got %d: %s", createSession.Code, createSession.Body.String())
	}
	var sessionResp struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(createSession.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if sessionResp.Session.SessionID == "" {
		t.Fatal("expected session_id")
	}

	request := postJSON(handler, "/api/install/custody/request", map[string]any{
		"session_id":  sessionResp.Session.SessionID,
		"email":       "tex02@naver.com",
		"secret_name": "INSTALL_PASSWORD__TEST",
		"base_url":    "https://veilkey.test.internal",
	})
	if request.Code != 201 {
		t.Fatalf("create custody challenge: expected 201, got %d: %s", request.Code, request.Body.String())
	}
	var requestResp map[string]any
	if err := json.Unmarshal(request.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode custody request response: %v", err)
	}
	token, _ := requestResp["token"].(string)
	link, _ := requestResp["link"].(string)
	if token == "" || !strings.Contains(link, token) {
		t.Fatalf("expected tokenized link, got token=%q link=%q", token, link)
	}

	challenge, err := srv.db.GetInstallCustodyChallenge(token)
	if err != nil {
		t.Fatalf("load custody challenge: %v", err)
	}
	if challenge.Status != "pending" {
		t.Fatalf("expected pending challenge, got %q", challenge.Status)
	}

	mailBody, err := os.ReadFile(secretInbox)
	if err != nil {
		t.Fatalf("read fake sendmail inbox: %v", err)
	}
	if !strings.Contains(string(mailBody), link) {
		t.Fatalf("expected custody link in mail body, got %q", string(mailBody))
	}

	page := getJSON(handler, "/approve/install/custody?token="+token)
	if page.Code != 200 {
		t.Fatalf("custody page: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "INSTALL_PASSWORD__TEST") {
		t.Fatalf("expected secret name on page, got %q", page.Body.String())
	}
	if !strings.Contains(page.Body.String(), `action="/approve/install/custody"`) {
		t.Fatalf("expected canonical approval form action, got %q", page.Body.String())
	}

	tokenRoute := getJSON(handler, "/approve/t/"+token)
	if tokenRoute.Code != 200 {
		t.Fatalf("token approval page: expected 200, got %d: %s", tokenRoute.Code, tokenRoute.Body.String())
	}
	if !strings.Contains(tokenRoute.Body.String(), "INSTALL_PASSWORD__TEST") {
		t.Fatalf("expected install custody token route to render challenge page, got %q", tokenRoute.Body.String())
	}

	submit := postForm(handler, "/approve/install/custody", map[string]string{
		"token": token,
		"value": "super-secret-password",
	})
	if submit.Code != 200 {
		t.Fatalf("custody submit: expected 200, got %d: %s", submit.Code, submit.Body.String())
	}

	completed, err := srv.db.GetInstallCustodyChallenge(token)
	if err != nil {
		t.Fatalf("reload custody challenge: %v", err)
	}
	if completed.Status != "submitted" {
		t.Fatalf("expected submitted challenge, got %q", completed.Status)
	}
	if len(completed.Ciphertext) == 0 || len(completed.Nonce) == 0 {
		t.Fatal("expected protected custody payload")
	}

	session, err := srv.db.GetInstallSession(sessionResp.Session.SessionID)
	if err != nil {
		t.Fatalf("reload install session: %v", err)
	}
	completedStages := decodeStringList(session.CompletedStagesJSON)
	found := false
	for _, stage := range completedStages {
		if stage == "custody" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custody stage in completed stages, got %v", completedStages)
	}
	if session.LastStage != "custody" {
		t.Fatalf("expected last_stage custody, got %q", session.LastStage)
	}
}

func TestInstallCustodyChallengeAllowsMissingEmail(t *testing.T) {
	srv, handler, _ := setupServerWithPassword(t, "install-pass")

	createSession := postJSON(handler, "/api/install/session", map[string]any{
		"language":         "ko",
		"quickstart":       true,
		"flow":             "wizard",
		"deployment_mode":  "docker",
		"install_scope":    "host+keycenter",
		"bootstrap_mode":   "email",
		"planned_stages":   []string{"language", "bootstrap", "custody"},
		"completed_stages": []string{"language", "bootstrap"},
		"last_stage":       "bootstrap",
	})
	if createSession.Code != 201 {
		t.Fatalf("create install session: expected 201, got %d: %s", createSession.Code, createSession.Body.String())
	}
	var sessionResp struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(createSession.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}

	request := postJSON(handler, "/api/install/custody/request", map[string]any{
		"session_id":  sessionResp.Session.SessionID,
		"secret_name": "INSTALL_PASSWORD__TEST",
		"base_url":    "https://veilkey.test.internal",
	})
	if request.Code != 201 {
		t.Fatalf("create custody challenge without email: expected 201, got %d: %s", request.Code, request.Body.String())
	}
	var requestResp map[string]any
	if err := json.Unmarshal(request.Body.Bytes(), &requestResp); err != nil {
		t.Fatalf("decode custody request response: %v", err)
	}
	token, _ := requestResp["token"].(string)
	if token == "" {
		t.Fatal("expected token")
	}

	challenge, err := srv.db.GetInstallCustodyChallenge(token)
	if err != nil {
		t.Fatalf("load custody challenge: %v", err)
	}
	if challenge.Email != "" {
		t.Fatalf("expected empty email, got %q", challenge.Email)
	}

	page := getJSON(handler, "/approve/install/custody?token="+token)
	if page.Code != 200 {
		t.Fatalf("custody page: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "Recipient: <code>-</code>") {
		t.Fatalf("expected placeholder email on page, got %q", page.Body.String())
	}
}

func TestLegacyInstallCustodyRouteStillWorksDuringTransition(t *testing.T) {
	srv, handler, _ := setupServerWithPassword(t, "install-pass")
	session := &db.InstallSession{
		SessionID:           "install-legacy-custody",
		Language:            "ko",
		Flow:                "wizard",
		DeploymentMode:      "docker",
		InstallScope:        "host+keycenter",
		BootstrapMode:       "email",
		MailTransport:       "sendmail-fallback",
		PlannedStagesJSON:   `["language","bootstrap","custody"]`,
		CompletedStagesJSON: `["language","bootstrap"]`,
		LastStage:           "bootstrap",
	}
	if err := srv.db.SaveInstallSession(session); err != nil {
		t.Fatalf("save install session: %v", err)
	}
	challenge := &db.InstallCustodyChallenge{
		Token:      "legacy-token",
		SessionID:  session.SessionID,
		Email:      "tex02@naver.com",
		SecretName: "INSTALL_PASSWORD__TEST",
		Status:     "pending",
	}
	if err := srv.db.SaveInstallCustodyChallenge(challenge); err != nil {
		t.Fatalf("save custody challenge: %v", err)
	}

	page := getJSON(handler, "/ui/install/custody?token=legacy-token")
	if page.Code != 200 {
		t.Fatalf("legacy custody page: expected 200, got %d: %s", page.Code, page.Body.String())
	}

	submit := postForm(handler, "/ui/install/custody", map[string]string{
		"token": "legacy-token",
		"value": "legacy-secret-password",
	})
	if submit.Code != 200 {
		t.Fatalf("legacy custody submit: expected 200, got %d: %s", submit.Code, submit.Body.String())
	}
}

func TestInstallCustodyRequestRespectsTrustedIP(t *testing.T) {
	srv, handler := setupTrustedIPServer(t, []string{"10.0.0.100"})
	_ = srv

	createSession := postJSONFromIP(handler, "/api/install/session", "10.0.0.100:1234", map[string]any{
		"language":        "ko",
		"flow":            "wizard",
		"deployment_mode": "docker",
	})
	if createSession.Code != 201 {
		t.Fatalf("create install session: expected 201, got %d: %s", createSession.Code, createSession.Body.String())
	}
	var sessionResp struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(createSession.Body.Bytes(), &sessionResp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}

	blocked := postJSONFromIP(handler, "/api/install/custody/request", "192.168.1.50:9999", map[string]any{
		"session_id":  sessionResp.Session.SessionID,
		"email":       "tex02@naver.com",
		"secret_name": "INSTALL_PASSWORD__TEST",
	})
	if blocked.Code != 403 {
		t.Fatalf("expected 403 for blocked IP, got %d: %s", blocked.Code, blocked.Body.String())
	}

	sendmail := filepath.Join(t.TempDir(), "sendmail")
	if err := os.WriteFile(sendmail, []byte("#!/bin/sh\ncat >/dev/null\n"), 0o755); err != nil {
		t.Fatalf("write fake sendmail: %v", err)
	}
	t.Setenv("VEILKEY_OTP_SENDMAIL", sendmail)
	t.Setenv("VEILKEY_OTP_SMTP_HOST", "")

	allowed := postJSONFromIP(handler, "/api/install/custody/request", "10.0.0.100:9999", map[string]any{
		"session_id":  sessionResp.Session.SessionID,
		"email":       "tex02@naver.com",
		"secret_name": "INSTALL_PASSWORD__TEST",
		"base_url":    "https://veilkey.test.internal",
	})
	if allowed.Code != 201 {
		t.Fatalf("expected 201 for allowed IP, got %d: %s", allowed.Code, allowed.Body.String())
	}
}

func TestApprovalTokenRouteShowsPlaceholderForUnknownToken(t *testing.T) {
	_, handler := setupTestServer(t)

	page := getJSON(handler, "/approve/t/not-migrated-yet")
	if page.Code != 200 {
		t.Fatalf("approval placeholder: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "did not match a migrated approval challenge yet") {
		t.Fatalf("expected placeholder copy, got %q", page.Body.String())
	}
}

func TestSecureInputApprovalChallengeRoundTrip(t *testing.T) {
	srv, handler := setupTestServer(t)
	cookie := enrollAndLoginAdmin(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/admin/approval-challenges/secure-input", strings.NewReader(`{
		"title":"Production Mail OTP",
		"prompt":"Provide the mailbox OTP that was just delivered.",
		"input_label":"Mailbox OTP",
		"submit_label":"Store OTP",
		"target_name":"mail-otp"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createW := httptest.NewRecorder()
	handler.ServeHTTP(createW, createReq)
	if createW.Code != http.StatusCreated {
		t.Fatalf("create secure input challenge: expected 201, got %d: %s", createW.Code, createW.Body.String())
	}

	var createResp struct {
		Token string `json:"token"`
		Kind  string `json:"kind"`
		Link  string `json:"link"`
	}
	if err := json.Unmarshal(createW.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode challenge create response: %v", err)
	}
	if createResp.Token == "" || createResp.Kind != "secure_input" || !strings.Contains(createResp.Link, "/approve/t/"+createResp.Token) {
		t.Fatalf("unexpected create response: %+v", createResp)
	}

	page := getJSON(handler, "/approve/t/"+createResp.Token)
	if page.Code != http.StatusOK {
		t.Fatalf("approval token page: expected 200, got %d: %s", page.Code, page.Body.String())
	}
	if !strings.Contains(page.Body.String(), "Production Mail OTP") || !strings.Contains(page.Body.String(), "Mailbox OTP") {
		t.Fatalf("expected secure input page, got %q", page.Body.String())
	}

	submit := postForm(handler, "/approve/t/"+createResp.Token, map[string]string{
		"token": createResp.Token,
		"value": "502991",
	})
	if submit.Code != http.StatusOK {
		t.Fatalf("approval token submit: expected 200, got %d: %s", submit.Code, submit.Body.String())
	}

	challenge, err := srv.db.GetApprovalTokenChallenge(createResp.Token)
	if err != nil {
		t.Fatalf("reload approval token challenge: %v", err)
	}
	if challenge.Status != "submitted" || len(challenge.Ciphertext) == 0 || len(challenge.Nonce) == 0 {
		t.Fatalf("expected submitted protected challenge, got %+v", challenge)
	}

	rows, err := srv.db.ListAuditEvents("approval_token", createResp.Token)
	if err != nil {
		t.Fatalf("list approval token audit: %v", err)
	}
	if len(rows) == 0 || rows[0].ApprovalChallengeID != createResp.Token || rows[0].Action != "submit" {
		t.Fatalf("expected approval token submit audit, got %+v", rows)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/approval-challenges?target_name=mail-otp&kind=secure_input&limit=4", nil)
	listReq.AddCookie(cookie)
	listW := httptest.NewRecorder()
	handler.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list approval challenges: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var listResp struct {
		Count      int `json:"count"`
		Challenges []struct {
			Token      string `json:"token"`
			Status     string `json:"status"`
			TargetName string `json:"target_name"`
			UsedAt     any    `json:"used_at"`
			Link       string `json:"link"`
		} `json:"challenges"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("decode approval challenge list: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Challenges) != 1 {
		t.Fatalf("expected one approval challenge in list, got %+v", listResp)
	}
	if listResp.Challenges[0].Token != createResp.Token || listResp.Challenges[0].Status != "submitted" || listResp.Challenges[0].TargetName != "mail-otp" || listResp.Challenges[0].UsedAt == nil || !strings.Contains(listResp.Challenges[0].Link, "/approve/t/"+createResp.Token) {
		t.Fatalf("unexpected listed approval challenge: %+v", listResp.Challenges[0])
	}
}
