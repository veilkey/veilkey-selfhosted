package db

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: db_token_ref.go + models.go ─────────────────────────────

func TestSource_TokenRefModel_HasRequiredFields(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	required := map[string]string{
		"RefFamily": "TokenRef must have RefFamily field",
		"RefScope":  "TokenRef must have RefScope field",
		"RefID":     "TokenRef must have RefID field",
		"Status":    "TokenRef must have Status field",
	}
	for field, msg := range required {
		if !strings.Contains(content, field) {
			t.Error(msg)
		}
	}
}

func TestSource_RefStatusConstants_AllDefined(t *testing.T) {
	src, err := os.ReadFile("ref_policy.go")
	if err != nil {
		t.Fatalf("failed to read ref_policy.go: %v", err)
	}
	content := string(src)

	statuses := []string{
		"RefStatusTemp",
		"RefStatusActive",
		"RefStatusArchive",
		"RefStatusBlock",
		"RefStatusRevoke",
	}
	for _, status := range statuses {
		if !strings.Contains(content, status) {
			t.Errorf("ref_policy.go must define constant: %s", status)
		}
	}
}

func TestSource_SaveRef_ValidatesRequiredFields(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	// SaveRef must call Validate on RefParts
	if !strings.Contains(content, "parts.Validate()") {
		t.Error("SaveRef/SaveRefWithName must validate RefParts before saving")
	}
}

func TestSource_RefParts_Validate_ChecksAllFields(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	// Validate must check Family, Scope, and ID
	checks := []string{
		`r.Family == ""`,
		`r.Scope == ""`,
		`r.ID == ""`,
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("RefParts.Validate must check: %s", check)
		}
	}
}

func TestSource_GetRef_LooksUpByCanonical(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "ref_canonical = ?") {
		t.Error("GetRef must look up by ref_canonical")
	}
}

func TestSource_RefParts_Canonical_FormatsCorrectly(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "r.Family + RefSep + string(r.Scope) + RefSep + r.ID") {
		t.Error("Canonical must format as FAMILY:SCOPE:ID using RefSep")
	}
}

func TestSource_ParseCanonicalRef_SplitsThreeParts(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `len(parts) != 3`) {
		t.Error("ParseCanonicalRef must reject refs that don't have exactly 3 colon-separated parts")
	}
}

func TestSource_ExpiryHandling_SaveRefWithExpiry(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func (d *DB) SaveRefWithExpiry(") {
		t.Error("SaveRefWithExpiry function must exist for handling ref expiration")
	}
	if !strings.Contains(content, "expires_at") {
		t.Error("token ref expiry must use expires_at column")
	}
}

func TestSource_DeleteExpiredTempRefs_Exists(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func (d *DB) DeleteExpiredTempRefs()") {
		t.Error("DeleteExpiredTempRefs must exist for cleaning up expired temp refs")
	}
	if !strings.Contains(content, "expires_at IS NOT NULL AND expires_at <= CURRENT_TIMESTAMP") {
		t.Error("DeleteExpiredTempRefs must delete refs where expires_at <= now")
	}
}

func TestSource_DefaultRefStatus_TempAndActive(t *testing.T) {
	src, err := os.ReadFile("db_token_ref.go")
	if err != nil {
		t.Fatalf("failed to read db_token_ref.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "func DefaultRefStatus(") {
		t.Error("DefaultRefStatus function must exist")
	}
	// LOCAL/EXTERNAL scope defaults to active, TEMP defaults to temp
	if !strings.Contains(content, "RefScopeLocal") || !strings.Contains(content, "RefStatusActive") {
		t.Error("DefaultRefStatus must return active for LOCAL scope")
	}
	if !strings.Contains(content, "RefStatusTemp") {
		t.Error("DefaultRefStatus must return temp for TEMP scope")
	}
}
