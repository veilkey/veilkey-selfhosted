package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAdminShellCoreFlowAPIs(t *testing.T) {
	t.Setenv("VEILKEY_PRODUCT_VERSION", "0.1.0")

	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "admin-shell-agent", map[string]string{
		"REPO_ORIGIN_URL": "https://gitlab.example/veilkey/services/veilkey.git",
	}, nil)

	saveKey := postJSON(handler, "/api/vaults/"+agentHash+"/keys", map[string]string{
		"name":  "LOGIN_ROOT_ID",
		"value": "rootid01",
	})
	if saveKey.Code != http.StatusOK {
		t.Fatalf("save key: expected 200, got %d: %s", saveKey.Code, saveKey.Body.String())
	}

	saveFn := postJSON(handler, "/api/functions/global", map[string]any{
		"name":          "admin-shell-function",
		"function_hash": "fnhash-admin-shell-001",
		"category":      "ops",
		"command":       "echo ok",
		"vars_json":     `{}`,
	})
	if saveFn.Code != http.StatusOK {
		t.Fatalf("save function: expected 200, got %d: %s", saveFn.Code, saveFn.Body.String())
	}

	tests := []struct {
		name string
		path string
	}{
		{name: "ui config", path: "/api/ui/config"},
		{name: "system update", path: "/api/system/update"},
		{name: "functions list", path: "/api/functions/global"},
		{name: "function detail", path: "/api/functions/global/admin-shell-function"},
		{name: "function bindings", path: "/api/targets/function/admin-shell-function/bindings"},
		{name: "function impact", path: "/api/targets/function/admin-shell-function/impact"},
		{name: "function summary", path: "/api/targets/function/admin-shell-function/summary"},
		{name: "vault detail", path: "/api/vaults/" + agentHash},
		{name: "vault keys", path: "/api/vaults/" + agentHash + "/keys"},
		{name: "vault key detail", path: "/api/vaults/" + agentHash + "/keys/LOGIN_ROOT_ID"},
		{name: "agent configs", path: "/api/agents/" + agentHash + "/configs"},
		{name: "agent config detail", path: "/api/agents/" + agentHash + "/configs/REPO_ORIGIN_URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := getJSON(handler, tt.path)
			if w.Code != http.StatusOK {
				t.Fatalf("%s: expected 200, got %d: %s", tt.path, w.Code, w.Body.String())
			}
			if !json.Valid(w.Body.Bytes()) {
				t.Fatalf("%s: expected valid json, got %q", tt.path, w.Body.String())
			}
		})
	}
}
