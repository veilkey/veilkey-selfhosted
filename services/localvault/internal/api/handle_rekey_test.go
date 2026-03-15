package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"veilkey-localvault/internal/crypto"
	"veilkey-localvault/internal/db"
)

func TestHandleRekeyMixedSecretsSkipsAlreadyCurrentSecrets(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	oldDEK, err := server.getLocalDEK()
	if err != nil {
		t.Fatalf("getLocalDEK: %v", err)
	}
	newDEK := []byte("fedcba9876543210fedcba9876543210")

	currentCiphertext, currentNonce, err := crypto.Encrypt(newDEK, []byte("current-value"))
	if err != nil {
		t.Fatalf("Encrypt current secret: %v", err)
	}
	if err := server.db.SaveSecret(&db.Secret{
		ID:         crypto.GenerateUUID(),
		Name:       "CURRENT_SECRET",
		Ref:        "cafebabe",
		Ciphertext: currentCiphertext,
		Nonce:      currentNonce,
		Version:    104,
	}); err != nil {
		t.Fatalf("SaveSecret current: %v", err)
	}

	body, err := json.Marshal(map[string]interface{}{
		"dek":     newDEK,
		"version": 104,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/rekey", bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Status         string `json:"status"`
		SecretsUpdated int    `json:"secrets_updated"`
		SecretsSkipped int    `json:"secrets_skipped"`
		Version        int    `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "rekeyed" || resp.Version != 104 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.SecretsUpdated != 1 || resp.SecretsSkipped != 1 {
		t.Fatalf("unexpected counters: %+v", resp)
	}

	oldSecret, err := server.db.GetSecretByRef("deadbeef")
	if err != nil {
		t.Fatalf("GetSecretByRef old: %v", err)
	}
	if oldSecret.Version != 104 {
		t.Fatalf("old secret version = %d, want 104", oldSecret.Version)
	}
	if _, err := crypto.Decrypt(newDEK, oldSecret.Ciphertext, oldSecret.Nonce); err != nil {
		t.Fatalf("old secret should decrypt with new DEK: %v", err)
	}

	currentSecret, err := server.db.GetSecretByRef("cafebabe")
	if err != nil {
		t.Fatalf("GetSecretByRef current: %v", err)
	}
	if currentSecret.Version != 104 {
		t.Fatalf("current secret version = %d, want 104", currentSecret.Version)
	}
	if _, err := crypto.Decrypt(newDEK, currentSecret.Ciphertext, currentSecret.Nonce); err != nil {
		t.Fatalf("current secret should still decrypt with new DEK: %v", err)
	}
	if _, err := crypto.Decrypt(oldDEK, currentSecret.Ciphertext, currentSecret.Nonce); err == nil {
		t.Fatal("current secret should not decrypt with old DEK after rekey")
	}
}
