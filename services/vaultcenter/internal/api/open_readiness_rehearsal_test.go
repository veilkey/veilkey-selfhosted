package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestOpenReadinessRehearsalAgentSaveGetAndVersionGuard(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "open-readiness-agent", map[string]string{
		"VEILKEY_VAULTCENTER_URL": "http://127.0.0.1:10180",
	}, nil)

	save := postJSON(handler, "/api/agents/"+agentHash+"/secrets", map[string]string{
		"name":  "OPEN_ALPHA_SECRET",
		"value": "open-alpha-secret",
	})
	if save.Code != http.StatusOK {
		t.Fatalf("save secret expected 200, got %d: %s", save.Code, save.Body.String())
	}

	var saveResp struct {
		Ref   string `json:"ref"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(save.Body.Bytes(), &saveResp); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	if saveResp.Ref == "" || saveResp.Token == "" {
		t.Fatalf("save response missing ref/token: %#v", saveResp)
	}

	get := getJSON(handler, "/api/agents/"+agentHash+"/secrets/OPEN_ALPHA_SECRET")
	if get.Code != http.StatusOK {
		t.Fatalf("get secret expected 200, got %d: %s", get.Code, get.Body.String())
	}

	var getResp struct {
		Value string `json:"value"`
		Ref   string `json:"ref"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if getResp.Value != "open-alpha-secret" {
		t.Fatalf("value = %q", getResp.Value)
	}
	if getResp.Ref != saveResp.Ref {
		t.Fatalf("ref = %q, want %q", getResp.Ref, saveResp.Ref)
	}

	heartbeat := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "vault-node-open",
		"label":         "open-readiness-agent",
		"vault_hash":    "93a8094e",
		"vault_name":    "proxmox-test-lab-veilkey",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 1,
		"configs_count": 1,
		"version":       1,
	})
	if heartbeat.Code != http.StatusOK {
		t.Fatalf("heartbeat expected 200, got %d: %s", heartbeat.Code, heartbeat.Body.String())
	}

	mismatch := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "vault-node-open",
		"label":         "open-readiness-agent",
		"vault_hash":    "93a8094e",
		"vault_name":    "proxmox-test-lab-veilkey",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 1,
		"configs_count": 1,
		"version":       1,
	})
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("mismatch heartbeat expected 409, got %d: %s", mismatch.Code, mismatch.Body.String())
	}
	if !bytesContains(mismatch.Body.String(), "key_version_mismatch") {
		t.Fatalf("mismatch response should mention key_version_mismatch: %s", mismatch.Body.String())
	}
}

func bytesContains(s, needle string) bool {
	return len(s) >= len(needle) && (s == needle || len(needle) == 0 || stringContains(s, needle))
}

func stringContains(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
