package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"veilkey-localvault/internal/crypto"
	"veilkey-localvault/internal/db"
)

func setupReencryptTestServer(t *testing.T) *Server {
	t.Helper()

	database, err := db.New(filepath.Join(t.TempDir(), "localvault.db"))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}

	kek := []byte("0123456789abcdef0123456789abcdef")
	dek := []byte("abcdef0123456789abcdef0123456789")
	encDEK, encNonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		t.Fatalf("Encrypt DEK: %v", err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{
		NodeID:   "test-node",
		DEK:      encDEK,
		DEKNonce: encNonce,
		Version:  1,
	}); err != nil {
		t.Fatalf("SaveNodeInfo: %v", err)
	}

	ciphertext, nonce, err := crypto.Encrypt(dek, []byte("secret-value"))
	if err != nil {
		t.Fatalf("Encrypt secret: %v", err)
	}
	if err := database.SaveSecret(&db.Secret{
		ID:         crypto.GenerateUUID(),
		Name:       "TEST_SECRET",
		Ref:        "deadbeef",
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Version:    1,
	}); err != nil {
		t.Fatalf("SaveSecret: %v", err)
	}

	return NewServer(database, kek, []string{"127.0.0.1"})
}

func TestParseScopedRefAcceptsCanonicalFamiliesAndScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw    string
		family RefFamily
		scope  RefScope
		id     string
	}{
		{raw: "VK:TEMP:deadbeef", family: RefFamilyVK, scope: RefScopeTemp, id: "deadbeef"},
		{raw: "VK:LOCAL:deadbeef", family: RefFamilyVK, scope: RefScopeLocal, id: "deadbeef"},
		{raw: "VE:EXTERNAL:config_01", family: RefFamilyVE, scope: RefScopeExternal, id: "config_01"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()
			got, err := ParseScopedRef(tt.raw)
			if err != nil {
				t.Fatalf("ParseScopedRef(%q): %v", tt.raw, err)
			}
			if got.Family != tt.family || got.Scope != tt.scope || got.ID != tt.id {
				t.Fatalf("parsed = %+v", got)
			}
			if got.CanonicalString() != tt.raw {
				t.Fatalf("canonical = %q", got.CanonicalString())
			}
		})
	}
}

func TestParseScopedRefRejectsNonCanonicalRefs(t *testing.T) {
	t.Parallel()

	tests := []string{
		"",
		"VK:TEMP:dead beef",
		"VE:TEMP:dead beef",
		"VK:TEMP:dead:beef",
		"VK:TEMP:",
		"VK:TEMP:dead beef",
		"BAD:TEMP:deadbeef",
	}

	for _, raw := range tests {
		raw := raw
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseScopedRef(raw); err == nil {
				t.Fatalf("ParseScopedRef(%q) should fail", raw)
			}
		})
	}
}

