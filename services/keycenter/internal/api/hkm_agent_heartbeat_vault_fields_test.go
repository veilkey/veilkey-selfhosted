package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentHeartbeatRequiresVaultFields(t *testing.T) {
	_, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id": "node-1",
		"label":   "agent-1",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAgentHeartbeatStoresVaultIdentityAndKeyVersion(t *testing.T) {
	_, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-1",
		"label":         "agent-1",
		"vault_hash":    "93a8094e",
		"vault_name":    "proxmox-test-lab-veilkey",
		"managed_paths": []string{"/var/www/services/demo", "/var/www/services/demo", "relative/path"},
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = getJSON(handler, "/api/agents")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Agents []struct {
			NodeID           string   `json:"node_id"`
			VaultNodeUUID    string   `json:"vault_node_uuid"`
			VaultRuntimeHash string   `json:"vault_runtime_hash"`
			AgentHash        string   `json:"agent_hash"`
			VaultHash        string   `json:"vault_hash"`
			VaultName        string   `json:"vault_name"`
			VaultID          string   `json:"vault_id"`
			ManagedPaths     []string `json:"managed_paths"`
			KeyVersion       int      `json:"key_version"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents len = %d", len(resp.Agents))
	}
	agent := resp.Agents[0]
	if agent.VaultNodeUUID != agent.NodeID {
		t.Fatalf("vault node uuid aliases = %q / %q", agent.VaultNodeUUID, agent.NodeID)
	}
	if agent.VaultRuntimeHash == "" || agent.AgentHash != agent.VaultRuntimeHash {
		t.Fatalf("runtime hash aliases = %q / %q", agent.VaultRuntimeHash, agent.AgentHash)
	}
	if agent.VaultHash != "93a8094e" {
		t.Fatalf("vault_hash = %q", agent.VaultHash)
	}
	if agent.VaultName != "proxmox-test-lab-veilkey" {
		t.Fatalf("vault_name = %q", agent.VaultName)
	}
	if agent.VaultID != "proxmox-test-lab-veilkey:93a8094e" {
		t.Fatalf("vault_id = %q", agent.VaultID)
	}
	if len(agent.ManagedPaths) != 1 || agent.ManagedPaths[0] != "/var/www/services/demo" {
		t.Fatalf("managed_paths = %#v", agent.ManagedPaths)
	}
	if agent.KeyVersion != 7 {
		t.Fatalf("key_version = %d", agent.KeyVersion)
	}
}

func TestAgentHeartbeatPrefersForwardedIPWhenRequestPassesThroughProxy(t *testing.T) {
	_, handler := setupHKMServer(t)

	body := strings.NewReader(`{
		"node_id":"node-forwarded",
		"label":"agent-forwarded",
		"vault_hash":"feedbeef",
		"vault_name":"forwarded-vault",
		"key_version":1,
		"port":10180,
		"secrets_count":1,
		"configs_count":2,
		"version":1
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/heartbeat", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.0.0.60, 10.0.0.61")
	req.Header.Set("X-Real-IP", "10.0.0.60")
	req.RemoteAddr = "127.0.0.1:54321"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	list := getJSON(handler, "/api/agents")
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}

	var resp struct {
		Agents []struct {
			NodeID string `json:"node_id"`
			IP     string `json:"ip"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal agents list: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents len = %d", len(resp.Agents))
	}
	if resp.Agents[0].NodeID != "node-forwarded" {
		t.Fatalf("node_id = %q", resp.Agents[0].NodeID)
	}
	if resp.Agents[0].IP != "10.0.0.60" {
		t.Fatalf("ip = %q, want 10.0.0.60", resp.Agents[0].IP)
	}
}

func TestAgentHeartbeatIgnoresForwardedIPFromUntrustedClient(t *testing.T) {
	_, handler := setupHKMServer(t)

	body := strings.NewReader(`{
		"node_id":"node-untrusted-forwarded",
		"label":"agent-untrusted-forwarded",
		"vault_hash":"beadfeed",
		"vault_name":"untrusted-forwarded-vault",
		"key_version":1,
		"port":10180,
		"secrets_count":1,
		"configs_count":2,
		"version":1
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/heartbeat", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Forwarded-For", "10.0.0.60, 10.0.0.61")
	req.Header.Set("X-Real-IP", "10.0.0.60")
	req.RemoteAddr = "198.51.100.55:54321"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	list := getJSON(handler, "/api/agents")
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", list.Code, list.Body.String())
	}

	var resp struct {
		Agents []struct {
			NodeID string `json:"node_id"`
			IP     string `json:"ip"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal agents list: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("agents len = %d", len(resp.Agents))
	}
	if resp.Agents[0].NodeID != "node-untrusted-forwarded" {
		t.Fatalf("node_id = %q", resp.Agents[0].NodeID)
	}
	if resp.Agents[0].IP != "198.51.100.55" {
		t.Fatalf("ip = %q, want 198.51.100.55", resp.Agents[0].IP)
	}
}

func TestAgentHeartbeatReturnsManagedPathsInResponse(t *testing.T) {
	_, handler := setupHKMServer(t)

	w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-2",
		"label":         "agent-2",
		"vault_hash":    "12ab34cd",
		"vault_name":    "project-local-vault",
		"managed_paths": []string{"/Users/demo/work/app", "/Users/demo/work/app/sub"},
		"key_version":   1,
		"ip":            "10.0.0.2",
		"port":          10180,
		"secrets_count": 0,
		"configs_count": 0,
		"version":       1,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		ManagedPaths []string `json:"managed_paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.ManagedPaths) != 2 {
		t.Fatalf("managed_paths len = %d", len(resp.ManagedPaths))
	}
	if resp.ManagedPaths[0] != "/Users/demo/work/app" || resp.ManagedPaths[1] != "/Users/demo/work/app/sub" {
		t.Fatalf("managed_paths = %#v", resp.ManagedPaths)
	}
}

func TestAgentHeartbeatRejectsKeyVersionMismatch(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-1",
		"label":         "agent-1",
		"vault_hash":    "93a8094e",
		"vault_name":    "proxmox-test-lab-veilkey",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	second := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-1",
		"label":         "agent-1",
		"vault_hash":    "93a8094e",
		"vault_name":    "proxmox-test-lab-veilkey",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("second heartbeat expected 409, got %d: %s", second.Code, second.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal conflict response: %v", err)
	}
	if resp["status"] != "key_version_mismatch" {
		t.Fatalf("status = %#v", resp["status"])
	}
	if resp["rebind_required"] != true {
		t.Fatalf("rebind_required = %#v", resp["rebind_required"])
	}
	if resp["retry_stage"] != float64(1) {
		t.Fatalf("retry_stage = %#v", resp["retry_stage"])
	}
}

func TestAgentHeartbeatAllowsDuplicateManagedPathOwnershipAcrossAgents(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-a",
		"label":         "agent-a",
		"vault_hash":    "11111111",
		"vault_name":    "service-a",
		"managed_paths": []string{"/var/www/html"},
		"key_version":   1,
		"ip":            "10.0.0.10",
		"port":          10180,
		"secrets_count": 0,
		"configs_count": 0,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	second := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-b",
		"label":         "agent-b",
		"vault_hash":    "22222222",
		"vault_name":    "service-b",
		"managed_paths": []string{"/var/www/html"},
		"key_version":   1,
		"ip":            "10.0.0.11",
		"port":          10180,
		"secrets_count": 0,
		"configs_count": 0,
		"version":       1,
	})
	if second.Code != http.StatusOK {
		t.Fatalf("second heartbeat expected 200, got %d: %s", second.Code, second.Body.String())
	}
}

func TestAgentHeartbeatAllowsOverlappingManagedPathOwnershipAcrossAgents(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-a",
		"label":         "agent-a",
		"vault_hash":    "11111111",
		"vault_name":    "service-a",
		"managed_paths": []string{"/var/www"},
		"key_version":   1,
		"ip":            "10.0.0.10",
		"port":          10180,
		"secrets_count": 0,
		"configs_count": 0,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	second := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-b",
		"label":         "agent-b",
		"vault_hash":    "22222222",
		"vault_name":    "service-b",
		"managed_paths": []string{"/var/www/html"},
		"key_version":   1,
		"ip":            "10.0.0.11",
		"port":          10180,
		"secrets_count": 0,
		"configs_count": 0,
		"version":       1,
	})
	if second.Code != http.StatusOK {
		t.Fatalf("second heartbeat expected 200, got %d: %s", second.Code, second.Body.String())
	}
}

