package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

func saveInstallSessionForTest(t *testing.T, srv *Server, planned, completed []string, lastStage string) {
	t.Helper()
	if err := srv.db.SaveInstallSession(&db.InstallSession{
		SessionID:           crypto.GenerateUUID(),
		Flow:                "wizard",
		PlannedStagesJSON:   encodeStringList(planned),
		CompletedStagesJSON: encodeStringList(completed),
		LastStage:           lastStage,
	}); err != nil {
		t.Fatalf("SaveInstallSession: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers specific to integration tests
// ---------------------------------------------------------------------------

// setupServerWithPassword creates a server that is initially locked and
// requires unlock via password (uses real PBKDF2 KEK derivation).
func setupServerWithPassword(t *testing.T, password string) (*Server, http.Handler, []byte) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("setupServerWithPassword: db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	salt, err := crypto.GenerateSalt()
	if err != nil {
		t.Fatalf("setupServerWithPassword: GenerateSalt: %v", err)
	}
	kek := crypto.DeriveKEK(password, salt)

	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("setupServerWithPassword: GenerateKey (DEK): %v", err)
	}
	encDEK, nonce, err := crypto.EncryptDEK(kek, dek)
	if err != nil {
		t.Fatalf("setupServerWithPassword: EncryptDEK: %v", err)
	}
	if err = database.SaveNodeInfo(&db.NodeInfo{
		NodeID:   crypto.GenerateUUID(),
		DEK:      encDEK,
		DEKNonce: nonce,
		Version:  1,
	}); err != nil {
		t.Fatalf("setupServerWithPassword: SaveNodeInfo: %v", err)
	}

	srv := NewServer(database, nil, []string{})
	srv.SetSalt(salt)
	handler := srv.SetupRoutes()

	return srv, handler, salt
}

// setupTrustedIPServer creates an unlocked server that restricts to specific IPs.
func setupTrustedIPServer(t *testing.T, trustedIPs []string) (*Server, http.Handler) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("setupTrustedIPServer: db.New: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	kek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("setupTrustedIPServer: GenerateKey (KEK): %v", err)
	}
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("setupTrustedIPServer: GenerateKey (DEK): %v", err)
	}
	encDEK, nonce, err := crypto.EncryptDEK(kek, dek)
	if err != nil {
		t.Fatalf("setupTrustedIPServer: EncryptDEK: %v", err)
	}
	if err = database.SaveNodeInfo(&db.NodeInfo{
		NodeID:   crypto.GenerateUUID(),
		DEK:      encDEK,
		DEKNonce: nonce,
		Version:  1,
	}); err != nil {
		t.Fatalf("setupTrustedIPServer: SaveNodeInfo: %v", err)
	}

	srv := NewServer(database, kek, trustedIPs)
	handler := srv.SetupRoutes()
	return srv, handler
}

