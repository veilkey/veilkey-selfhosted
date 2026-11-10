package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-localvault/internal/db"
)

func remediationServer(t *testing.T, registered bool) (*Server, string) {
	t.Helper()
	kek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("VEILKEY_DB_KEY", DeriveDBKeyFromKEK(kek))
	path := filepath.Join(t.TempDir(), "vault.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, nonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{NodeID: "test-node", DEK: encrypted, DEKNonce: nonce, Version: 1}); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(dek)
	if registered {
		encrypted, nonce, err = crypto.Encrypt(kek, []byte(token))
		if err != nil {
			t.Fatal(err)
		}
		if err := database.UpdateAgentSecret(encrypted, nonce); err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer(database, kek, []string{"192.0.2.1"})
	server.SetDBPath(path, nil)
	server.SetIdentity(&NodeIdentity{NodeID: "test-node"})
	return server, token
}

func remediationRequest(t *testing.T, handler http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestProtectedRoutesRequireAgentSecret(t *testing.T) {
	server, token := remediationServer(t, true)
	routes := []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/configs", map[string]string{"key": "APP_MODE", "value": "test"}},
		{"PUT", "/api/configs/bulk", map[string]any{"configs": map[string]string{"APP_MODE": "test"}}},
		{"DELETE", "/api/configs/APP_MODE", nil},
		{"GET", "/api/configs", nil},
		{"POST", "/api/bulk-apply/precheck", nil},
		{"POST", "/api/bulk-apply/execute", nil},
		{"GET", "/api/secrets", nil},
		{"GET", "/api/functions", nil},
		{"GET", "/api/functions/test", nil},
		{"POST", "/api/functions", nil},
		{"GET", "/api/node-info", nil},
		{"POST", "/api/lock", nil},
		{"GET", "/api/admin/diagnostics", nil},
		{"GET", "/api/admin/tls-info", nil},
		{"POST", "/api/activate", nil},
		{"PATCH", "/api/install/vaultcenter-url", map[string]string{"vaultcenter_url": "https://center.example.test"}},
	}
	handler := server.SetupRoutes()
	for _, route := range routes {
		t.Run(route.method+route.path, func(t *testing.T) {
			for _, auth := range []string{"", token + "invalid"} {
				rec := remediationRequest(t, handler, route.method, route.path, auth, route.body)
				if rec.Code != http.StatusUnauthorized {
					t.Errorf("status = %d, want 401", rec.Code)
				}
			}
		})
	}
	if server.IsLocked() {
		t.Error("unauthorized lock changed server state")
	}
	if _, err := server.db.GetConfig("APP_MODE"); err == nil {
		t.Error("unauthorized config write changed database")
	}
	for _, route := range []string{"/api/secrets", "/api/functions", "/api/node-info", "/api/admin/diagnostics"} {
		rec := remediationRequest(t, handler, "GET", route, token, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("authenticated %s = %d", route, rec.Code)
		}
	}
	rec := remediationRequest(t, handler, "POST", "/api/configs", token, map[string]string{"key": "APP_MODE", "value": "test"})
	if rec.Code != http.StatusOK {
		t.Errorf("authenticated config write = %d", rec.Code)
	}
}

func TestPublicStatusHasNoVaultMetadata(t *testing.T) {
	server, _ := remediationServer(t, true)
	rec := remediationRequest(t, server.SetupRoutes(), "GET", "/api/status", "", nil)
	var status map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK || len(status) != 1 || status["status"] != "ok" {
		t.Error("public status must return only the health state")
	}
}

func TestAgentSecretFailsClosedBeforeRegistration(t *testing.T) {
	server, _ := remediationServer(t, false)
	rec := remediationRequest(t, server.SetupRoutes(), "GET", "/api/cipher/test", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestConfigsRejectReservedKeys(t *testing.T) {
	server, token := remediationServer(t, true)
	t.Setenv("VEILKEY_VAULTCENTER_URL", "")
	const original = "https://center.example.test"
	if err := server.db.SaveConfig(db.ConfigKeyVaultcenterURL, original); err != nil {
		t.Fatal(err)
	}
	handler := server.SetupRoutes()
	for _, route := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/configs", map[string]string{"key": db.ConfigKeyVaultcenterURL, "value": "https://other.example.test"}},
		{"PUT", "/api/configs/bulk", map[string]any{"configs": map[string]string{db.ConfigKeyVaultcenterURL: "https://other.example.test", "APP_MODE": "test"}}},
		{"DELETE", "/api/configs/" + db.ConfigKeyVaultcenterURL, nil},
	} {
		rec := remediationRequest(t, handler, route.method, route.path, token, route.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", route.method, rec.Code)
		}
	}
	if server.VaultcenterURL() != original {
		t.Error("reserved target changed")
	}
	if _, err := server.db.GetConfig("APP_MODE"); err == nil {
		t.Error("invalid bulk request partially applied")
	}
}

func TestConfigsLockedReturnsUnavailable(t *testing.T) {
	server := NewServer(nil, nil, []string{"192.0.2.1"})
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/configs"}, {"POST", "/api/configs"}, {"PUT", "/api/configs/bulk"}, {"DELETE", "/api/configs/APP_MODE"},
	} {
		rec := remediationRequest(t, server.SetupRoutes(), route.method, route.path, "", nil)
		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("%s = %d, want 503", route.method, rec.Code)
		}
	}
}

func TestCipherAPIsRejectInvalidLengths(t *testing.T) {
	server, token := remediationServer(t, true)
	if err := server.db.SaveSecret(&db.Secret{ID: "test-id", Name: "APP_VALUE", Ref: "test-ref", Ciphertext: make([]byte, 16), Nonce: make([]byte, 12), Scope: db.RefScopeLocal, Status: db.RefStatusActive}); err != nil {
		t.Fatal(err)
	}
	handler := server.SetupRoutes()
	for _, sizes := range []struct{ cipher, nonce int }{{16, 1}, {16, 11}, {16, 13}, {15, 12}, {0, 12}} {
		payload := map[string]any{"name": "APP_VALUE", "ciphertext": make([]byte, sizes.cipher), "nonce": make([]byte, sizes.nonce)}
		rec := remediationRequest(t, handler, "POST", "/api/cipher", token, payload)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("cipher (%d,%d) = %d", sizes.cipher, sizes.nonce, rec.Code)
		}
		field := map[string]any{"key": "VALUE", "ciphertext": make([]byte, sizes.cipher), "nonce": make([]byte, sizes.nonce)}
		rec = remediationRequest(t, handler, "POST", "/api/secrets/fields", token, map[string]any{"name": "APP_VALUE", "fields": []any{field}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("field (%d,%d) = %d", sizes.cipher, sizes.nonce, rec.Code)
		}
	}
	dek, err := server.GetLocalDEK()
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, nonce, err := crypto.Encrypt(dek, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []struct {
		path string
		body any
	}{
		{"/api/cipher", map[string]any{"name": "APP_VALUE", "ciphertext": ciphertext, "nonce": nonce}},
		{"/api/secrets/fields", map[string]any{"name": "APP_VALUE", "fields": []any{map[string]any{"key": "VALUE", "ciphertext": ciphertext, "nonce": nonce}}}},
	} {
		rec := remediationRequest(t, handler, "POST", route.path, token, route.body)
		if rec.Code != http.StatusOK {
			t.Errorf("valid empty-plaintext cipher %s = %d", route.path, rec.Code)
		}
	}
}

func TestWrongKEKLeavesLegacyDatabaseUnchanged(t *testing.T) {
	t.Setenv("VEILKEY_DB_KEY", "")
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	kek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	encrypted, nonce, err := crypto.Encrypt(kek, dek)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveNodeInfo(&db.NodeInfo{NodeID: "legacy-node", DEK: encrypted, DEKNonce: nonce, Version: 1}); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(nil, nil, nil)
	server.SetDBPath(path, nil)
	t.Cleanup(server.Close)
	wrong, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Unlock(wrong); err == nil {
		t.Fatal("wrong KEK accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("wrong KEK changed plaintext database")
	}
	if err := server.Unlock(kek); err != nil {
		t.Fatalf("correct KEK after failed unlock: %v", err)
	}
}
