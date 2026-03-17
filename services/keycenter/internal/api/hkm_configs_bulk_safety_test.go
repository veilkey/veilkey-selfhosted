package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// === Safety: bulk-update rejects simultaneous key+value change ===

func TestHKM_BulkUpdate_RejectsKeyAndValueChange(t *testing.T) {
	_, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "OLD_KEY",
		"new_key":   "NEW_KEY",
		"new_value": "new-value",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when changing both key and value, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
		Hint  string `json:"hint"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Error == "" {
		t.Error("expected error message about simultaneous key+value change")
	}
	if resp.Hint == "" {
		t.Error("expected hint about splitting the operation")
	}
}

// === Safety: bulk-update allows same key name in new_key (no actual rename) ===

func TestHKM_BulkUpdate_AllowsSameKeyName(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "same-key-test", map[string]string{"PORT": "8080"}, nil)

	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "PORT",
		"new_key":   "PORT",
		"new_value": "9090",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when new_key == key, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Updated != 1 {
		t.Errorf("updated = %d, want 1", resp.Updated)
	}
}

// === Safety: bulk-set rejects when all agents already have exact same key+value ===

func TestHKM_BulkSet_RejectsNoOpOverwrite(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "redis-a", map[string]string{"REDIS_HOST": "198.51.100.14"}, nil)
	registerMockAgent(t, srv, "redis-b", map[string]string{"REDIS_HOST": "198.51.100.14"}, nil)
	registerMockAgent(t, srv, "redis-c", map[string]string{"REDIS_HOST": "198.51.100.14"}, nil)

	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":   "REDIS_HOST",
		"value": "198.51.100.14",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 when all agents already have same value, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string `json:"error"`
		AgentCount int    `json:"agent_count"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AgentCount != 3 {
		t.Errorf("agent_count = %d, want 3", resp.AgentCount)
	}
}

// === Safety: bulk-set allows when at least one agent has different value ===

func TestHKM_BulkSet_AllowsWhenValuesDiffer(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "svc-1", map[string]string{"KEYCENTER_URL": "http://old:10180"}, nil)
	registerMockAgent(t, srv, "svc-2", map[string]string{"KEYCENTER_URL": "http://new:10180"}, nil)

	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":   "KEYCENTER_URL",
		"value": "http://new:10180",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when values differ, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// svc-1 updated (old→new), svc-2 already has value → skipped, only 1 updated
	if resp.Updated != 1 {
		t.Errorf("updated = %d, want 1", resp.Updated)
	}
}

// === Safety: bulk-set allows when key doesn't exist on any agent ===

func TestHKM_BulkSet_AllowsNewKey(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "fresh-1", map[string]string{"DOMAIN": "test.kr"}, nil)
	registerMockAgent(t, srv, "fresh-2", map[string]string{"DOMAIN": "test2.kr"}, nil)

	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":   "NEW_CONFIG_KEY",
		"value": "new-value",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for new key, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Updated != 2 {
		t.Errorf("updated = %d, want 2", resp.Updated)
	}
}

// === Safety: bulk-set with overwrite=false skips existing ===

func TestHKM_BulkSet_NoOverwrite_SkipsExisting(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "ow-1", map[string]string{"KEY_A": "existing"}, nil)
	registerMockAgent(t, srv, "ow-2", map[string]string{}, nil)

	overwrite := false
	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":       "KEY_A",
		"value":     "new-val",
		"overwrite": overwrite,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// ow-1: key exists, overwrite=false → skip; ow-2: create → 1 updated
	if resp.Updated != 1 {
		t.Errorf("updated = %d, want 1", resp.Updated)
	}
}

// === All-or-nothing: bulk-set rolls back on partial failure ===

func TestHKM_BulkSet_RollbackOnPartialFailure(t *testing.T) {
	srv, handler := setupHKMServer(t)

	// Agent 1: normal mock
	registerMockAgent(t, srv, "ok-agent", map[string]string{"PORT": "8080"}, nil)

	// Agent 2: mock that fails on POST /api/configs
	failAgent := newFailingMockAgent(map[string]string{"PORT": "8080"})
	t.Cleanup(failAgent.Close)
	registerMockAgentWithServer(t, srv, "fail-agent", failAgent)

	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":   "PORT",
		"value": "9090",
	})
	// Should fail because one agent failed
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on partial failure, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string   `json:"error"`
		Failed     []string `json:"failed"`
		RolledBack int      `json:"rolled_back"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Failed) == 0 {
		t.Error("expected failed list to be non-empty")
	}

	// Verify ok-agent was rolled back to original value
	w = getJSON(handler, "/api/configs/search/PORT")
	var searchResp struct {
		Matches []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"matches"`
	}
	json.Unmarshal(w.Body.Bytes(), &searchResp)
	for _, m := range searchResp.Matches {
		if m.Label == "ok-agent" && m.Value != "8080" {
			t.Errorf("ok-agent PORT should be rolled back to 8080, got %q", m.Value)
		}
	}
}

