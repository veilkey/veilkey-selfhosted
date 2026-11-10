package api

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	chain "github.com/veilkey/veilkey-chain"
	"github.com/veilkey/veilkey-go-package/crypto"
	"github.com/veilkey/veilkey-go-package/refs"
	"veilkey-vaultcenter/internal/db"
)

type tokenRefCaptureStore struct {
	chain.Store
	payload *chain.SaveTokenRefPayload
}

func (c *tokenRefCaptureStore) SaveRefWithExpiryAndHash(parts chain.RefParts, ciphertext string, version int, status refs.RefStatus, expiresAt time.Time, name, hash string) error {
	c.payload = &chain.SaveTokenRefPayload{RefFamily: parts.Family, RefScope: parts.Scope, RefID: parts.ID, SecretName: name, PlaintextHash: hash, Ciphertext: ciphertext, Version: version, Status: status, ExpiresAt: &expiresAt}
	return c.Store.SaveRefWithExpiryAndHash(parts, ciphertext, version, status, expiresAt, name, hash)
}

func TestTempEncryptionDoesNotReplicatePlaintextHash(t *testing.T) {
	for _, keycenter := range []bool{false, true} {
		t.Run(map[bool]string{false: "encrypt", true: "keycenter"}[keycenter], func(t *testing.T) {
			s, _ := newTestServer(t)
			dek, err := crypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			encryptedDEK, nonce, err := crypto.Encrypt(s.GetKEK(), dek)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.db.SaveNodeInfo(&db.NodeInfo{NodeID: "temp-test", Version: 1, DEK: encryptedDEK, DEKNonce: nonce}); err != nil {
				t.Fatal(err)
			}
			capture := &tokenRefCaptureStore{Store: s.chainStore}
			s.chainStore = capture
			valueBytes, err := crypto.GenerateKey()
			if err != nil {
				t.Fatal(err)
			}
			key := "plaintext"
			if keycenter {
				key = "value"
			}
			body, err := json.Marshal(map[string]string{key: hex.EncodeToString(valueBytes)})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/encrypt", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			if keycenter {
				s.handleKeycenterCreateTempRef(rec, req)
			} else {
				s.handleTempEncrypt(rec, req)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d", rec.Code)
			}
			if capture.payload == nil {
				t.Fatal("token reference transaction missing")
			}
			envelope, err := chain.BuildEnvelope(chain.TxSaveTokenRef, *capture.payload, chain.TxActor{})
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := chain.MarshalEnvelope(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(`"plaintext_hash"`)) {
				t.Fatal("token reference envelope contains a plaintext verifier")
			}
		})
	}
}

func TestRebuildMaskCacheContainsMalformedDEKPanic(t *testing.T) {
	s, _ := newTestServer(t)
	if err := s.db.UpsertAgent("mask-test", "mask", "vault-mask", "mask", "", 0, 0, 0, 1, 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.db.UpdateAgentDEK("mask-test", "a1b2c3d4", []byte{1}, []byte{1}); err != nil {
		t.Fatal(err)
	}
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		s.hkmHandler.RebuildMaskCache()
	}()
	select {
	case recovered := <-done:
		if recovered != nil {
			t.Fatal("cache rebuild allowed a decrypt panic to escape")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cache rebuild did not finish")
	}
}

type maskPanicTransport struct{ closeOnly bool }

type maskPanicBody struct{ *bytes.Reader }

func (maskPanicBody) Close() error { panic("test response cleanup failure") }

func (transport maskPanicTransport) RoundTrip(*http.Request) (*http.Response, error) {
	if transport.closeOnly {
		return &http.Response{StatusCode: http.StatusOK, Body: maskPanicBody{bytes.NewReader([]byte(`{"configs":[]}`))}}, nil
	}
	panic("test transport failure")
}

func TestRebuildMaskCacheContainsConfigWorkerPanic(t *testing.T) {
	for _, closeOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "transport", true: "cleanup"}[closeOnly], func(t *testing.T) {
			s, _ := newTestServer(t)
			if err := s.db.UpsertAgent("mask-config", "mask-config", "vault-config", "config", "127.0.0.1", 1234, 0, 0, 1, 1, ""); err != nil {
				t.Fatal(err)
			}
			s.httpClient = &http.Client{Transport: maskPanicTransport{closeOnly: closeOnly}}
			done := make(chan struct{})
			go func() { s.hkmHandler.RebuildMaskCache(); close(done) }()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Fatal("config worker failed to release cache rebuild")
			}
		})
	}
}

func TestSessionCookiesSecureByDefault(t *testing.T) {
	t.Setenv("VEILKEY_ALLOW_INSECURE_COOKIES", "")
	_, mux := newTestServer(t)
	for _, proto := range []string{"", "http", "https"} {
		for _, method := range []string{"login", "logout"} {
			req := loginRequest(testAdminPassword)
			if method == "logout" {
				req = httptest.NewRequest(http.MethodPost, "/api/admin/logout", nil)
			}
			req.Header.Set("X-Forwarded-Proto", proto)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			cookies := rec.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("session cookie missing")
			}
			if !cookies[0].Secure {
				t.Errorf("%s proto=%q returned an insecure cookie", method, proto)
			}
		}
	}
}
