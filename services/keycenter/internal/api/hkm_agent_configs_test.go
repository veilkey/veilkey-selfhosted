package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"veilkey-keycenter/internal/crypto"
)

// mockAgent simulates a veilkey-localvault with configs + secrets endpoints
type mockAgent struct {
	mu      sync.RWMutex
	configs map[string]mockConfig
	secrets map[string]mockSecret // name → encrypted secret metadata
}

type mockConfig struct {
	Value  string
	Scope  string
	Status string
}

type mockSecret struct {
	Ref        string
	Ciphertext []byte
	Nonce      []byte
	Version    int
	Scope      string
	Status     string
	Fields     map[string]mockSecretField
}

type mockSecretField struct {
	Type       string
	Ciphertext []byte
	Nonce      []byte
}

func newMockAgent(configs map[string]string, secrets map[string]string) *httptest.Server {
	agent := &mockAgent{
		configs: map[string]mockConfig{},
		secrets: map[string]mockSecret{},
	}
	for key, value := range configs {
		agent.configs[key] = mockConfig{Value: value, Scope: "LOCAL", Status: "active"}
	}
	for name, ref := range secrets {
		scope := "LOCAL"
		if strings.HasPrefix(ref, "EXTERNAL:") {
			scope = "EXTERNAL"
			ref = strings.TrimPrefix(ref, "EXTERNAL:")
		} else if strings.HasPrefix(ref, "TEMP:") {
			scope = "TEMP"
			ref = strings.TrimPrefix(ref, "TEMP:")
		}
		agent.secrets[name] = mockSecret{Ref: ref, Version: 1, Scope: scope, Status: "active", Fields: map[string]mockSecretField{}}
	}
	mux := http.NewServeMux()

	// Configs endpoints
	mux.HandleFunc("GET /api/configs", func(w http.ResponseWriter, r *http.Request) {
		agent.mu.RLock()
		defer agent.mu.RUnlock()
		type entry struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Scope  string `json:"scope"`
			Status string `json:"status"`
		}
		var result []entry
		for k, cfg := range agent.configs {
			scope := cfg.Scope
			if scope == "" {
				scope = "LOCAL"
			}
			status := cfg.Status
			if status == "" {
				status = "active"
			}
			result = append(result, entry{Key: k, Value: cfg.Value, Scope: scope, Status: status})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"configs": result, "count": len(result)})
	})

	mux.HandleFunc("GET /api/configs/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		agent.mu.RLock()
		cfg, ok := agent.configs[key]
		agent.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		scope := cfg.Scope
		if scope == "" {
			scope = "LOCAL"
		}
		status := cfg.Status
		if status == "" {
			status = "active"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"key": key, "value": cfg.Value, "scope": scope, "status": status})
	})

	mux.HandleFunc("POST /api/configs", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Scope  string `json:"scope"`
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		agent.mu.Lock()
		scope := req.Scope
		if scope == "" {
			scope = "LOCAL"
		}
		status := req.Status
		if status == "" {
			if scope == "TEMP" {
				status = "temp"
			} else {
				status = "active"
			}
		}
		agent.configs[req.Key] = mockConfig{Value: req.Value, Scope: scope, Status: status}
		agent.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"key": req.Key, "value": req.Value, "scope": scope, "status": status, "action": "saved"})
	})

	mux.HandleFunc("PUT /api/configs/bulk", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Configs map[string]string `json:"configs"`
			Scope   string            `json:"scope"`
			Status  string            `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		agent.mu.Lock()
		scope := req.Scope
		if scope == "" {
			scope = "LOCAL"
		}
		status := req.Status
		if status == "" {
			if scope == "TEMP" {
				status = "temp"
			} else {
				status = "active"
			}
		}
		for k, v := range req.Configs {
			agent.configs[k] = mockConfig{Value: v, Scope: scope, Status: status}
		}
		agent.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"saved": len(req.Configs)})
	})

	mux.HandleFunc("DELETE /api/configs/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		agent.mu.Lock()
		defer agent.mu.Unlock()
		if _, ok := agent.configs[key]; !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		delete(agent.configs, key)
		_ = json.NewEncoder(w).Encode(map[string]string{"deleted": key})
	})

	// Secrets endpoints (return metadata only, no plaintext)
	mux.HandleFunc("GET /api/secrets", func(w http.ResponseWriter, r *http.Request) {
		agent.mu.RLock()
		defer agent.mu.RUnlock()
		type entry struct {
			Name        string `json:"name"`
			Ref         string `json:"ref"`
			Scope       string `json:"scope"`
			Status      string `json:"status"`
			FieldsCount int    `json:"fields_count,omitempty"`
		}
		var result []entry
		for name, sec := range agent.secrets {
			scope := sec.Scope
			if scope == "" {
				scope = "TEMP"
			}
			status := sec.Status
			if status == "" {
				status = "temp"
			}
			result = append(result, entry{Name: name, Ref: sec.Ref, Scope: scope, Status: status, FieldsCount: len(sec.Fields)})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"secrets": result, "count": len(result)})
	})

	mux.HandleFunc("GET /api/secrets/meta/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		agent.mu.RLock()
		sec, ok := agent.secrets[name]
		agent.mu.RUnlock()
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		scope := sec.Scope
		if scope == "" {
			scope = "TEMP"
		}
		status := sec.Status
		if status == "" {
			status = "temp"
		}
		fields := make([]map[string]string, 0, len(sec.Fields))
		for key, field := range sec.Fields {
			fields = append(fields, map[string]string{"key": key, "type": field.Type})
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": name, "ref": sec.Ref, "scope": scope, "status": status, "token": "VK:" + scope + ":" + sec.Ref, "version": sec.Version, "fields": fields, "fields_count": len(fields)})
	})

	mux.HandleFunc("GET /api/cipher/{ref}", func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("ref")
		agent.mu.RLock()
		defer agent.mu.RUnlock()
		for name, sec := range agent.secrets {
			if sec.Ref == ref {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"name":       name,
					"ciphertext": sec.Ciphertext,
					"nonce":      sec.Nonce,
					"version":    sec.Version,
				})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})

	mux.HandleFunc("POST /api/cipher", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name       string `json:"name"`
			Ref        string `json:"ref"`
			Ciphertext []byte `json:"ciphertext"`
			Nonce      []byte `json:"nonce"`
			Version    int    `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || len(req.Ciphertext) == 0 || len(req.Nonce) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		agent.mu.Lock()
		defer agent.mu.Unlock()
		sec, ok := agent.secrets[req.Name]
		action := "updated"
		if !ok {
			sec = mockSecret{Ref: req.Ref, Version: req.Version, Scope: "TEMP", Status: "temp", Fields: map[string]mockSecretField{}}
			if sec.Ref == "" {
				sec.Ref = "generatedref"
			}
			action = "created"
		}
		sec.Ciphertext = req.Ciphertext
		sec.Nonce = req.Nonce
		if req.Version != 0 {
			sec.Version = req.Version
		}
		if sec.Scope == "" {
			sec.Scope = "TEMP"
		}
		if sec.Status == "" {
			sec.Status = "temp"
		}
		agent.secrets[req.Name] = sec
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": req.Name, "ref": sec.Ref, "token": "VK:" + sec.Scope + ":" + sec.Ref, "scope": sec.Scope, "status": sec.Status, "action": action})
	})

	mux.HandleFunc("POST /api/secrets/fields", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name   string `json:"name"`
			Fields []struct {
				Key        string `json:"key"`
				Type       string `json:"type"`
				Ciphertext []byte `json:"ciphertext"`
				Nonce      []byte `json:"nonce"`
			} `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || len(req.Fields) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}
		agent.mu.Lock()
		defer agent.mu.Unlock()
		sec, ok := agent.secrets[req.Name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		if (sec.Scope != "LOCAL" && sec.Scope != "EXTERNAL") || sec.Status != "active" {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "secret fields require VK:LOCAL or VK:EXTERNAL active lifecycle"})
			return
		}
		if sec.Fields == nil {
			sec.Fields = map[string]mockSecretField{}
		}
		for _, field := range req.Fields {
			sec.Fields[field.Key] = mockSecretField{Type: field.Type, Ciphertext: field.Ciphertext, Nonce: field.Nonce}
		}
		agent.secrets[req.Name] = sec
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"name": req.Name, "saved": len(req.Fields)})
	})

	mux.HandleFunc("GET /api/cipher/{ref}/fields/{field}", func(w http.ResponseWriter, r *http.Request) {
		ref := r.PathValue("ref")
		fieldKey := r.PathValue("field")
		agent.mu.RLock()
		defer agent.mu.RUnlock()
		for name, sec := range agent.secrets {
			if sec.Ref != ref {
				continue
			}
			field, ok := sec.Fields[fieldKey]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"name":       name,
				"field":      fieldKey,
				"type":       field.Type,
				"ciphertext": field.Ciphertext,
				"nonce":      field.Nonce,
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	})

	mux.HandleFunc("DELETE /api/secrets/{name}/fields/{field}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		fieldKey := r.PathValue("field")
		agent.mu.Lock()
		defer agent.mu.Unlock()
		sec, ok := agent.secrets[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		if _, ok := sec.Fields[fieldKey]; !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		delete(sec.Fields, fieldKey)
		agent.secrets[name] = sec
		_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "deleted": fieldKey})
	})

	mux.HandleFunc("DELETE /api/secrets/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		agent.mu.Lock()
		defer agent.mu.Unlock()
		if _, ok := agent.secrets[name]; !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
			return
		}
		delete(agent.secrets, name)
		_ = json.NewEncoder(w).Encode(map[string]string{"deleted": name})
	})

	return httptest.NewServer(mux)
}

// registerMockAgent creates a mock agent and registers it in Hub DB
func registerMockAgent(t *testing.T, srv *Server, label string, configs map[string]string, secrets map[string]string) (*httptest.Server, string) {
	t.Helper()
	mockSrv := newMockAgent(configs, secrets)
	t.Cleanup(mockSrv.Close)

	// Extract host:port from mock server URL
	addr := strings.TrimPrefix(mockSrv.URL, "http://")
	parts := strings.Split(addr, ":")
	ip := parts[0]
	port := 0
	if len(parts) > 1 {
		for _, c := range parts[1] {
			port = port*10 + int(c-'0')
		}
	}

	nodeID := "node-" + label
	secretsCount := len(secrets)
	configsCount := len(configs)

	if err := srv.db.UpsertAgent(nodeID, label, "hash-"+label, label, ip, port, secretsCount, configsCount, 1, 1); err != nil {
		t.Fatalf("UpsertAgent %s: %v", label, err)
	}

	// Assign agent hash
	agentHash, err := generateAgentHash()
	if err != nil {
		t.Fatalf("generateAgentHash %s: %v", label, err)
	}
	agentDEK, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey %s: %v", label, err)
	}
	encDEK, encNonce, err := crypto.Encrypt(srv.kek, agentDEK)
	if err != nil {
		t.Fatalf("Encrypt %s: %v", label, err)
	}
	if err := srv.db.UpdateAgentDEK(nodeID, agentHash, encDEK, encNonce); err != nil {
		t.Fatalf("UpdateAgentDEK %s: %v", label, err)
	}

	return mockSrv, agentHash
}

// === Test: Agent Configs CRUD via Hub Proxy ===

func TestHKM_AgentConfigsCRUD(t *testing.T) {
	srv, handler := setupHKMServer(t)

	configs := map[string]string{"DOMAIN": "test.example.com", "SERVICE_PORT": "8080"}
	_, agentHash := registerMockAgent(t, srv, "test-service", configs, nil)

	// List configs
	w := getJSON(handler, "/api/agents/"+agentHash+"/configs")
	if w.Code != http.StatusOK {
		t.Fatalf("list configs: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list configs: %v", err)
	}
	if listResp.Count != 2 {
		t.Errorf("configs count = %d, want 2", listResp.Count)
	}

	// Get single config
	w = getJSON(handler, "/api/agents/"+agentHash+"/configs/DOMAIN")
	if w.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d", w.Code)
	}
	var getResp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get config: %v", err)
	}
	if getResp.Value != "test.example.com" {
		t.Errorf("DOMAIN value = %q, want test.example.com", getResp.Value)
	}

	// Save new config
	w = postJSON(handler, "/api/agents/"+agentHash+"/configs", map[string]string{"key": "NEW_KEY", "value": "new_val"})
	if w.Code != http.StatusOK {
		t.Fatalf("save config: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := srv.db.GetRef("VE:LOCAL:NEW_KEY"); err != nil {
		t.Fatalf("tracked config ref missing after save: %v", err)
	}
	saveAudit, err := srv.db.ListAuditEvents("config", "VE:LOCAL:NEW_KEY")
	if err != nil {
		t.Fatalf("ListAuditEvents(save): %v", err)
	}
	if len(saveAudit) == 0 || saveAudit[0].Action != "save" {
		t.Fatalf("expected config save audit, got %+v", saveAudit)
	}

	// Verify new config exists
	w = getJSON(handler, "/api/agents/"+agentHash+"/configs/NEW_KEY")
	if w.Code != http.StatusOK {
		t.Fatalf("get new config: expected 200, got %d", w.Code)
	}

	// Delete config
	w = deleteJSON(handler, "/api/agents/"+agentHash+"/configs/NEW_KEY")
	if w.Code != http.StatusOK {
		t.Fatalf("delete config: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := srv.db.GetRef("VE:LOCAL:NEW_KEY"); err == nil {
		t.Fatal("tracked config ref should be removed after delete")
	}
	deleteAudit, err := srv.db.ListAuditEvents("config", "VE:LOCAL:NEW_KEY")
	if err != nil {
		t.Fatalf("ListAuditEvents(delete): %v", err)
	}
	if len(deleteAudit) < 2 || deleteAudit[0].Action != "delete" {
		t.Fatalf("expected config delete audit, got %+v", deleteAudit)
	}

	// Verify deleted
	w = getJSON(handler, "/api/agents/"+agentHash+"/configs/NEW_KEY")
	if w.Code != http.StatusNotFound {
		t.Errorf("get deleted config: expected 404, got %d", w.Code)
	}
}

func TestHKM_AgentConfigsSupportTempAndExternalScopes(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "scoped-configs", map[string]string{}, nil)

	saveTemp := postJSON(handler, "/api/agents/"+agentHash+"/configs", map[string]string{
		"key":   "BOOTSTRAP_TOKEN",
		"value": "tmp-123",
		"scope": "TEMP",
	})
	if saveTemp.Code != http.StatusOK {
		t.Fatalf("save temp config: expected 200, got %d: %s", saveTemp.Code, saveTemp.Body.String())
	}
	if _, err := srv.db.GetRef("VE:TEMP:BOOTSTRAP_TOKEN"); err != nil {
		t.Fatalf("tracked temp config ref missing: %v", err)
	}

	saveExternal := postJSON(handler, "/api/agents/"+agentHash+"/configs", map[string]string{
		"key":   "REMOTE_API",
		"value": "ext-456",
		"scope": "EXTERNAL",
	})
	if saveExternal.Code != http.StatusOK {
		t.Fatalf("save external config: expected 200, got %d: %s", saveExternal.Code, saveExternal.Body.String())
	}
	if _, err := srv.db.GetRef("VE:EXTERNAL:REMOTE_API"); err != nil {
		t.Fatalf("tracked external config ref missing: %v", err)
	}

	list := getJSON(handler, "/api/agents/"+agentHash+"/configs")
	if list.Code != http.StatusOK {
		t.Fatalf("list configs: expected 200, got %d: %s", list.Code, list.Body.String())
	}
	var payload struct {
		Configs []map[string]any `json:"configs"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal config list: %v", err)
	}
	scopes := map[string]string{}
	for _, cfg := range payload.Configs {
		key, _ := cfg["key"].(string)
		scope, _ := cfg["scope"].(string)
		scopes[key] = scope
	}
	if scopes["BOOTSTRAP_TOKEN"] != "TEMP" {
		t.Fatalf("BOOTSTRAP_TOKEN scope = %q, want TEMP", scopes["BOOTSTRAP_TOKEN"])
	}
	if scopes["REMOTE_API"] != "EXTERNAL" {
		t.Fatalf("REMOTE_API scope = %q, want EXTERNAL", scopes["REMOTE_API"])
	}
}

