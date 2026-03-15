package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestInstallSessionRoundTripOnLockedServer(t *testing.T) {
	_, handler, _ := setupServerWithPassword(t, "install-pass")

	create := postJSON(handler, "/api/install/session", map[string]interface{}{
		"language":         "ko",
		"quickstart":       true,
		"flow":             "quickstart",
		"deployment_mode":  "docker",
		"install_scope":    "host+keycenter",
		"bootstrap_mode":   "email",
		"mail_transport":   "smtp",
		"planned_stages":   []string{"language", "deployment_mode", "final_smoke"},
		"completed_stages": []string{"language"},
		"last_stage":       "language",
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create install session: expected 201, got %d: %s", create.Code, create.Body.String())
	}

	var created struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if created.Session.SessionID == "" {
		t.Fatalf("expected session_id to be generated")
	}
	if len(created.Session.PlannedStages) != 3 {
		t.Fatalf("planned_stages length = %d, want 3", len(created.Session.PlannedStages))
	}

	get := getJSON(handler, "/api/install/state?session_id="+created.Session.SessionID)
	if get.Code != http.StatusOK {
		t.Fatalf("get install state: expected 200, got %d: %s", get.Code, get.Body.String())
	}
	var loaded struct {
		Exists  bool                `json:"exists"`
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(get.Body.Bytes(), &loaded); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if !loaded.Exists {
		t.Fatalf("expected exists=true")
	}
	if loaded.Session.DeploymentMode != "docker" {
		t.Fatalf("deployment_mode = %q, want docker", loaded.Session.DeploymentMode)
	}
	if loaded.Session.PlannedStages[2] != "final_smoke" {
		t.Fatalf("planned_stages[2] = %q, want final_smoke", loaded.Session.PlannedStages[2])
	}

	patch := patchJSON(handler, "/api/install/state", map[string]interface{}{
		"session_id":       created.Session.SessionID,
		"completed_stages": []string{"language", "deployment_mode"},
		"last_stage":       "deployment_mode",
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch install state: expected 200, got %d: %s", patch.Code, patch.Body.String())
	}

	getLatest := getJSON(handler, "/api/install/state")
	if getLatest.Code != http.StatusOK {
		t.Fatalf("get latest install state: expected 200, got %d: %s", getLatest.Code, getLatest.Body.String())
	}
	var latest struct {
		Exists  bool                `json:"exists"`
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(getLatest.Body.Bytes(), &latest); err != nil {
		t.Fatalf("unmarshal latest response: %v", err)
	}
	if latest.Session.LastStage != "deployment_mode" {
		t.Fatalf("last_stage = %q, want deployment_mode", latest.Session.LastStage)
	}
	if len(latest.Session.CompletedStages) != 2 {
		t.Fatalf("completed_stages length = %d, want 2", len(latest.Session.CompletedStages))
	}
}

func TestInstallSessionWriteRespectsTrustedIP(t *testing.T) {
	_, handler := setupTrustedIPServer(t, []string{"10.0.0.100"})

	blocked := postJSONFromIP(handler, "/api/install/session", "192.168.1.50:1234", map[string]interface{}{
		"language": "ko",
	})
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("blocked create: expected 403, got %d: %s", blocked.Code, blocked.Body.String())
	}

	allowed := postJSONFromIP(handler, "/api/install/session", "10.0.0.100:1234", map[string]interface{}{
		"language": "ko",
	})
	if allowed.Code != http.StatusCreated {
		t.Fatalf("allowed create: expected 201, got %d: %s", allowed.Code, allowed.Body.String())
	}

	var created struct {
		Session installStatePayload `json:"session"`
	}
	if err := json.Unmarshal(allowed.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal allowed create: %v", err)
	}

	getBlocked := getJSONFromIP(handler, "/api/install/state?session_id="+created.Session.SessionID, "192.168.1.50:1234")
	if getBlocked.Code != http.StatusForbidden {
		t.Fatalf("blocked get: expected 403, got %d: %s", getBlocked.Code, getBlocked.Body.String())
	}

	patchBlocked := patchJSONFromIP(handler, "/api/install/state", "192.168.1.50:1234", map[string]interface{}{
		"session_id": created.Session.SessionID,
		"last_stage": "bootstrap_mode",
	})
	if patchBlocked.Code != http.StatusForbidden {
		t.Fatalf("blocked patch: expected 403, got %d: %s", patchBlocked.Code, patchBlocked.Body.String())
	}
}