func TestHandleReencryptReturnsCanonicalScopedRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/reencrypt", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef"}`))
	w := httptest.NewRecorder()

	server.handleReencrypt(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VK:TEMP:deadbeef" {
		t.Fatalf("ciphertext = %q", resp.Ciphertext)
	}
	if resp.Changed {
		t.Fatal("changed should be false for canonical scoped ref")
	}
}

func TestHandleReencryptRejectsLegacyRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/reencrypt", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:dead beef"}`))
	w := httptest.NewRecorder()

	server.handleReencrypt(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReencryptRejectsVisibleFamily(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/reencrypt", bytes.NewBufferString(`{"ciphertext":"VE:TEMP:deadbeef"}`))
	w := httptest.NewRecorder()

	server.handleReencrypt(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleReencryptMissingRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/reencrypt", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:missing00"}`))
	w := httptest.NewRecorder()

	server.handleReencrypt(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandleActivateRouteActivatesTempRefIntoLocal(t *testing.T) {
	server := setupReencryptTestServer(t)
	var syncPayload struct {
		VaultNodeUUID string `json:"vault_node_uuid"`
		NodeID        string `json:"node_id"`
		Ref           string `json:"ref"`
		PreviousRef   string `json:"previous_ref"`
		Status        string `json:"status"`
	}
	vaultcenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tracked-refs/sync" {
			t.Fatalf("unexpected vaultcenter path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&syncPayload); err != nil {
			t.Fatalf("decode sync payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer vaultcenter.Close()
	t.Setenv("VEILKEY_VAULTCENTER_URL", vaultcenter.URL)
	server.SetIdentity(&NodeIdentity{NodeID: "test-node", VaultHash: "deadbeef", VaultName: "test-vault"})
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VK:LOCAL:deadbeef" || resp.Status != "active" || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	secret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secret.Status != "active" {
		t.Fatalf("secret status = %q", secret.Status)
	}
	if secret.Scope != "LOCAL" {
		t.Fatalf("secret scope = %q", secret.Scope)
	}
	if syncPayload.VaultNodeUUID != "test-node" {
		t.Fatalf("sync vault_node_uuid = %q, want test-node", syncPayload.VaultNodeUUID)
	}
	if syncPayload.NodeID != "test-node" {
		t.Fatalf("sync node_id = %q, want test-node", syncPayload.NodeID)
	}
	if syncPayload.Ref != "VK:LOCAL:deadbeef" {
		t.Fatalf("sync ref = %q", syncPayload.Ref)
	}
	if syncPayload.PreviousRef != "VK:TEMP:deadbeef" {
		t.Fatalf("sync previous_ref = %q", syncPayload.PreviousRef)
	}
	if syncPayload.Status != "active" {
		t.Fatalf("sync status = %q", syncPayload.Status)
	}
}

func TestHandleActivatePrefersEnvVaultcenterOverStaleDBConfig(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("VEILKEY_VAULTCENTER_URL", "http://stale.example:10180"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	var syncPath string
	vaultcenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syncPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer vaultcenter.Close()

	t.Setenv("VEILKEY_VAULTCENTER_URL", vaultcenter.URL)
	server.SetIdentity(&NodeIdentity{NodeID: "test-node", VaultHash: "deadbeef", VaultName: "test-vault"})
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if syncPath != "/api/tracked-refs/sync" {
		t.Fatalf("syncPath = %q", syncPath)
	}
}

func TestHandleActivateReturnsDegradedWhenTrackedRefSyncFails(t *testing.T) {
	server := setupReencryptTestServer(t)
	vaultcenter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "sync failed upstream", http.StatusBadGateway)
	}))
	defer vaultcenter.Close()

	t.Setenv("VEILKEY_VAULTCENTER_URL", vaultcenter.URL)
	server.SetIdentity(&NodeIdentity{NodeID: "test-node", VaultHash: "deadbeef", VaultName: "test-vault"})
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		SyncStatus string `json:"sync_status"`
		SyncTarget string `json:"sync_target"`
		SyncError  string `json:"sync_error"`
		Warning    string `json:"warning"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "active" || resp.SyncStatus != "degraded" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.SyncTarget != vaultcenter.URL {
		t.Fatalf("sync target = %q", resp.SyncTarget)
	}
	if resp.SyncError == "" || resp.Warning == "" {
		t.Fatalf("missing degraded diagnostics: %+v", resp)
	}
	secret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secret.Status != "active" || secret.Scope != "LOCAL" {
		t.Fatalf("secret lifecycle not updated: status=%q scope=%q", secret.Status, secret.Scope)
	}
}

func TestHandleActivateRouteActivatesTempRefIntoExternal(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef","scope":"EXTERNAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VK:EXTERNAL:deadbeef" || resp.Status != "active" || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestHandleGetConfigReturnsLockedForBlockedRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("BLOCKED_CFG", "value"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := server.db.UpdateConfigLifecycle("BLOCKED_CFG", "LOCAL", "block"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/configs/BLOCKED_CFG", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCipherReturnsLockedForBlockedRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.UpdateSecretLifecycle("deadbeef", "LOCAL", "block"); err != nil {
		t.Fatalf("UpdateSecretLifecycle: %v", err)
	}
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodGet, "/api/cipher/deadbeef", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleActivateRejectsNonTempRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VK:LOCAL:deadbeef","scope":"EXTERNAL"}`))
	w := httptest.NewRecorder()

	server.handleActivate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleActiveRouteIsRemoved(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/active", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef","scope":"LOCAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleArchiveReturnsArchiveStatus(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.UpdateSecretLifecycle("deadbeef", "LOCAL", "active"); err != nil {
		t.Fatalf("UpdateSecretLifecycle: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/archive", bytes.NewBufferString(`{"ciphertext":"VK:LOCAL:deadbeef"}`))
	w := httptest.NewRecorder()

	server.handleArchive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VK:LOCAL:deadbeef" || resp.Status != "archive" || resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	secret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secret.Status != "archive" {
		t.Fatalf("secret status = %q", secret.Status)
	}
	if secret.Scope != "LOCAL" {
		t.Fatalf("secret scope = %q", secret.Scope)
	}
}

func TestReencryptAllSecretsMarksRotationTimestamp(t *testing.T) {
	server := setupReencryptTestServer(t)

	dek, err := server.getLocalDEK()
	if err != nil {
		t.Fatalf("getLocalDEK: %v", err)
	}

	count, err := server.db.ReencryptAllSecrets(
		func(ciphertext, nonce []byte) ([]byte, error) {
			return crypto.Decrypt(dek, ciphertext, nonce)
		},
		func(plaintext []byte) ([]byte, []byte, error) {
			return crypto.Encrypt(dek, plaintext)
		},
		2,
	)
	if err != nil {
		t.Fatalf("ReencryptAllSecrets: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	secret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("GetSecretByRef: %v", err)
	}
	if !secret.LastRotatedAt.Valid {
		t.Fatal("expected last_rotated_at to be set after reencrypt")
	}
	if secret.Version != 2 {
		t.Fatalf("version = %d, want 2", secret.Version)
	}
}

func TestHandleRevokeRejectsTempRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/revoke", bytes.NewBufferString(`{"ciphertext":"VK:TEMP:deadbeef"}`))
	w := httptest.NewRecorder()

	server.handleRevoke(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleActivateRouteActivatesTempVERefIntoLocal(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("APP_URL", "https://example.test"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/activate", bytes.NewBufferString(`{"ciphertext":"VE:TEMP:APP_URL","scope":"LOCAL"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VE:LOCAL:APP_URL" || resp.Status != "active" || !resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}

	config, err := server.db.GetConfig("APP_URL")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Scope != "LOCAL" || config.Status != "active" {
		t.Fatalf("config lifecycle = %s/%s", config.Scope, config.Status)
	}
}

func TestHandleArchiveReturnsArchiveStatusForVERef(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("APP_ENV", "production"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := server.db.UpdateConfigLifecycle("APP_ENV", "EXTERNAL", "active"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/archive", bytes.NewBufferString(`{"ciphertext":"VE:EXTERNAL:APP_ENV"}`))
	w := httptest.NewRecorder()

	server.handleArchive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	config, err := server.db.GetConfig("APP_ENV")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if config.Scope != "EXTERNAL" || config.Status != "archive" {
		t.Fatalf("config lifecycle = %s/%s", config.Scope, config.Status)
	}
}

func TestHandleBlockReturnsBlockStatusForVKRef(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.UpdateSecretLifecycle("deadbeef", "LOCAL", "active"); err != nil {
		t.Fatalf("UpdateSecretLifecycle: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/block", bytes.NewBufferString(`{"ciphertext":"VK:LOCAL:deadbeef"}`))
	w := httptest.NewRecorder()

	server.handleBlock(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Ciphertext string `json:"ciphertext"`
		Status     string `json:"status"`
		Changed    bool   `json:"changed"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Ciphertext != "VK:LOCAL:deadbeef" || resp.Status != "block" || resp.Changed {
		t.Fatalf("unexpected response: %+v", resp)
	}
	secret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secret.Status != "block" {
		t.Fatalf("secret status = %q", secret.Status)
	}
}

func TestHandleCipherRejectsBlockedSecret(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.UpdateSecretLifecycle("deadbeef", "LOCAL", "block"); err != nil {
		t.Fatalf("UpdateSecretLifecycle: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/cipher/deadbeef", nil)
	req.SetPathValue("ref", "deadbeef")
	w := httptest.NewRecorder()

	server.handleCipher(w, req)

	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetConfigRejectsBlockedConfig(t *testing.T) {
	server := setupReencryptTestServer(t)
	if err := server.db.SaveConfig("APP_URL", "https://example.test"); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	if err := server.db.UpdateConfigLifecycle("APP_URL", "LOCAL", "block"); err != nil {
		t.Fatalf("UpdateConfigLifecycle: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/configs/APP_URL", nil)
	req.SetPathValue("key", "APP_URL")
	w := httptest.NewRecorder()

	server.handleGetConfig(w, req)

	if w.Code != http.StatusLocked {
		t.Fatalf("expected 423, got %d: %s", w.Code, w.Body.String())
	}
}
