package hkm

import (
	"os"
	"strings"
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Source-level tests for handleMaskMap VE entry construction.
// Verifies invariants: deduplication, active-only filtering,
// VE ref format, and VK/VE separation.
// ══════════════════════════════════════════════════════════════════

func readMaskMapSource(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("hkm_mask_map.go")
	if err != nil {
		t.Fatalf("failed to read hkm_mask_map.go: %v", err)
	}
	return string(src)
}

// --- VE deduplication ---

// Guarantees: VE entries are deduplicated by value to prevent repeated
// tagging of the same config value across multiple vaults.
func TestSource_MaskMap_VE_DeduplicatesByValue(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "veSeenValues") {
		t.Error("must use veSeenValues map for VE deduplication")
	}
	// Check the dedup guard inside the config loop
	if !strings.Contains(src, "veSeenValues[cfg.Value]") {
		t.Error("must check veSeenValues before adding VE entry")
	}
	if !strings.Contains(src, `veSeenValues[cfg.Value] = true`) {
		t.Error("must mark VE value as seen after adding")
	}
}

// --- VE skips VK secret values ---

// Guarantees: VE entries skip values that already appear as VK secrets.
// Without this, a config value like "10.0.0.5" that also happens to be
// a secret value would be double-masked.
func TestSource_MaskMap_VE_SkipsVKSecretValues(t *testing.T) {
	src := readMaskMapSource(t)
	// veSeenValues must be seeded with existing secret values
	if !strings.Contains(src, "for _, e := range entries") {
		t.Error("must seed veSeenValues with existing secret entry values")
	}
	if !strings.Contains(src, "veSeenValues[e.Value] = true") {
		t.Error("must mark secret values as seen before processing VE configs")
	}
}

// --- VE only includes active configs ---

// Guarantees: Only configs with Status == "active" are included.
// Deleted or disabled configs must not appear in the mask map.
func TestSource_MaskMap_VE_FiltersActiveOnly(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, `cfg.Status != "active"`) {
		t.Error("must filter configs by Status == active")
	}
}

// --- VE skips empty values ---

// Guarantees: Configs with empty Value are skipped.
func TestSource_MaskMap_VE_SkipsEmptyValue(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, `cfg.Value == ""`) {
		t.Error("must skip configs with empty Value")
	}
}

// --- VE ref format ---

// Guarantees: VE references follow the format "VE:<scope>:<key>".
func TestSource_MaskMap_VE_RefFormat(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, `"VE:" + cfg.Scope + ":" + cfg.Key`) {
		t.Error("VE ref must be constructed as VE:<scope>:<key>")
	}
}

// --- VE uses agent VaultName ---

// Guarantees: Each VE entry records the vault name of its source agent.
func TestSource_MaskMap_VE_RecordsVaultName(t *testing.T) {
	src := readMaskMapSource(t)
	// Check that VE maskEntry uses agent.VaultName
	if !strings.Contains(src, "agent.VaultName") {
		t.Error("VE entries must record agent.VaultName")
	}
}

// --- VE skips agents without IP ---

// Guarantees: Agents with empty IP are skipped when fetching configs,
// because we can't reach their /api/configs endpoint.
func TestSource_MaskMap_VE_SkipsAgentWithoutIP(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, `agent.IP == ""`) {
		t.Error("must skip agents with empty IP for VE config fetch")
	}
}

// --- Config fetch failure is graceful ---

// Guarantees: If fetching /api/configs from an agent fails,
// the handler continues to the next agent without crashing.
func TestSource_MaskMap_VE_ConfigFetchFailureGraceful(t *testing.T) {
	src := readMaskMapSource(t)
	// After configErr, must continue (not return/panic)
	if !strings.Contains(src, "configErr != nil") {
		t.Error("must handle config fetch error")
	}
	// Check that the error handling continues rather than returns
	configErrIdx := strings.Index(src, "configErr != nil")
	if configErrIdx == -1 {
		t.Fatal("configErr check not found")
	}
	afterErr := src[configErrIdx : configErrIdx+100]
	if !strings.Contains(afterErr, "continue") {
		t.Error("config fetch error must continue to next agent, not return")
	}
}

// --- JSON decode failure is graceful ---

// Guarantees: If the config JSON response is malformed, the handler
// continues without adding any VE entries for that agent.
func TestSource_MaskMap_VE_JSONDecodeFailureGraceful(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "json.NewDecoder") {
		t.Error("must use json.NewDecoder for config response")
	}
	// The decode is inside an if-err-nil guard
	if !strings.Contains(src, "Decode(&configData); err == nil") {
		t.Error("must guard JSON decode with err == nil check")
	}
}

// --- Response body is always closed ---

// Guarantees: configResp.Body is closed after reading, preventing
// connection leaks even on decode failure.
func TestSource_MaskMap_VE_ClosesResponseBody(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "configResp.Body.Close()") {
		t.Error("must close config response body to prevent connection leaks")
	}
}

// --- VE entries mixed with VK in single response ---

// Guarantees: The response uses a single "entries" array containing
// both VK secrets and VE configs (no separate ve_entries field).
func TestSource_MaskMap_SingleEntriesArray(t *testing.T) {
	src := readMaskMapSource(t)
	// Must NOT have a separate "ve_entries" field in response
	if strings.Contains(src, `"ve_entries"`) {
		t.Error("response must NOT have separate ve_entries field — all entries in one array")
	}
	// VE entries are appended to the same entries slice
	if !strings.Contains(src, "entries = append(entries, maskEntry{") {
		t.Error("VE entries must be appended to same entries slice as VK secrets")
	}
}

// --- Long-poll version tracking ---

// Guarantees: The mask-map endpoint supports long polling via version parameter.
func TestSource_MaskMap_LongPollSupport(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "clientVersion") {
		t.Error("must accept client version for long polling")
	}
	if !strings.Contains(src, "MaskMapWait") {
		t.Error("must wait on MaskMapWait channel for long polling")
	}
	if !strings.Contains(src, `"changed"`) {
		t.Error("response must include changed field for long polling")
	}
}

// --- Response includes count ---

// Guarantees: The response includes a count field matching entries length.
func TestSource_MaskMap_ResponseIncludesCount(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, `"count"`) {
		t.Error("response must include count field")
	}
	if !strings.Contains(src, "len(entries)") {
		t.Error("count must equal len(entries)")
	}
}

// --- Agent auth for config fetch ---

// Guarantees: Config fetch requests include agent auth headers.
func TestSource_MaskMap_VE_ConfigFetchAuthenticated(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "setAgentAuthHeader") {
		t.Error("config fetch must include agent auth headers")
	}
}

// --- SSH refs also included ---

// Guarantees: SSH keys stored on VaultCenter are included in the mask map.
func TestSource_MaskMap_IncludesSSHRefs(t *testing.T) {
	src := readMaskMapSource(t)
	if !strings.Contains(src, "RefScopeSSH") {
		t.Error("must include SSH scope refs in mask map")
	}
	if !strings.Contains(src, `"vaultcenter"`) {
		t.Error("SSH refs must be attributed to vaultcenter vault")
	}
}

// --- Trusted IP only ---

// Guarantees: The mask-map endpoint is protected by trusted IP filter.
func TestSource_MaskMap_TrustedIPOnly(t *testing.T) {
	handlerSrc, err := os.ReadFile("handler.go")
	if err != nil {
		t.Fatalf("failed to read handler.go: %v", err)
	}
	content := string(handlerSrc)
	if !strings.Contains(content, `trusted(ready(h.handleMaskMap))`) {
		t.Error("mask-map endpoint must be wrapped in trusted() middleware")
	}
}
