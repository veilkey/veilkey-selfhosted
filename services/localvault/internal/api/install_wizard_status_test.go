package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleInstallStatusIncludesVaultcenterCompatibilityFields(t *testing.T) {
	server := setupStatusTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	if err := server.db.SaveConfig("VEILKEY_VAULTCENTER_URL", upstream.URL); err != nil {
		t.Fatalf("SaveConfig VEILKEY_VAULTCENTER_URL: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/install/status", nil)
	w := httptest.NewRecorder()

	server.HandleInstallStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["connected"] != true {
		t.Fatalf("connected = %#v", resp["connected"])
	}
	if resp["vaultcenter_connected"] != true {
		t.Fatalf("vaultcenter_connected = %#v", resp["vaultcenter_connected"])
	}
	if _, ok := resp["vaultcenter_error"]; ok {
		t.Fatalf("vaultcenter_error should be omitted on success, got %#v", resp["vaultcenter_error"])
	}
}

func TestHandleInstallStatusIncludesVaultcenterErrorAlias(t *testing.T) {
	server := setupStatusTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()

	if err := server.db.SaveConfig("VEILKEY_VAULTCENTER_URL", upstream.URL); err != nil {
		t.Fatalf("SaveConfig VEILKEY_VAULTCENTER_URL: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/install/status", nil)
	w := httptest.NewRecorder()

	server.HandleInstallStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["connected"] != false {
		t.Fatalf("connected = %#v", resp["connected"])
	}
	if resp["vaultcenter_connected"] != false {
		t.Fatalf("vaultcenter_connected = %#v", resp["vaultcenter_connected"])
	}
	if resp["vaultcenter_error"] != "vaultcenter returned 502 Bad Gateway" {
		t.Fatalf("vaultcenter_error = %#v", resp["vaultcenter_error"])
	}
	if resp["error"] != "vaultcenter returned 502 Bad Gateway" {
		t.Fatalf("error = %#v", resp["error"])
	}
}
