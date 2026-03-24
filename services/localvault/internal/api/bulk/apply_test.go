package bulk

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: apply.go ────────────────────────────────────────────────

func TestSource_AllowedTargetPaths_Whitelist(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "defaultBulkApplyTargets") {
		t.Error("apply.go must define defaultBulkApplyTargets allowlist")
	}
	expectedPaths := []string{
		"/opt/mattermost/config/config.json",
		"/opt/mattermost/.env",
		"/etc/systemd/system/mattermost.service.d/override.conf",
		"/etc/gitlab/gitlab.rb",
	}
	for _, path := range expectedPaths {
		if !strings.Contains(content, path) {
			t.Errorf("defaultBulkApplyTargets must include: %s", path)
		}
	}
}

func TestSource_RejectedPaths_Relative(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "filepath.IsAbs") {
		t.Error("path validation must reject non-absolute paths using filepath.IsAbs")
	}
}

func TestSource_RejectedPaths_DotDot(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `".."`) {
		t.Error("path validation must reject paths containing '..' traversal")
	}
}

func TestSource_IsAllowedBulkApplyTarget_ChecksBothMaps(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func isAllowedBulkApplyTarget(") {
		t.Error("isAllowedBulkApplyTarget function must exist")
	}
	if !strings.Contains(content, "defaultBulkApplyTargets") {
		t.Error("isAllowedBulkApplyTarget must check defaultBulkApplyTargets")
	}
	if !strings.Contains(content, "extraBulkApplyTargets") {
		t.Error("isAllowedBulkApplyTarget must check extraBulkApplyTargets for env-configured paths")
	}
}

func TestSource_AllowedHooks_Whitelist(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "defaultBulkApplyHooks") {
		t.Error("apply.go must define defaultBulkApplyHooks allowlist")
	}
	expectedHooks := []string{
		"reload_systemd",
		"restart_mattermost",
		"reconfigure_gitlab",
	}
	for _, hook := range expectedHooks {
		if !strings.Contains(content, hook) {
			t.Errorf("defaultBulkApplyHooks must include: %s", hook)
		}
	}
}

func TestSource_GetAllowedBulkApplyHook_ChecksBothMaps(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func getAllowedBulkApplyHook(") {
		t.Error("getAllowedBulkApplyHook function must exist")
	}
	if !strings.Contains(content, "defaultBulkApplyHooks") {
		t.Error("getAllowedBulkApplyHook must check defaultBulkApplyHooks")
	}
	if !strings.Contains(content, "extraBulkApplyHooks") {
		t.Error("getAllowedBulkApplyHook must check extraBulkApplyHooks for env-configured hooks")
	}
}

func TestSource_WriteAtomically_TempFileCleanup(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func writeAtomically(") {
		t.Error("writeAtomically function must exist")
	}
	if !strings.Contains(content, "os.CreateTemp(") {
		t.Error("writeAtomically must use os.CreateTemp for atomic writes")
	}
	if !strings.Contains(content, "os.Remove(tmpName)") {
		t.Error("writeAtomically must clean up temp file in defer")
	}
	if !strings.Contains(content, "os.Rename(tmpName, path)") {
		t.Error("writeAtomically must use os.Rename for atomic replacement")
	}
}

func TestSource_WriteAtomically_PreservesPermissions(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "os.Chmod(tmpName, mode)") {
		t.Error("writeAtomically must preserve file permissions via os.Chmod")
	}
	if !strings.Contains(content, "os.Chown(tmpName, uid, gid)") {
		t.Error("writeAtomically must preserve file ownership via os.Chown")
	}
}

func TestSource_RecursiveJSONMerge_DeepNesting(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func recursiveJSONMerge(") {
		t.Error("recursiveJSONMerge function must exist for deep JSON merge")
	}
	// Must handle nested maps recursively
	if !strings.Contains(content, "srcIsMap && dstIsMap") {
		t.Error("recursiveJSONMerge must detect and handle nested map types")
	}
	// Must recurse into nested maps
	if !strings.Contains(content, "recursiveJSONMerge(dstMap, srcMap)") {
		t.Error("recursiveJSONMerge must recurse into nested maps")
	}
}

func TestSource_RecursiveJSONMerge_TypeHandling(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	// When src value is not a map, it should overwrite dst
	if !strings.Contains(content, "dst[key] = srcValue") {
		t.Error("recursiveJSONMerge must overwrite non-map values from src")
	}
}

func TestSource_Postcheck_MattermostConfigValidation(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "mattermost_config_required_keys") {
		t.Error("postchecks must include mattermost_config_required_keys check")
	}
	if !strings.Contains(content, "ServiceSettings") {
		t.Error("mattermost postcheck must verify ServiceSettings section")
	}
	if !strings.Contains(content, "SqlSettings") {
		t.Error("mattermost postcheck must verify SqlSettings section")
	}
	if !strings.Contains(content, "SiteURL") {
		t.Error("mattermost postcheck must verify SiteURL is present")
	}
	if !strings.Contains(content, "DataSource") {
		t.Error("mattermost postcheck must verify DataSource is present")
	}
}

func TestSource_ValidateBulkApplyStep_RejectsTempRefs(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `VK:TEMP:`) {
		t.Error("validateBulkApplyStep must reject content containing VK:TEMP references")
	}
}

func TestSource_BulkApplyRoutes_RegisteredWithMiddleware(t *testing.T) {
	src, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(src)

	routes := []string{
		"/api/bulk-apply/precheck",
		"/api/bulk-apply/execute",
	}
	for _, route := range routes {
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(line, route) {
				found = true
				if !strings.Contains(line, "trusted(") || !strings.Contains(line, "ready(") {
					t.Errorf("bulk-apply route %s must use trusted + ready middleware", route)
				}
				break
			}
		}
		if !found {
			t.Errorf("bulk-apply route not registered: %s", route)
		}
	}
}

func TestSource_ExtraAllowedPaths_FromEnv(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "VEILKEY_BULK_APPLY_ALLOWED_PATHS") {
		t.Error("extra allowed paths must be loaded from VEILKEY_BULK_APPLY_ALLOWED_PATHS env var")
	}
	if !strings.Contains(content, "VEILKEY_BULK_APPLY_ALLOWED_HOOKS") {
		t.Error("extra allowed hooks must be loaded from VEILKEY_BULK_APPLY_ALLOWED_HOOKS env var")
	}
}

func TestSource_SystemdOverride_PostcheckExists(t *testing.T) {
	src, err := os.ReadFile("apply.go")
	if err != nil {
		t.Fatalf("failed to read apply.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "systemd_override_parse") {
		t.Error("postchecks must include systemd_override_parse for override.conf validation")
	}
	if !strings.Contains(content, "[Service]") {
		t.Error("systemd override postcheck must verify [Service] section exists")
	}
}
