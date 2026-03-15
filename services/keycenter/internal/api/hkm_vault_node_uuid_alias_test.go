package api

import (
	"encoding/json"
	"testing"
)

func TestRegisterAcceptsVaultNodeUUIDAlias(t *testing.T) {
	srv, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/register", map[string]string{
		"vault_node_uuid": "vault-node-alias-register",
		"label":           "alias-register",
		"url":             "http://198.51.100.10:10180",
	})
	if w.Code != 200 {
		t.Fatalf("register with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	child, err := srv.db.GetChild("vault-node-alias-register")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	if child == nil {
		t.Fatal("expected child row to exist")
	}
}

func TestHeartbeatAcceptsVaultNodeUUIDAlias(t *testing.T) {
	_, handler := setupHKMServer(t)
	const nodeID = "vault-node-alias-heartbeat"
	const url = "http://198.51.100.11:10180"

	w := postJSON(handler, "/api/register", map[string]string{
		"vault_node_uuid": nodeID,
		"label":           "alias-heartbeat",
		"url":             url,
	})
	if w.Code != 200 {
		t.Fatalf("register with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = postJSON(handler, "/api/heartbeat", map[string]interface{}{
		"vault_node_uuid": nodeID,
		"url":             url,
		"version":         1,
	})
	if w.Code != 200 {
		t.Fatalf("heartbeat with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAgentHeartbeatAcceptsVaultNodeUUIDAlias(t *testing.T) {
	srv, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/agents/heartbeat", map[string]interface{}{
		"vault_node_uuid": "vault-node-alias-agent",
		"label":           "alias-agent",
		"vault_hash":      "abc12345",
		"vault_name":      "alias-vault",
		"managed_paths":   []string{"/srv/alias"},
		"key_version":     1,
		"ip":              "10.0.0.77",
		"port":            10180,
		"secrets_count":   1,
		"configs_count":   2,
		"version":         1,
	})
	if w.Code != 200 {
		t.Fatalf("agent heartbeat with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Status           string `json:"status"`
		VaultRuntimeHash string `json:"vault_runtime_hash"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status == "" || resp.VaultRuntimeHash == "" {
		t.Fatalf("expected status and vault_runtime_hash, got %+v", resp)
	}

	agent, err := srv.db.GetAgentByNodeID("vault-node-alias-agent")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent row to exist")
	}
}

func TestTrackedRefSyncAcceptsVaultNodeUUIDAlias(t *testing.T) {
	srv, handler := setupHKMServer(t)
	const nodeID = "vault-node-alias-ref-sync"

	w := postJSON(handler, "/api/agents/heartbeat", map[string]interface{}{
		"vault_node_uuid": nodeID,
		"label":           "alias-ref-sync",
		"vault_hash":      "def67890",
		"vault_name":      "alias-sync-vault",
		"managed_paths":   []string{"/srv/sync"},
		"key_version":     1,
		"ip":              "10.0.0.88",
		"port":            10180,
		"secrets_count":   0,
		"configs_count":   0,
		"version":         1,
	})
	if w.Code != 200 {
		t.Fatalf("agent heartbeat with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = postJSON(handler, "/api/tracked-refs/sync", map[string]interface{}{
		"vault_node_uuid": nodeID,
		"ref":             "VK:TEMP:abcd1234",
		"version":         1,
		"status":          "temp",
	})
	if w.Code != 200 {
		t.Fatalf("tracked ref sync with vault_node_uuid: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ref, err := srv.db.GetRef("VK:TEMP:abcd1234")
	if err != nil {
		t.Fatalf("get tracked ref: %v", err)
	}
	if ref == nil {
		t.Fatal("expected tracked ref row to exist")
	}
}
