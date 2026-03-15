package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestResolveHostVaultKeyRef(t *testing.T) {
	_, handler := setupHKMServer(t)

	save := postJSON(handler, "/api/host-vault/keys", map[string]string{
		"name":  "GITLAB_GLAB_PAT",
		"value": "pat-host-123",
		"scope": "LOCAL",
	})
	if save.Code != http.StatusOK {
		t.Fatalf("save host key: expected 200, got %d: %s", save.Code, save.Body.String())
	}

	var saveResp struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(save.Body.Bytes(), &saveResp); err != nil {
		t.Fatalf("unmarshal save response: %v", err)
	}
	if saveResp.Token == "" {
		t.Fatal("expected host key token in response")
	}

	w := getJSON(handler, "/api/resolve/"+saveResp.Token)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve host key: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resolve body: %v", err)
	}
	if body["value"] != "pat-host-123" {
		t.Fatalf("expected host key plaintext, got %#v", body["value"])
	}
	if body["vault_runtime_hash"] != "host" {
		t.Fatalf("expected host vault marker, got %#v", body["vault_runtime_hash"])
	}
}

func TestResolveHostVaultConfigRef(t *testing.T) {
	_, handler := setupHKMServer(t)

	save := postJSON(handler, "/api/host-vault/configs", map[string]string{
		"key":   "GITLAB_URL",
		"value": "https://gitlab-restore.test.internal",
		"scope": "LOCAL",
	})
	if save.Code != http.StatusOK {
		t.Fatalf("save host config: expected 200, got %d: %s", save.Code, save.Body.String())
	}

	var saveResp struct {
		Ref string `json:"ref"`
	}
	if err := json.Unmarshal(save.Body.Bytes(), &saveResp); err != nil {
		t.Fatalf("unmarshal save response: %v", err)
	}
	if saveResp.Ref == "" {
		t.Fatal("expected host config ref in response")
	}

	w := getJSON(handler, "/api/resolve/"+saveResp.Ref)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve host config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal resolve body: %v", err)
	}
	if body["value"] != "https://gitlab-restore.test.internal" {
		t.Fatalf("expected host config plaintext, got %#v", body["value"])
	}
}
