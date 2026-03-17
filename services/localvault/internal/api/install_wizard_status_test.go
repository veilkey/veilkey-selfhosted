package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleInstallStatusIncludesKeycenterCompatibilityFields(t *testing.T) {
	server := setupStatusTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", upstream.URL); err != nil {
		t.Fatalf("SaveConfig VEILKEY_KEYCENTER_URL: %v", err)
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
	if resp["keycenter_connected"] != true {
		t.Fatalf("keycenter_connected = %#v", resp["keycenter_connected"])
	}
	if _, ok := resp["keycenter_error"]; ok {
		t.Fatalf("keycenter_error should be omitted on success, got %#v", resp["keycenter_error"])
	}
}

func TestHandleInstallStatusIncludesKeycenterErrorAlias(t *testing.T) {
	server := setupStatusTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer upstream.Close()

	if err := server.db.SaveConfig("VEILKEY_KEYCENTER_URL", upstream.URL); err != nil {
		t.Fatalf("SaveConfig VEILKEY_KEYCENTER_URL: %v", err)
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
	if resp["keycenter_connected"] != false {
		t.Fatalf("keycenter_connected = %#v", resp["keycenter_connected"])
	}
	if resp["keycenter_error"] != "keycenter returned 502 Bad Gateway" {
		t.Fatalf("keycenter_error = %#v", resp["keycenter_error"])
	}
	if resp["error"] != "keycenter returned 502 Bad Gateway" {
		t.Fatalf("error = %#v", resp["error"])
	}
}