// === All-or-nothing: bulk-update rolls back on partial failure ===

func TestHKM_BulkUpdate_RollbackOnPartialFailure(t *testing.T) {
	srv, handler := setupHKMServer(t)

	// Agent 1: normal
	registerMockAgent(t, srv, "good-agent", map[string]string{"DB_HOST": "198.51.100.12"}, nil)

	// Agent 2: fails on POST
	failAgent := newFailingMockAgent(map[string]string{"DB_HOST": "198.51.100.12"})
	t.Cleanup(failAgent.Close)
	registerMockAgentWithServer(t, srv, "bad-agent", failAgent)

	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "DB_HOST",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Error      string   `json:"error"`
		Failed     []string `json:"failed"`
		RolledBack int      `json:"rolled_back"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Failed) == 0 {
		t.Error("expected failed list")
	}
}

// --- Test helpers for failing agents ---

// newFailingMockAgent creates a mock that serves GET normally but fails POST /api/configs
func newFailingMockAgent(configs map[string]string) *httptest.Server {
	var mu sync.RWMutex
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/configs/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		mu.RLock()
		v, ok := configs[key]
		mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"key": key, "value": v})
	})

	// POST always fails with 500
	mux.HandleFunc("POST /api/configs", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "simulated failure"})
	})

	mux.HandleFunc("DELETE /api/configs/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		mu.Lock()
		delete(configs, key)
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]string{"deleted": key})
	})

	return httptest.NewServer(mux)
}

// registerMockAgentWithServer registers an existing httptest.Server as an agent
func registerMockAgentWithServer(t *testing.T, srv *Server, label string, mockSrv *httptest.Server) {
	t.Helper()
	addr := mockSrv.URL[len("http://"):]
	var ip string
	var port int
	for i, c := range addr {
		if c == ':' {
			ip = addr[:i]
			for _, d := range addr[i+1:] {
				port = port*10 + int(d-'0')
			}
			break
		}
	}
	nodeID := "node-" + label
	if err := srv.db.UpsertAgent(nodeID, label, "hash-"+label, label, ip, port, 0, 0, 1, 1); err != nil {
		t.Fatalf("UpsertAgent %s: %v", label, err)
	}
	agentHash, _ := generateAgentHash()
	if err := srv.db.UpdateAgentDEK(nodeID, agentHash, []byte("fake-dek"), []byte("fake-nonce")); err != nil {
		t.Fatalf("UpdateAgentDEK %s: %v", label, err)
	}
}

// === Existing behavior preserved: bulk-update with multiple values requires old_value ===

func TestHKM_BulkUpdate_MultipleValues_StillRequiresOldValue(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "svc-x", map[string]string{"DB_HOST": "198.51.100.12"}, nil)
	registerMockAgent(t, srv, "svc-y", map[string]string{"DB_HOST": "198.51.100.13"}, nil)

	// Without old_value → 409
	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "DB_HOST",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}

	// With old_value → only matching agent updated
	w = postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "DB_HOST",
		"old_value": "198.51.100.12",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
		Skipped int `json:"skipped"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Updated != 1 {
		t.Errorf("updated = %d, want 1", resp.Updated)
	}
	if resp.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", resp.Skipped)
	}
}

// === Verify: all-or-nothing means successful agent's value is actually restored ===

func TestHKM_BulkSet_RollbackRestoresOriginalValue(t *testing.T) {
	srv, handler := setupHKMServer(t)

	// Create a normal agent we can inspect directly via HTTP
	normalMock := newMockAgent(map[string]string{"SETTING": "original-value"}, nil)
	t.Cleanup(normalMock.Close)
	registerMockAgentWithServer(t, srv, "inspect-agent", normalMock)

	// Create a failing agent
	failMock := newFailingMockAgent(map[string]string{"SETTING": "original-value"})
	t.Cleanup(failMock.Close)
	registerMockAgentWithServer(t, srv, "fail-agent-2", failMock)

	w := postJSON(handler, "/api/configs/bulk-set", map[string]interface{}{
		"key":   "SETTING",
		"value": "changed-value",
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}

	// Directly query the normal mock to verify rollback
	resp, err := http.Get(normalMock.URL + "/api/configs/SETTING")
	if err != nil {
		t.Fatalf("direct GET failed: %v", err)
	}
	defer resp.Body.Close()
	var data struct {
		Value string `json:"value"`
	}
	json.NewDecoder(resp.Body).Decode(&data)
	if data.Value != "original-value" {
		t.Errorf("after rollback, SETTING = %q, want original-value", data.Value)
	}
}
