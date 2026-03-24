package bulk

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: files.go — template format allowlist ─────────────────────

func TestSource_TemplateFormatAllowlist(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "allowedBulkApplyFormatsFile") {
		t.Error("bulk apply templates must define allowedBulkApplyFormatsFile")
	}
	for _, format := range []string{`"env"`, `"json"`, `"json_merge"`, `"line_patch"`, `"raw"`} {
		if !strings.Contains(content, format) {
			t.Errorf("allowed formats must include: %s", format)
		}
	}
}

func TestSource_TemplateFormatValidated(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `allowedBulkApplyFormatsFile[format]`) {
		t.Error("template normalization must validate format against allowlist")
	}
}

func TestUnit_AllowedFormatsMap(t *testing.T) {
	expected := []string{"env", "json", "json_merge", "line_patch", "raw"}
	for _, format := range expected {
		if _, ok := allowedBulkApplyFormatsFile[format]; !ok {
			t.Errorf("allowedBulkApplyFormatsFile must include %q", format)
		}
	}
	if len(allowedBulkApplyFormatsFile) != len(expected) {
		t.Errorf("allowedBulkApplyFormatsFile has %d entries, want %d", len(allowedBulkApplyFormatsFile), len(expected))
	}
}

// ── Source analysis: workflows.go — workflow step validation ──────────────────