func TestAgentHeartbeatBlocksAfterRepeatedRebindAttempts(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-rebind",
		"label":         "agent-rebind",
		"vault_hash":    "93a8094e",
		"vault_name":    "rebind-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	for i := 0; i < 3; i++ {
		w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
			"node_id":       "node-rebind",
			"label":         "agent-rebind",
			"vault_hash":    "93a8094e",
			"vault_name":    "rebind-vault",
			"key_version":   8,
			"ip":            "10.0.0.50",
			"port":          10180,
			"secrets_count": 2,
			"configs_count": 3,
			"version":       1,
		})
		if w.Code != http.StatusConflict {
			t.Fatalf("attempt %d expected 409, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	blocked := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-rebind",
		"label":         "agent-rebind",
		"vault_hash":    "93a8094e",
		"vault_name":    "rebind-vault",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if blocked.Code != http.StatusLocked {
		t.Fatalf("blocked heartbeat expected 423, got %d: %s", blocked.Code, blocked.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(blocked.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal blocked response: %v", err)
	}
	if resp["status"] != "blocked" {
		t.Fatalf("status = %#v", resp["status"])
	}

	list := getJSON(handler, "/api/agents")
	if list.Code != http.StatusOK {
		t.Fatalf("agents list expected 200, got %d: %s", list.Code, list.Body.String())
	}
	var listResp struct {
		Agents []struct {
			Status      string `json:"status"`
			Blocked     bool   `json:"blocked"`
			RetryStage  int    `json:"retry_stage"`
			NextRetryAt string `json:"next_retry_at"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.Agents) != 1 {
		t.Fatalf("agents len = %d", len(listResp.Agents))
	}
	if !listResp.Agents[0].Blocked || listResp.Agents[0].Status != "blocked" {
		t.Fatalf("blocked list entry = %+v", listResp.Agents[0])
	}
}

func TestAgentHeartbeatReportsNextRetryTimestamp(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-retry",
		"label":         "agent-retry",
		"vault_hash":    "93a8094e",
		"vault_name":    "retry-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	second := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-retry",
		"label":         "agent-retry",
		"vault_hash":    "93a8094e",
		"vault_name":    "retry-vault",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if second.Code != http.StatusConflict {
		t.Fatalf("second heartbeat expected 409, got %d: %s", second.Code, second.Body.String())
	}

	var resp struct {
		NextRetryAt       string `json:"next_retry_at"`
		RetryAfterSeconds int    `json:"retry_after_seconds"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal retry response: %v", err)
	}
	if resp.NextRetryAt == "" {
		t.Fatal("next_retry_at should not be empty")
	}
	if resp.RetryAfterSeconds < 0 || resp.RetryAfterSeconds > int(time.Minute.Seconds()) {
		t.Fatalf("retry_after_seconds = %d", resp.RetryAfterSeconds)
	}
}

func TestAgentHeartbeatSupportsPlannedRotation(t *testing.T) {
	srv, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-rotate",
		"label":         "agent-rotate",
		"vault_hash":    "93a8094e",
		"vault_name":    "rotate-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	rotate := postJSON(handler, "/api/agents/rotate-all", map[string]any{})
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate all expected 200, got %d: %s", rotate.Code, rotate.Body.String())
	}
	auditRows, err := srv.db.ListAuditEvents("vault", "node-rotate")
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	if len(auditRows) == 0 || auditRows[0].Action != "schedule_rotation" {
		t.Fatalf("expected schedule_rotation audit, got %+v", auditRows)
	}
	if auditRows[0].ActorID == "" {
		t.Fatalf("expected operator actor id in audit row, got %+v", auditRows[0])
	}

	mismatch := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-rotate",
		"label":         "agent-rotate",
		"vault_hash":    "93a8094e",
		"vault_name":    "rotate-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("mismatch heartbeat expected 409, got %d: %s", mismatch.Code, mismatch.Body.String())
	}
	var mismatchResp map[string]any
	if err := json.Unmarshal(mismatch.Body.Bytes(), &mismatchResp); err != nil {
		t.Fatalf("unmarshal mismatch response: %v", err)
	}
	if mismatchResp["status"] != "rotation_required" {
		t.Fatalf("status = %#v", mismatchResp["status"])
	}
	if mismatchResp["rotation_required"] != true {
		t.Fatalf("rotation_required = %#v", mismatchResp["rotation_required"])
	}
	if mismatchResp["expected_key_version"] != float64(8) {
		t.Fatalf("expected_key_version = %#v", mismatchResp["expected_key_version"])
	}

	updated := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-rotate",
		"label":         "agent-rotate",
		"vault_hash":    "93a8094e",
		"vault_name":    "rotate-vault",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("updated heartbeat expected 200, got %d: %s", updated.Code, updated.Body.String())
	}
}

func TestRotateAllBlocksUnresponsivePlannedRotationAfterRetries(t *testing.T) {
	srv, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-stale-rotate",
		"label":         "agent-stale-rotate",
		"vault_hash":    "93a8094e",
		"vault_name":    "stale-rotate-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	rotate := postJSON(handler, "/api/agents/rotate-all", map[string]any{})
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate all expected 200, got %d: %s", rotate.Code, rotate.Body.String())
	}

	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		if _, err := srv.db.AdvancePendingRotations(now.Add(time.Hour * time.Duration(i+1))); err != nil {
			t.Fatalf("AdvancePendingRotations %d: %v", i+1, err)
		}
	}

	agent, err := srv.db.GetAgentByNodeID("node-stale-rotate")
	if err != nil {
		t.Fatalf("GetAgentByNodeID: %v", err)
	}
	if agent.BlockedAt == nil {
		t.Fatal("blocked_at should be set after exhausting rotation retries")
	}
	if agent.BlockReason != "rotation_timeout" {
		t.Fatalf("block_reason = %q", agent.BlockReason)
	}
	if agent.RotationRequired {
		t.Fatal("rotation_required should be cleared after block")
	}
}

