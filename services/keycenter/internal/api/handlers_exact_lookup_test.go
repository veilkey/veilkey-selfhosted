package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"veilkey-keycenter/internal/crypto"
	"veilkey-keycenter/internal/db"
)

func TestExactLookupFindsTempRef(t *testing.T) {
	_, handler := setupTestServer(t)

	save := postJSON(handler, "/api/encrypt", map[string]string{"plaintext": "test-secret-value"})
	if save.Code != http.StatusOK {
		t.Fatalf("POST /api/encrypt = %d: %s", save.Code, save.Body.String())
	}

	var saved map[string]string
	if err := json.Unmarshal(save.Body.Bytes(), &saved); err != nil {
		t.Fatalf("json.Unmarshal(save): %v", err)
	}

	lookup := postJSON(handler, "/api/lookup/exact", map[string]string{"plaintext": "test-secret-value"})
	if lookup.Code != http.StatusOK {
		t.Fatalf("POST /api/lookup/exact = %d: %s", lookup.Code, lookup.Body.String())
	}

	var resp struct {
		Matches []exactLookupMatch `json:"matches"`
		Count   int                `json:"count"`
	}
	if err := json.Unmarshal(lookup.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal(lookup): %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("count = %d, want 1", resp.Count)
	}
	if len(resp.Matches) != 1 || resp.Matches[0].Ref != saved["ref"] {
		t.Fatalf("matches = %+v, want ref %q", resp.Matches, saved["ref"])
	}
}

func TestExactLookupFindsHostLocalSecret(t *testing.T) {
	srv, handler := setupTestServer(t)

	localDEK, err := srv.getLocalDEK()
	if err != nil {
		t.Fatalf("getLocalDEK: %v", err)
	}
	ciphertext, nonce, err := crypto.Encrypt(localDEK, []byte("already-managed"))
	if err != nil {
		t.Fatalf("crypto.Encrypt: %v", err)
	}

	if err := srv.db.SaveSecret(&db.Secret{
		ID:         crypto.GenerateUUID(),
		Name:       "OPENAI_API_KEY",
		Ref:        "deadbeef",
		Ciphertext: ciphertext,
		Nonce:      nonce,
		Version:    1,
	}); err != nil {
		t.Fatalf("SaveSecret: %v", err)
	}
	if err := srv.db.SaveRefWithName(
		db.RefParts{Family: "VK", Scope: "LOCAL", ID: "deadbeef"},
		"tracked-host",
		1,
		"active",
		"",
		"OPENAI_API_KEY",
	); err != nil {
		t.Fatalf("SaveRefWithName: %v", err)
	}

	lookup := postJSON(handler, "/api/lookup/exact", map[string]string{"plaintext": "already-managed"})
	if lookup.Code != http.StatusOK {
		t.Fatalf("POST /api/lookup/exact = %d: %s", lookup.Code, lookup.Body.String())
	}

	var resp struct {
		Matches []exactLookupMatch `json:"matches"`
		Count   int                `json:"count"`
	}
	if err := json.Unmarshal(lookup.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal(lookup): %v", err)
	}
	if resp.Count != 1 {
		t.Fatalf("count = %d, want 1", resp.Count)
	}
	if got := resp.Matches[0].Ref; got != "VK:LOCAL:deadbeef" {
		t.Fatalf("ref = %q, want VK:LOCAL:deadbeef", got)
	}
	if got := resp.Matches[0].SecretName; got != "OPENAI_API_KEY" {
		t.Fatalf("secret_name = %q, want OPENAI_API_KEY", got)
	}
}

func TestExactLookupNoMatch(t *testing.T) {
	_, handler := setupTestServer(t)

	lookup := postJSON(handler, "/api/lookup/exact", map[string]string{"plaintext": "unknown-value"})
	if lookup.Code != http.StatusOK {
		t.Fatalf("POST /api/lookup/exact = %d: %s", lookup.Code, lookup.Body.String())
	}
	if !strings.Contains(lookup.Body.String(), "\"count\":0") {
		t.Fatalf("body = %s, want count 0", lookup.Body.String())
	}
}

func TestExactLookupRejectsEmptyPlaintext(t *testing.T) {
	_, handler := setupTestServer(t)

	lookup := postJSON(handler, "/api/lookup/exact", map[string]string{"plaintext": ""})
	if lookup.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/lookup/exact = %d, want 400", lookup.Code)
	}
}
