package api

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: lifecycle.go ─────────────────────────────────────────────

func TestSource_Activation_RequiresTempScope(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "parsed.Scope != RefScopeTemp") {
		t.Error("handleActivate must require TEMP scope for activation source")
	}
	if !strings.Contains(content, `ciphertext must use TEMP scope`) {
		t.Error("handleActivate must return error message about TEMP scope requirement")
	}
}

func TestSource_Activation_TargetScopeLocalOrExternal(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ParseActivationScope") {
		t.Error("handleActivate must call ParseActivationScope to validate target scope")
	}
}

func TestSource_ParseActivationScope_OnlyLocalOrExternal(t *testing.T) {
	src, err := os.ReadFile("vkref.go")
	if err != nil {
		t.Fatalf("failed to read vkref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func ParseActivationScope(") {
		t.Fatal("ParseActivationScope must exist")
	}
	if !strings.Contains(content, "RefScopeLocal") {
		t.Error("ParseActivationScope must accept LOCAL scope")
	}
	if !strings.Contains(content, "RefScopeExternal") {
		t.Error("ParseActivationScope must accept EXTERNAL scope")
	}
	if !strings.Contains(content, `scope must be LOCAL or EXTERNAL`) {
		t.Error("ParseActivationScope must reject non-LOCAL/EXTERNAL scopes")
	}
}

func TestSource_StatusTransition_ArchiveBlockRevoke_SetStatus(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	// Each handler delegates to handleStatusTransition with the correct status
	transitions := map[string]string{
		"handleArchive": "RefStatusArchive",
		"handleBlock":   "RefStatusBlock",
		"handleRevoke":  "RefStatusRevoke",
	}
	for handler, status := range transitions {
		if !strings.Contains(content, handler) {
			t.Errorf("lifecycle.go must define %s", handler)
		}
		if !strings.Contains(content, status) {
			t.Errorf("lifecycle.go must reference %s for status transition", status)
		}
	}
}

func TestSource_BlockStatusCheck_RejectsBlockedRefs(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	// Both activate and status transition must check for blocked refs
	if strings.Count(content, "RefStatusBlock") < 3 {
		t.Error("lifecycle handlers must check RefStatusBlock in both activate and status transition paths")
	}
	if !strings.Contains(content, "http.StatusLocked") {
		t.Error("blocked refs must return HTTP 423 (Locked)")
	}
	if !strings.Contains(content, "ref is blocked") {
		t.Error("blocked refs must return 'ref is blocked' error message")
	}
}

func TestSource_StatusTransition_RejectsTempScope(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "parsed.Scope == RefScopeTemp") {
		t.Error("handleStatusTransition must reject TEMP scope refs (must use LOCAL or EXTERNAL)")
	}
}

func TestSource_Lifecycle_SyncsTrackedRefWithVaultcenter(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "syncTrackedRefWithVaultcenter") {
		t.Error("lifecycle transitions must sync tracked ref with VaultCenter")
	}
}

func TestSource_Lifecycle_AllHandlersExist(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	handlers := []string{
		"func (s *Server) handleReencrypt(",
		"func (s *Server) handleActivate(",
		"func (s *Server) handleArchive(",
		"func (s *Server) handleBlock(",
		"func (s *Server) handleRevoke(",
	}
	for _, h := range handlers {
		if !strings.Contains(content, h) {
			t.Errorf("lifecycle handler missing: %s", h)
		}
	}
}

func TestSource_Lifecycle_RoutesRegistered(t *testing.T) {
	src, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("failed to read api.go: %v", err)
	}
	content := string(src)

	routes := []struct {
		path    string
		handler string
	}{
		{"/api/activate", "handleActivate"},
		{"/api/archive", "handleArchive"},
		{"/api/block", "handleBlock"},
		{"/api/revoke", "handleRevoke"},
	}
	for _, route := range routes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route.handler) && strings.Contains(line, route.path) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("route %s not registered with handler %s", route.path, route.handler)
		}
	}
}

func TestSource_Lifecycle_SupportsBothVKAndVE(t *testing.T) {
	src, err := os.ReadFile("lifecycle.go")
	if err != nil {
		t.Fatalf("failed to read lifecycle.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "RefFamilyVK") {
		t.Error("lifecycle handlers must support VK (secret) family")
	}
	if !strings.Contains(content, "RefFamilyVE") {
		t.Error("lifecycle handlers must support VE (config) family")
	}
}