func TestAgentListAdvancesPendingRotationsToBlocked(t *testing.T) {
	srv, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-list-rotate",
		"label":         "agent-list-rotate",
		"vault_hash":    "1199aa22",
		"vault_name":    "list-rotate-vault",
		"key_version":   3,
		"ip":            "10.0.0.51",
		"port":          10180,
		"secrets_count": 1,
		"configs_count": 1,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	rotate := postJSON(handler, "/api/agents/rotate-all", map[string]any{})
	if rotate.Code != http.StatusOK {
		t.Fatalf("rotate all expected 200, got %d: %s", rotate.Code, rotate.Body.String())
	}

	past := time.Now().UTC().Add(-time.Hour)
	if err := srv.db.UpdateAgentRotationState("node-list-rotate", 3, &past, true, "planned_rotation", nil, ""); err != nil {
		t.Fatalf("prepare stale rotation state: %v", err)
	}

	list := getJSON(handler, "/api/agents")
	if list.Code != http.StatusOK {
		t.Fatalf("list expected 200, got %d: %s", list.Code, list.Body.String())
	}

	agent, err := srv.db.GetAgentByNodeID("node-list-rotate")
	if err != nil {
		t.Fatalf("GetAgentByNodeID: %v", err)
	}
	if agent.BlockedAt == nil {
		t.Fatal("blocked_at should be set after list-triggered pending rotation advance")
	}
	if agent.BlockReason != "rotation_timeout" {
		t.Fatalf("block_reason = %q", agent.BlockReason)
	}
	if agent.RotationRequired {
		t.Fatal("rotation_required should be cleared after list-triggered block")
	}
}

