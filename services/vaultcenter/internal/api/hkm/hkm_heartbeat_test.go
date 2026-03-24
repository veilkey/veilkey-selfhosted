package hkm

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: hkm_agent_heartbeat.go ──────────────────────────────────

func TestSource_Heartbeat_NewAgentRequiresRegistrationTokenOrTrustedIP(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	// New agent registration requires token or trusted IP
	if !strings.Contains(content, "RegistrationToken") {
		t.Error("heartbeat must accept registration_token field for new agent registration")
	}
	if !strings.Contains(content, "IsTrustedIPString") {
		t.Error("heartbeat must check trusted IP as alternative to registration token")
	}
	if !strings.Contains(content, "registration_token is required for first-time agent registration") {
		t.Error("heartbeat must reject new agents without registration_token and non-trusted IP")
	}
}

func TestSource_Heartbeat_ConsumeRegistrationToken(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ConsumeRegistrationToken") {
		t.Error("heartbeat must consume registration token atomically to prevent race conditions")
	}
}

func TestSource_Heartbeat_DeletedAgentRestoredOnReconnect(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "agent.DeletedAt != nil") {
		t.Error("heartbeat must check if existing agent is soft-deleted")
	}
	if !strings.Contains(content, "RestoreDeletedAgent") {
		t.Error("heartbeat must restore soft-deleted agent on reconnection")
	}
}

func TestSource_Heartbeat_KeyVersionMismatchDetection(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "agent.KeyVersion != req.KeyVersion") {
		t.Error("heartbeat must detect key version mismatch between server and agent")
	}
	if !strings.Contains(content, "key_version_mismatch") {
		t.Error("heartbeat must report key_version_mismatch status")
	}
	if !strings.Contains(content, "expected_key_version") {
		t.Error("heartbeat must include expected_key_version in mismatch response")
	}
	if !strings.Contains(content, "provided_key_version") {
		t.Error("heartbeat must include provided_key_version in mismatch response")
	}
}

func TestSource_Heartbeat_BlockedAgentReturns423(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "agent.BlockedAt != nil") {
		t.Error("heartbeat must check if agent is blocked")
	}
	if !strings.Contains(content, "http.StatusLocked") {
		t.Error("heartbeat must return HTTP 423 (Locked) for blocked agents")
	}
	if !strings.Contains(content, `"blocked"`) {
		t.Error("heartbeat must return 'blocked' status for blocked agents")
	}
}

func TestSource_HeartbeatResponse_IncludesExpectedFields(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	fields := []string{
		`"status"`,
		`"vault_id"`,
		`"managed_paths"`,
		`"key_version"`,
	}
	for _, field := range fields {
		if !strings.Contains(content, field) {
			t.Errorf("heartbeat response must include field: %s", field)
		}
	}
}

func TestSource_HeartbeatAgentState_IncludesRotationAndRebindFields(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	stateFields := []string{
		`"rotation_required"`,
		`"rebind_required"`,
		`"retry_stage"`,
	}
	for _, field := range stateFields {
		if !strings.Contains(content, field) {
			t.Errorf("heartbeatAgentState must include field: %s", field)
		}
	}
}

func TestSource_Heartbeat_SetNodeAndRuntimeAliases(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "setNodeIdentityAliases") {
		t.Error("heartbeat must set node identity aliases (vault_node_uuid, node_id)")
	}
	if !strings.Contains(content, "setRuntimeHashAliases") {
		t.Error("heartbeat must set runtime hash aliases (vault_runtime_hash, agent_hash)")
	}
}

func TestSource_Heartbeat_RotationRequired_ClearsOnMatch(t *testing.T) {
	src, err := os.ReadFile("hkm_agent_heartbeat.go")
	if err != nil {
		t.Fatalf("failed to read hkm_agent_heartbeat.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "agent.RotationRequired") {
		t.Error("heartbeat must check RotationRequired flag")
	}
	if !strings.Contains(content, "clearRotationPayload") {
		t.Error("heartbeat must clear rotation state when key versions match")
	}
}