// postJSONFromIP sends a POST with the given RemoteAddr.
func postJSONFromIP(handler http.Handler, path, remoteAddr string, body interface{}) *httptest.ResponseRecorder {
	b, err := json.Marshal(body)
	if err != nil {
		panic("postJSONFromIP: json.Marshal: " + err.Error())
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// getJSONFromIP sends a GET with the given RemoteAddr.
func getJSONFromIP(handler http.Handler, path, remoteAddr string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// Integration Test 1 – Unlock lifecycle
// ---------------------------------------------------------------------------

func TestIntegration_UnlockLifecycle(t *testing.T) {
	const password = "correct-horse-battery-staple"

	_, handler, _ := setupServerWithPassword(t, password)

	// Server must start locked
	var healthResp map[string]string
	if err := json.Unmarshal(getJSON(handler, "/health").Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if healthResp["status"] != "locked" {
		t.Errorf("health before unlock: expected status=locked, got %q", healthResp["status"])
	}

	// Status before unlock must be rejected
	w := getJSON(handler, "/api/status")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status before unlock: expected 503, got %d", w.Code)
	}

	// Unlock with correct password
	w = postJSON(handler, "/api/unlock", map[string]string{"password": password})
	if w.Code != http.StatusOK {
		t.Fatalf("unlock: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var unlockResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &unlockResp); err != nil {
		t.Fatalf("unmarshal unlock: %v", err)
	}
	if unlockResp["status"] != "unlocked" {
		t.Errorf("unlock response status = %v, want unlocked", unlockResp["status"])
	}

	// Health must now report ok
	if err := json.Unmarshal(getJSON(handler, "/health").Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("unmarshal health after unlock: %v", err)
	}
	if healthResp["status"] != "ok" {
		t.Errorf("health after unlock: expected status=ok, got %q", healthResp["status"])
	}

	// Second unlock call on already-unlocked server must be idempotent
	w = postJSON(handler, "/api/unlock", map[string]string{"password": password})
	if w.Code != http.StatusOK {
		t.Fatalf("second unlock: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var secondUnlock map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &secondUnlock); err != nil {
		t.Fatalf("unmarshal second unlock: %v", err)
	}
	if secondUnlock["status"] != "already_unlocked" {
		t.Errorf("second unlock status = %v, want already_unlocked", secondUnlock["status"])
	}
}

// ---------------------------------------------------------------------------
// Integration Test 2 – Trusted IP middleware
// ---------------------------------------------------------------------------

func TestIntegration_TrustedIP(t *testing.T) {
	const allowedIP = "10.0.0.1"
	const blockedIP = "192.168.1.99"
	const allowedCIDRIP = "172.16.0.5"

	_, handler := setupTrustedIPServer(t, []string{allowedIP, "172.16.0.0/24"})

	t.Run("allowed exact IP can unlock", func(t *testing.T) {
		w := postJSONFromIP(handler, "/api/unlock", allowedIP+":12345", map[string]string{"password": "any"})
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("allowed CIDR IP can unlock", func(t *testing.T) {
		w := postJSONFromIP(handler, "/api/unlock", allowedCIDRIP+":9999", map[string]string{"password": "any"})
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("blocked IP gets 403 on unlock", func(t *testing.T) {
		w := postJSONFromIP(handler, "/api/unlock", blockedIP+":12345", map[string]string{"password": "any"})
		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("health endpoint is not IP-restricted", func(t *testing.T) {
		w := getJSONFromIP(handler, "/health", blockedIP+":12345")
		if w.Code != http.StatusOK {
			t.Errorf("health from blocked IP: expected 200, got %d", w.Code)
		}
	})

	t.Run("unlock endpoint is IP-restricted", func(t *testing.T) {
		w := postJSONFromIP(handler, "/api/unlock", blockedIP+":12345", map[string]string{"password": "any"})
		if w.Code != http.StatusForbidden {
			t.Errorf("unlock from blocked IP: expected 403, got %d", w.Code)
		}
	})
}

func TestIntegration_TrustedIP_EmptyList(t *testing.T) {
	_, handler := setupTestServer(t)

	// Any IP should be allowed for status (no trust list = open mode)
	for _, ip := range []string{"1.2.3.4", "192.168.99.1", "10.0.0.1"} {
		w := getJSONFromIP(handler, "/api/status", ip+":1234")
		if w.Code != http.StatusOK {
			t.Errorf("IP %s: expected 200, got %d", ip, w.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Test 3 – Error cases
// ---------------------------------------------------------------------------

func TestIntegration_UnlockWrongPassword(t *testing.T) {
	const correctPassword = "the-right-password"
	_, handler, _ := setupServerWithPassword(t, correctPassword)

	w := postJSON(handler, "/api/unlock", map[string]string{"password": "wrong-password-1"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("first wrong attempt: expected 401, got %d: %s", w.Code, w.Body.String())
	}

	var healthResp map[string]string
	if err := json.Unmarshal(getJSON(handler, "/health").Body.Bytes(), &healthResp); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if healthResp["status"] != "locked" {
		t.Errorf("after wrong password: health status = %q, want locked", healthResp["status"])
	}

	w = postJSON(handler, "/api/unlock", map[string]string{"password": correctPassword})
	if w.Code != http.StatusOK {
		t.Errorf("correct password: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntegration_UnlockMissingPassword(t *testing.T) {
	_, handler, _ := setupServerWithPassword(t, "some-password")

	w := postJSON(handler, "/api/unlock", map[string]string{"password": ""})
	if w.Code != http.StatusBadRequest {
		t.Errorf("empty password: expected 400, got %d: %s", w.Code, w.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/api/unlock", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	if rw.Code != http.StatusBadRequest {
		t.Errorf("malformed body: expected 400, got %d", rw.Code)
	}
}

func TestIntegration_InstallGateBlocksOperatorAPIsUntilComplete(t *testing.T) {
	srv, handler := setupTestServer(t)
	saveInstallSessionForTest(t, srv, []string{"language", "bootstrap", "custody"}, []string{"language"}, "language")

	status := getJSON(handler, "/api/status")
	if status.Code != http.StatusOK {
		t.Fatalf("status should remain visible, got %d: %s", status.Code, status.Body.String())
	}
	var statusResp map[string]any
	if err := json.Unmarshal(status.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if statusResp["install_complete"] != false {
		t.Fatalf("install_complete = %v, want false", statusResp["install_complete"])
	}

	blocked := getJSON(handler, "/api/vault-inventory")
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", blocked.Code, blocked.Body.String())
	}
	if !bytes.Contains(blocked.Body.Bytes(), []byte("install flow is not complete")) {
		t.Fatalf("expected install gate message, got %s", blocked.Body.String())
	}

	ready := getJSON(handler, "/ready")
	if ready.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready should fail while install incomplete, got %d: %s", ready.Code, ready.Body.String())
	}
}

func TestIntegration_InstallGateAllowsOperatorAPIsAfterCompletion(t *testing.T) {
	srv, handler := setupTestServer(t)
	saveInstallSessionForTest(t, srv, []string{"language", "bootstrap", "custody"}, []string{"language", "bootstrap", "custody"}, "custody")

	status := getJSON(handler, "/api/status")
	if status.Code != http.StatusOK {
		t.Fatalf("status: expected 200, got %d: %s", status.Code, status.Body.String())
	}
	var statusResp map[string]any
	if err := json.Unmarshal(status.Body.Bytes(), &statusResp); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if statusResp["install_complete"] != true {
		t.Fatalf("install_complete = %v, want true", statusResp["install_complete"])
	}

	allowed := getJSON(handler, "/api/vault-inventory")
	if allowed.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", allowed.Code, allowed.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Integration Test 4 – Removed endpoints return 404
// ---------------------------------------------------------------------------

func TestIntegration_RemovedEndpoints(t *testing.T) {
	_, handler := setupTestServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/encrypt"},
		{"POST", "/api/decrypt"},
		{"POST", "/api/reencrypt"},
		{"POST", "/api/rotate"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if ep.method == "POST" {
				w = postJSON(handler, ep.path, map[string]string{"plaintext": "test"})
			} else {
				w = getJSON(handler, ep.path)
			}
			if w.Code != http.StatusNotFound {
				t.Errorf("%s %s: expected 404, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Integration Test 7 – Real HTTP server
// ---------------------------------------------------------------------------

func TestIntegration_RealHTTPServer(t *testing.T) {
	_, handler := setupTestServer(t)

	ts := httptest.NewServer(handler)
	defer ts.Close()

	client := ts.Client()

	doGet := func(path string) (*http.Response, []byte) {
		resp, err := client.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			t.Fatalf("ReadFrom %s: %v", path, err)
		}
		return resp, buf.Bytes()
	}

	// Health check
	resp, body := doGet("/health")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health: expected 200, got %d: %s", resp.StatusCode, body)
	}

	// Status
	resp, body = doGet("/api/status")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: expected 200, got %d: %s", resp.StatusCode, body)
	}

	// 404 for unknown path
	resp, _ = doGet("/unknown-path")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("unknown path: expected 404, got %d", resp.StatusCode)
	}

	// 404 for removed encrypt endpoint
	resp, _ = doGet("/api/encrypt")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("removed encrypt: expected 404, got %d", resp.StatusCode)
	}
}