func TestSource_WorkflowStepValidation(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func (h *Handler) loadBulkApplyWorkflowFile(")
	if !strings.Contains(fnBody, `len(workflow.Steps) == 0`) {
		t.Error("workflow validation must check for at least one step")
	}
	if !strings.Contains(fnBody, `step.Template`) {
		t.Error("workflow validation must check each step has a template reference")
	}
}

func TestSource_WorkflowStepSummaryHasPrechecks(t *testing.T) {
	src, err := os.ReadFile("workflows.go")
	if err != nil {
		t.Fatalf("failed to read workflows.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func bulkApplyWorkflowStepSummaryFromTemplate(")
	if !strings.Contains(fnBody, "ensure_target_parent_exists") {
		t.Error("workflow step must have ensure_target_parent_exists precheck")
	}
	if !strings.Contains(fnBody, "ensure_target_parent_writable") {
		t.Error("workflow step must have ensure_target_parent_writable precheck")
	}
}

func TestSource_WorkflowStepPostchecksPerFormat(t *testing.T) {
	src, err := os.ReadFile("workflows.go")
	if err != nil {
		t.Fatalf("failed to read workflows.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func bulkApplyWorkflowStepSummaryFromTemplate(")
	checks := map[string]string{
		`"json"`:       "json_parse",
		`"json_merge"`: "json_merge_verify",
		`"line_patch"`: "line_patch_verify",
	}
	for format, postcheck := range checks {
		if !strings.Contains(fnBody, format) {
			t.Errorf("workflow postchecks must handle format: %s", format)
		}
		if !strings.Contains(fnBody, postcheck) {
			t.Errorf("format %s must produce postcheck: %s", format, postcheck)
		}
	}
}

// ── Source analysis: files.go — atomic write pattern ──────────────────────────

func TestSource_AtomicWritePattern(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `func atomicWriteFile(`) {
		t.Error("atomicWriteFile function must exist")
	}
	fnBody := extractFn(content, "func atomicWriteFile(")
	// Must use temp file + rename pattern
	if !strings.Contains(fnBody, `.tmp-`) {
		t.Error("atomicWriteFile must create a temp file with .tmp- suffix")
	}
	if !strings.Contains(fnBody, `os.WriteFile(tmp`) {
		t.Error("atomicWriteFile must write to temp file first")
	}
	if !strings.Contains(fnBody, `os.Rename(tmp`) {
		t.Error("atomicWriteFile must rename temp file to final path")
	}
}

func TestSource_AtomicWritePreservesPermission(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func atomicWriteFile(")
	if !strings.Contains(fnBody, "mode os.FileMode") {
		t.Error("atomicWriteFile must accept file mode parameter")
	}
	if !strings.Contains(fnBody, "os.WriteFile(tmp, content, mode)") {
		t.Error("atomicWriteFile must apply mode to the temp file")
	}
}

func TestSource_AtomicWriteCreatesDirIfNeeded(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	fnBody := extractFn(content, "func atomicWriteFile(")
	if !strings.Contains(fnBody, "os.MkdirAll(filepath.Dir(path)") {
		t.Error("atomicWriteFile must create parent directories if needed")
	}
}

// ── Source analysis: handler.go — route registration ──────────────────────────

func TestSource_BulkHandler_WriteRoutesRequireTrustedIP(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	writeRoutes := []string{
		"POST /api/vaults/{vault}/bulk-apply/templates",
		"PUT /api/vaults/{vault}/bulk-apply/templates/{name}",
		"DELETE /api/vaults/{vault}/bulk-apply/templates/{name}",
		"POST /api/vaults/{vault}/bulk-apply/workflows/{name}/precheck",
		"POST /api/vaults/{vault}/bulk-apply/workflows/{name}/run",
	}
	for _, route := range writeRoutes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "requireTrustedIP") {
					t.Errorf("write route %s must be wrapped with requireTrustedIP", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("write route not registered: %s", route)
		}
	}
}

func TestSource_BulkHandler_ReadRoutesExist(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	readRoutes := []string{
		"GET /api/vaults/{vault}/bulk-apply/templates",
		"GET /api/vaults/{vault}/bulk-apply/templates/{name}",
		"GET /api/vaults/{vault}/bulk-apply/workflows",
		"GET /api/vaults/{vault}/bulk-apply/workflows/{name}",
	}
	for _, route := range readRoutes {
		if !strings.Contains(content, route) {
			t.Errorf("read route not registered: %s", route)
		}
	}
}

// ── Source analysis: files.go — template kind constant ────────────────────────

func TestSource_TemplateKindConstant(t *testing.T) {
	src, err := os.ReadFile("files.go")
	if err != nil {
		t.Fatalf("failed to read files.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `bulkApplyTemplateKind = "BulkApplyTemplate"`) {
		t.Error("bulkApplyTemplateKind must be BulkApplyTemplate")
	}
	if !strings.Contains(content, `bulkApplyWorkflowKind = "BulkApplyWorkflow"`) {
		t.Error("bulkApplyWorkflowKind must be BulkApplyWorkflow")
	}
}

// ── Source analysis: templates.go — sensitive value masking ───────────────────

func TestSource_SensitiveValueMasking(t *testing.T) {
	src, err := os.ReadFile("templates.go")
	if err != nil {
		t.Fatalf("failed to read templates.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func isSensitiveBulkApplyValue(") {
		t.Error("isSensitiveBulkApplyValue function must exist")
	}
	if !strings.Contains(content, "func maskBulkApplyValue(") {
		t.Error("maskBulkApplyValue function must exist")
	}
}

func TestUnit_IsSensitiveBulkApplyValue(t *testing.T) {
	tests := []struct {
		kind string
		name string
		want bool
	}{
		{"VK", "DB_PASSWORD", true},
		{"VK", "API_SECRET", true},
		{"VK", "AUTH_TOKEN", true},
		{"VK", "PRIVATE_KEY", true},
		{"VK", "APP_ENDPOINT", false},
		{"VK", "BASE_URL", false},
		{"VE", "DB_PASSWORD", false}, // VE family is never sensitive
		{"VK", "CLIENT_ID", false},
	}
	for _, tt := range tests {
		got := isSensitiveBulkApplyValue(tt.kind, tt.name)
		if got != tt.want {
			t.Errorf("isSensitiveBulkApplyValue(%q, %q) = %v, want %v", tt.kind, tt.name, got, tt.want)
		}
	}
}

func TestUnit_MaskBulkApplyValue(t *testing.T) {
	if got := maskBulkApplyValue("VK", "DB_PASSWORD", "supersecret"); got != "***" {
		t.Errorf("sensitive value should be masked, got %q", got)
	}
	if got := maskBulkApplyValue("VK", "APP_ENDPOINT", "https://example.com"); got != "https://example.com" {
		t.Errorf("non-sensitive value should not be masked, got %q", got)
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