// === Test: Bulk Save Configs ===

func TestHKM_AgentConfigsBulkSave(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "bulk-test", map[string]string{}, nil)

	w := putJSON(handler, "/api/agents/"+agentHash+"/configs/bulk", map[string]interface{}{
		"configs": map[string]string{
			"DOMAIN":     "bulk.example.com",
			"DB_HOST":    "198.51.100.12",
			"REDIS_HOST": "198.51.100.14",
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk save: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Saved int `json:"saved"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal bulk save: %v", err)
	}
	if resp.Saved != 3 {
		t.Errorf("saved = %d, want 3", resp.Saved)
	}

	// Verify all three exist
	for _, key := range []string{"DOMAIN", "DB_HOST", "REDIS_HOST"} {
		w = getJSON(handler, "/api/agents/"+agentHash+"/configs/"+key)
		if w.Code != http.StatusOK {
			t.Errorf("get %s after bulk: expected 200, got %d", key, w.Code)
		}
		if _, err := srv.db.GetRef("VE:LOCAL:" + key); err != nil {
			t.Errorf("tracked config ref missing for %s: %v", key, err)
		}
		auditRows, err := srv.db.ListAuditEvents("config", "VE:LOCAL:"+key)
		if err != nil {
			t.Fatalf("ListAuditEvents(%s): %v", key, err)
		}
		if len(auditRows) == 0 || auditRows[0].Action != "save" {
			t.Fatalf("expected bulk save audit for %s, got %+v", key, auditRows)
		}
	}
}

// === Test: Configs Summary ===

func TestHKM_ConfigsSummary(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "svc-a", map[string]string{"DOMAIN": "a.example.com", "PORT": "8080"}, nil)
	registerMockAgent(t, srv, "svc-b", map[string]string{"DOMAIN": "b.example.com"}, nil)
	registerMockAgent(t, srv, "svc-c", map[string]string{}, map[string]string{"SECRET_A": "ref1"}) // no configs

	w := getJSON(handler, "/api/configs/summary")
	if w.Code != http.StatusOK {
		t.Fatalf("summary: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalConfigs      int `json:"total_configs"`
		TotalAgents       int `json:"total_agents"`
		AgentsWithConfigs int `json:"agents_with_configs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}

	if resp.TotalAgents != 3 {
		t.Errorf("total_agents = %d, want 3", resp.TotalAgents)
	}
	if resp.TotalConfigs != 3 { // 2 + 1 + 0
		t.Errorf("total_configs = %d, want 3", resp.TotalConfigs)
	}
	if resp.AgentsWithConfigs != 2 {
		t.Errorf("agents_with_configs = %d, want 2", resp.AgentsWithConfigs)
	}
}

// === Test: Search Key Across Agents ===

func TestHKM_ConfigsSearchKey(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "web-1", map[string]string{"DOMAIN": "web1.example.com", "PORT": "8080"}, nil)
	registerMockAgent(t, srv, "web-2", map[string]string{"DOMAIN": "web2.example.com", "PORT": "8080"}, nil)
	registerMockAgent(t, srv, "api-1", map[string]string{"DOMAIN": "api.example.com", "PORT": "9000"}, nil)

	// Search DOMAIN — 3 agents, 3 unique values
	w := getJSON(handler, "/api/configs/search/DOMAIN")
	if w.Code != http.StatusOK {
		t.Fatalf("search DOMAIN: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Key          string         `json:"key"`
		MatchCount   int            `json:"match_count"`
		UniqueValues int            `json:"unique_values"`
		ValueSummary map[string]int `json:"value_summary"`
		ScopeSummary map[string]int `json:"scope_summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal search DOMAIN: %v", err)
	}

	if resp.MatchCount != 3 {
		t.Errorf("DOMAIN match_count = %d, want 3", resp.MatchCount)
	}
	if resp.UniqueValues != 3 {
		t.Errorf("DOMAIN unique_values = %d, want 3", resp.UniqueValues)
	}
	if resp.ScopeSummary["LOCAL"] != 3 {
		t.Errorf("DOMAIN LOCAL scope count = %d, want 3", resp.ScopeSummary["LOCAL"])
	}

	// Search PORT — 3 agents, 2 unique values (8080 x2, 9000 x1)
	w = getJSON(handler, "/api/configs/search/PORT")
	if w.Code != http.StatusOK {
		t.Fatalf("search PORT: expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal search PORT: %v", err)
	}

	if resp.MatchCount != 3 {
		t.Errorf("PORT match_count = %d, want 3", resp.MatchCount)
	}
	if resp.UniqueValues != 2 {
		t.Errorf("PORT unique_values = %d, want 2", resp.UniqueValues)
	}
	if resp.ValueSummary["8080"] != 2 {
		t.Errorf("PORT 8080 count = %d, want 2", resp.ValueSummary["8080"])
	}
	if resp.ValueSummary["9000"] != 1 {
		t.Errorf("PORT 9000 count = %d, want 1", resp.ValueSummary["9000"])
	}

	// Search nonexistent key
	w = getJSON(handler, "/api/configs/search/NONEXISTENT")
	if w.Code != http.StatusOK {
		t.Fatalf("search NONEXISTENT: expected 200, got %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal search NONEXISTENT: %v", err)
	}
	if resp.MatchCount != 0 {
		t.Errorf("NONEXISTENT match_count = %d, want 0", resp.MatchCount)
	}
}

// === Test: Bulk Update — Single Unique Value (no old_value needed) ===

func TestHKM_ConfigsBulkUpdate_SingleValue(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "redis-1", map[string]string{"REDIS_HOST": "198.51.100.14"}, nil)
	registerMockAgent(t, srv, "redis-2", map[string]string{"REDIS_HOST": "198.51.100.14"}, nil)

	// All agents have same value → can update without old_value
	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "REDIS_HOST",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-update: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal bulk-update: %v", err)
	}
	if resp.Updated != 2 {
		t.Errorf("updated = %d, want 2", resp.Updated)
	}

	// Verify both agents updated
	w = getJSON(handler, "/api/configs/search/REDIS_HOST")
	var searchResp struct {
		Matches []struct {
			Value string `json:"value"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &searchResp); err != nil {
		t.Fatalf("unmarshal search REDIS_HOST: %v", err)
	}
	for _, m := range searchResp.Matches {
		if m.Value != "198.51.100.200" {
			t.Errorf("after bulk update, value = %q, want 198.51.100.200", m.Value)
		}
	}
}

func TestHKM_ConfigsBulkUpdate_PreservesScope(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "bulk-update-scope", map[string]string{}, nil)

	w := postJSON(handler, "/api/agents/"+agentHash+"/configs", map[string]string{
		"key":   "REMOTE_ENDPOINT",
		"value": "old",
		"scope": "EXTERNAL",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("seed config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "REMOTE_ENDPOINT",
		"new_value": "new",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-update: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	get := getJSON(handler, "/api/agents/"+agentHash+"/configs/REMOTE_ENDPOINT")
	if get.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var payload struct {
		Scope  string `json:"scope"`
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal get config: %v", err)
	}
	if payload.Scope != "EXTERNAL" {
		t.Fatalf("scope = %q, want EXTERNAL", payload.Scope)
	}
	if payload.Status != "active" {
		t.Fatalf("status = %q, want active", payload.Status)
	}
	if payload.Value != "new" {
		t.Fatalf("value = %q, want new", payload.Value)
	}
}

func TestHKM_ConfigsBulkSet_PreservesExistingScope(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "bulk-set-scope", map[string]string{}, nil)

	w := postJSON(handler, "/api/agents/"+agentHash+"/configs", map[string]string{
		"key":   "BOOTSTRAP_URL",
		"value": "old",
		"scope": "TEMP",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("seed config: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = postJSON(handler, "/api/configs/bulk-set", map[string]string{
		"key":   "BOOTSTRAP_URL",
		"value": "new",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-set: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	get := getJSON(handler, "/api/agents/"+agentHash+"/configs/BOOTSTRAP_URL")
	if get.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var payload struct {
		Scope  string `json:"scope"`
		Status string `json:"status"`
		Value  string `json:"value"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal get config: %v", err)
	}
	if payload.Scope != "TEMP" {
		t.Fatalf("scope = %q, want TEMP", payload.Scope)
	}
	if payload.Status != "temp" {
		t.Fatalf("status = %q, want temp", payload.Status)
	}
	if payload.Value != "new" {
		t.Fatalf("value = %q, want new", payload.Value)
	}
}

// === Test: Bulk Update — Multiple Values (old_value required) ===

func TestHKM_ConfigsBulkUpdate_MultipleValues_RequiresOldValue(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "svc-x", map[string]string{"DB_HOST": "198.51.100.12"}, nil) // prod
	registerMockAgent(t, srv, "svc-y", map[string]string{"DB_HOST": "198.51.100.13"}, nil) // dev

	// Without old_value → should return 409 Conflict
	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "DB_HOST",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
	var conflictResp struct {
		UniqueValues int            `json:"unique_values"`
		ValueSummary map[string]int `json:"value_summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &conflictResp); err != nil {
		t.Fatalf("unmarshal conflict: %v", err)
	}
	if conflictResp.UniqueValues != 2 {
		t.Errorf("unique_values = %d, want 2", conflictResp.UniqueValues)
	}

	// With old_value → only update matching agents
	w = postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "DB_HOST",
		"old_value": "198.51.100.12",
		"new_value": "198.51.100.200",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("bulk-update with old_value: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Updated int `json:"updated"`
		Skipped int `json:"skipped"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal bulk-update: %v", err)
	}
	if resp.Updated != 1 {
		t.Errorf("updated = %d, want 1", resp.Updated)
	}
	if resp.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", resp.Skipped)
	}
}

// === Test: Secrets and Configs Are Completely Separate ===

func TestHKM_SecretsAndConfigsSeparation(t *testing.T) {
	srv, handler := setupHKMServer(t)

	// Agent has both secrets and configs with the SAME key name
	configs := map[string]string{"DB_PASSWORD": "this-is-config-not-secret"}
	secrets := map[string]string{"DB_PASSWORD": "ref12345"}
	_, agentHash := registerMockAgent(t, srv, "separation-test", configs, secrets)

	// Get config → returns plaintext config value
	w := getJSON(handler, "/api/agents/"+agentHash+"/configs/DB_PASSWORD")
	if w.Code != http.StatusOK {
		t.Fatalf("get config: expected 200, got %d", w.Code)
	}
	var configResp struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &configResp); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if configResp.Value != "this-is-config-not-secret" {
		t.Errorf("config value = %q, want 'this-is-config-not-secret'", configResp.Value)
	}

	// Configs endpoint never exposes secret data
	w = getJSON(handler, "/api/agents/"+agentHash+"/configs")
	var listResp struct {
		Configs []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"configs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal config list: %v", err)
	}

	for _, c := range listResp.Configs {
		if c.Value == "ref12345" {
			t.Error("config list should never contain secret ref values")
		}
	}
}

// === Test: Config Key Not Found Returns 404 ===

func TestHKM_AgentConfigNotFound(t *testing.T) {
	srv, handler := setupHKMServer(t)

	_, agentHash := registerMockAgent(t, srv, "notfound-test", map[string]string{"DOMAIN": "test.kr"}, nil)

	w := getJSON(handler, "/api/agents/"+agentHash+"/configs/NONEXISTENT")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// === Test: Bulk Update Key Not Found ===

func TestHKM_ConfigsBulkUpdate_KeyNotFound(t *testing.T) {
	srv, handler := setupHKMServer(t)

	registerMockAgent(t, srv, "empty-agent", map[string]string{"DOMAIN": "x.kr"}, nil)

	w := postJSON(handler, "/api/configs/bulk-update", map[string]string{
		"key":       "NONEXISTENT_KEY",
		"new_value": "anything",
	})
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
