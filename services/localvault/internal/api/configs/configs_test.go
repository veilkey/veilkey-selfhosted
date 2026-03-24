package configs

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: handler.go — route registration ──────────────────────────

func TestSource_GETRoutesWrappedWithTrusted(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	getRoutes := []string{
		"GET /api/configs",
		"GET /api/configs/{key}",
	}
	for _, route := range getRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "trusted(") {
					t.Errorf("GET route %s must be wrapped with trusted()", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("GET route not registered: %s", route)
		}
	}
}

func TestSource_WriteRoutesWrappedWithTrusted(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	writeRoutes := []string{
		"POST /api/configs",
		"PUT /api/configs/bulk",
		"DELETE /api/configs/{key}",
	}
	for _, route := range writeRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "trusted(") {
					t.Errorf("write route %s must be wrapped with trusted()", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("write route not registered: %s", route)
		}
	}
}

// ── Source analysis: configs.go — config key validation ───────────────────────

func TestSource_ConfigKeyValidation(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	// All CRUD handlers must validate keys
	handlers := []string{
		"func (h *Handler) handleGetConfig(",
		"func (h *Handler) handleSaveConfig(",
		"func (h *Handler) handleDeleteConfig(",
	}
	for _, handler := range handlers {
		fnBody := extractFn(content, handler)
		if !strings.Contains(fnBody, "isValidResourceName") {
			t.Errorf("handler %s must validate key with isValidResourceName", handler)
		}
	}
}

func TestSource_ConfigKeyValidationMessage(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `[A-Z_][A-Z0-9_]*`) {
		t.Error("config key validation error must describe the expected pattern [A-Z_][A-Z0-9_]*")
	}
}

// ── Source analysis: configs.go — bulk operations ─────────────────────────────

func TestSource_BulkOperationsMaxLimit(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveConfigsBulk(")
	if !strings.Contains(fnBody, "MaxBulkItems") {
		t.Error("bulk save must enforce MaxBulkItems limit")
	}
}

func TestSource_BulkOperationsValidatesAllKeys(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveConfigsBulk(")
	if !strings.Contains(fnBody, "isValidResourceName(k)") {
		t.Error("bulk save must validate every key with isValidResourceName")
	}
}

func TestSource_BulkOperationsRequiresNonEmpty(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleSaveConfigsBulk(")
	if !strings.Contains(fnBody, `len(req.Configs) == 0`) {
		t.Error("bulk save must reject empty configs map")
	}
}

// ── Source analysis: configs.go — response format ─────────────────────────────

func TestSource_ListConfigsResponseFormat(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleListConfigs(")
	for _, field := range []string{`"configs"`, `"count"`} {
		if !strings.Contains(fnBody, field) {
			t.Errorf("list configs response must include field: %s", field)
		}
	}
}

func TestSource_GetConfigResponseFormat(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleGetConfig(")
	for _, field := range []string{`"key"`, `"value"`, `"ref"`, `"scope"`, `"status"`} {
		if !strings.Contains(fnBody, field) {
			t.Errorf("get config response must include field: %s", field)
		}
	}
}

// ── Source analysis: constants.go — ref constants ─────────────────────────────

func TestSource_RefConstants(t *testing.T) {
	src, err := os.ReadFile("constants.go")
	if err != nil {
		t.Fatalf("failed to read constants.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "refFamilyVE") {
		t.Error("configs package must define refFamilyVE constant")
	}
	if !strings.Contains(content, "refScopeLocal") {
		t.Error("configs package must define refScopeLocal constant")
	}
	if !strings.Contains(content, "refStatusActive") {
		t.Error("configs package must define refStatusActive constant")
	}
	if !strings.Contains(content, "refStatusBlock") {
		t.Error("configs package must define refStatusBlock constant")
	}
}

// ── Source analysis: configs.go — blocked config returns 423 ──────────────────

func TestSource_BlockedConfigReturnsLocked(t *testing.T) {
	src, err := os.ReadFile("configs.go")
	if err != nil {
		t.Fatalf("failed to read configs.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) handleGetConfig(")
	if !strings.Contains(fnBody, "refStatusBlock") {
		t.Error("handleGetConfig must check for blocked status")
	}
	if !strings.Contains(fnBody, "StatusLocked") {
		t.Error("handleGetConfig must return StatusLocked (423) for blocked configs")
	}
}

// ── Helper ────────────────────────────────────────────────────────────────────

func extractFn(code, sig string) string {
	i := strings.Index(code, sig)
	if i < 0 {
		return ""
	}
	rest := code[i:]
	next := strings.Index(rest[1:], "\nfunc ")
	if next < 0 {
		return rest
	}
	return rest[:next+1]
}
