package api

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"veilkey-localvault/internal/crypto"
)

func TestHandleDecryptReturnsPlaintextForTrustedCaller(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	dek, err := server.getLocalDEK()
	if err != nil {
		t.Fatalf("getLocalDEK: %v", err)
	}
	ciphertext, nonce, err := crypto.Encrypt(dek, []byte("legacy-secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	token := "VK:100:" + base64.StdEncoding.EncodeToString(append(append([]byte{}, nonce...), ciphertext...))

	req := httptest.NewRequest(http.MethodPost, "/api/decrypt", bytes.NewBufferString(`{"ciphertext":"`+token+`"}`))
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"plaintext":"legacy-secret"`)) {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestHandleDecryptRejectsUntrustedCaller(t *testing.T) {
	server := setupReencryptTestServer(t)
	handler := server.SetupRoutes()

	req := httptest.NewRequest(http.MethodPost, "/api/decrypt", bytes.NewBufferString(`{"ciphertext":"VK:100:Zm9v"}`))
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}