func TestHeartbeatClearsKeyVersionMismatchRebindWhenVersionMatches(t *testing.T) {
	_, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-clear-rebind",
		"label":         "agent-clear-rebind",
		"vault_hash":    "93a8094e",
		"vault_name":    "clear-rebind-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	mismatch := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-clear-rebind",
		"label":         "agent-clear-rebind",
		"vault_hash":    "93a8094e",
		"vault_name":    "clear-rebind-vault",
		"key_version":   8,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("mismatch heartbeat expected 409, got %d: %s", mismatch.Code, mismatch.Body.String())
	}

	recovered := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-clear-rebind",
		"label":         "agent-clear-rebind",
		"vault_hash":    "93a8094e",
		"vault_name":    "clear-rebind-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered heartbeat expected 200, got %d: %s", recovered.Code, recovered.Body.String())
	}
}

func TestHeartbeatClearsBlockedKeyVersionMismatchWhenVersionMatches(t *testing.T) {
	srv, handler := setupHKMServer(t)

	first := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-clear-block",
		"label":         "agent-clear-block",
		"vault_hash":    "93a8094e",
		"vault_name":    "clear-block-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if first.Code != http.StatusOK {
		t.Fatalf("first heartbeat expected 200, got %d: %s", first.Code, first.Body.String())
	}

	for i := 0; i < 4; i++ {
		w := postJSON(handler, "/api/agents/heartbeat", map[string]any{
			"node_id":       "node-clear-block",
			"label":         "agent-clear-block",
			"vault_hash":    "93a8094e",
			"vault_name":    "clear-block-vault",
			"key_version":   8,
			"ip":            "10.0.0.50",
			"port":          10180,
			"secrets_count": 2,
			"configs_count": 3,
			"version":       1,
		})
		if i < 3 && w.Code != http.StatusConflict {
			t.Fatalf("attempt %d expected 409, got %d: %s", i+1, w.Code, w.Body.String())
		}
		if i == 3 && w.Code != http.StatusLocked {
			t.Fatalf("attempt %d expected 423, got %d: %s", i+1, w.Code, w.Body.String())
		}
	}

	recovered := postJSON(handler, "/api/agents/heartbeat", map[string]any{
		"node_id":       "node-clear-block",
		"label":         "agent-clear-block",
		"vault_hash":    "93a8094e",
		"vault_name":    "clear-block-vault",
		"key_version":   7,
		"ip":            "10.0.0.50",
		"port":          10180,
		"secrets_count": 2,
		"configs_count": 3,
		"version":       1,
	})
	if recovered.Code != http.StatusOK {
		t.Fatalf("recovered heartbeat expected 200, got %d: %s", recovered.Code, recovered.Body.String())
	}

	agent, err := srv.db.GetAgentByNodeID("node-clear-block")
	if err != nil {
		t.Fatalf("GetAgentByNodeID: %v", err)
	}
	if agent.BlockedAt != nil || agent.RebindRequired {
		t.Fatalf("agent should be cleared, got blocked_at=%v rebind_required=%v", agent.BlockedAt, agent.RebindRequired)
	}
}
