package api

import (
	"net/http"
	"testing"
)

func TestHKM_AgentUnregisterByNode(t *testing.T) {
	srv, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"vault_node_uuid": "node-unreg-1",
		"label":           "host-localvault",
		"vault_hash":      "abcd1234",
		"vault_name":      "host-localvault",
		"ip":              "10.0.0.70",
		"port":            10180,
		"configs_count":   2,
		"secrets_count":   0,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("register heartbeat: got %d", w.Code)
	}

	if _, err := srv.db.GetAgentByNodeID("node-unreg-1"); err != nil {
		t.Fatalf("agent should exist: %v", err)
	}

	del := deleteJSON(handler, "/api/agents/by-node/node-unreg-1")
	if del.Code != http.StatusOK {
		t.Fatalf("delete: got %d body=%s", del.Code, del.Body.String())
	}

	if _, err := srv.db.GetAgentByNodeID("node-unreg-1"); err == nil {
		t.Fatalf("agent should be deleted")
	}

	get := getJSON(handler, "/api/vaults?query=abcd1234")
	if get.Code != http.StatusOK {
		t.Fatalf("vault list: got %d", get.Code)
	}

	delAgain := deleteJSON(handler, "/api/agents/by-node/node-unreg-1")
	if delAgain.Code != http.StatusNotFound {
		t.Fatalf("delete again: expected 404, got %d", delAgain.Code)
	}
}
