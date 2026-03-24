package db

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: db_agent.go ─────────────────────────────────────────────

func TestSource_AgentModel_HasDEKFields(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "DEK ") {
		t.Error("Agent model must have DEK field for data encryption key storage")
	}
	if !strings.Contains(content, "DEKNonce") {
		t.Error("Agent model must have DEKNonce field for DEK nonce storage")
	}
}

func TestSource_AgentModel_HasAgentHashAndNodeID(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "AgentHash") {
		t.Error("Agent model must have AgentHash field")
	}
	if !strings.Contains(content, "NodeID") {
		t.Error("Agent model must have NodeID field")
	}
}

func TestSource_DeleteAgent_UsesSoftDelete(t *testing.T) {
	src, err := os.ReadFile("db_agent.go")
	if err != nil {
		t.Fatalf("failed to read db_agent.go: %v", err)
	}
	content := string(src)

	// DeleteAgentByNodeID must use soft delete (set deleted_at), not hard delete
	if !strings.Contains(content, `Update("deleted_at"`) {
		t.Error("DeleteAgentByNodeID must soft-delete by setting deleted_at, not hard delete")
	}
	// Must NOT use d.conn.Delete for agent removal
	if strings.Contains(content, `d.conn.Delete(&Agent{}`) {
		t.Error("Agent deletion must use soft delete (deleted_at), not GORM hard Delete")
	}
}

func TestSource_RestoreAgent_ClearsDeletedAt(t *testing.T) {
	src, err := os.ReadFile("db_agent.go")
	if err != nil {
		t.Fatalf("failed to read db_agent.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func (d *DB) RestoreDeletedAgent(") {
		t.Error("RestoreDeletedAgent function must exist")
	}
	if !strings.Contains(content, `Update("deleted_at", nil)`) {
		t.Error("RestoreDeletedAgent must clear deleted_at by setting it to nil")
	}
}

func TestSource_ListAgents_ExcludesDeleted(t *testing.T) {
	src, err := os.ReadFile("db_agent.go")
	if err != nil {
		t.Fatalf("failed to read db_agent.go: %v", err)
	}
	content := string(src)

	// Find the ListAgents function and verify it filters deleted_at
	funcStart := strings.Index(content, "func (d *DB) ListAgents()")
	if funcStart == -1 {
		t.Fatal("ListAgents function must exist")
	}
	// Get the function body (until next func or end)
	funcBody := content[funcStart:]
	nextFunc := strings.Index(funcBody[1:], "\nfunc ")
	if nextFunc > 0 {
		funcBody = funcBody[:nextFunc+1]
	}
	if !strings.Contains(funcBody, "deleted_at IS NULL") {
		t.Error("ListAgents must filter by deleted_at IS NULL to exclude soft-deleted agents")
	}
}

func TestSource_ListAgents_ExcludesArchived(t *testing.T) {
	src, err := os.ReadFile("db_agent.go")
	if err != nil {
		t.Fatalf("failed to read db_agent.go: %v", err)
	}
	content := string(src)

	funcStart := strings.Index(content, "func (d *DB) ListAgents()")
	if funcStart == -1 {
		t.Fatal("ListAgents function must exist")
	}
	funcBody := content[funcStart:]
	nextFunc := strings.Index(funcBody[1:], "\nfunc ")
	if nextFunc > 0 {
		funcBody = funcBody[:nextFunc+1]
	}
	if !strings.Contains(funcBody, "archived_at IS NULL") {
		t.Error("ListAgents must filter by archived_at IS NULL to exclude archived agents")
	}
}

func TestSource_ListAgentsIncludeArchived_StillExcludesDeleted(t *testing.T) {
	src, err := os.ReadFile("db_agent.go")
	if err != nil {
		t.Fatalf("failed to read db_agent.go: %v", err)
	}
	content := string(src)

	funcStart := strings.Index(content, "func (d *DB) ListAgentsIncludeArchived()")
	if funcStart == -1 {
		t.Fatal("ListAgentsIncludeArchived function must exist")
	}
	funcBody := content[funcStart:]
	nextFunc := strings.Index(funcBody[1:], "\nfunc ")
	if nextFunc > 0 {
		funcBody = funcBody[:nextFunc+1]
	}
	if !strings.Contains(funcBody, "deleted_at IS NULL") {
		t.Error("ListAgentsIncludeArchived must still exclude soft-deleted agents")
	}
}

func TestSource_AgentModel_HasDeletedAtField(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `DeletedAt`) {
		t.Error("Agent model must have DeletedAt field for soft delete support")
	}
}
