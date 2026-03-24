package api

import (
	"os"
	"strings"
	"testing"
)

// ── Bug 6: Promote must reject deleted agents ────────────────────────────────

func TestPromoteRejectsDeletedAgent(t *testing.T) {
	src, err := os.ReadFile("handle_keycenter.go")
	if err != nil {
		t.Fatalf("failed to read handle_keycenter.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (s *Server) handleKeycenterPromoteToVault(")
	if fnBody == "" {
		t.Fatal("handleKeycenterPromoteToVault must exist")
	}

	// After FindAgentRecord, must check if agent is deleted
	findIdx := strings.Index(fnBody, "FindAgentRecord")
	if findIdx < 0 {
		t.Fatal("handleKeycenterPromoteToVault must call FindAgentRecord")
	}
	afterFind := fnBody[findIdx:]

	if !strings.Contains(afterFind, "DeletedAt") {
		t.Error("handleKeycenterPromoteToVault must check agent.DeletedAt after FindAgentRecord")
	}

	// Must reject deleted agents
	if !strings.Contains(afterFind, "deleted") || !strings.Contains(afterFind, "agent") {
		t.Error("handleKeycenterPromoteToVault must reject promoting to a deleted agent")
	}
}
