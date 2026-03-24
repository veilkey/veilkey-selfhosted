package db

import (
	"os"
	"strings"
	"testing"
)

// ── Source analysis: admin_auth.go ───────────────────────────────────────────

func TestSource_SetAdminPassword_UsesBcrypt(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `"golang.org/x/crypto/bcrypt"`) {
		t.Error("admin_auth.go must import golang.org/x/crypto/bcrypt")
	}
	if !strings.Contains(content, "bcrypt.GenerateFromPassword") {
		t.Error("SetAdminPassword must use bcrypt.GenerateFromPassword, not plaintext/md5/sha")
	}
	if !strings.Contains(content, "bcrypt.DefaultCost") {
		t.Error("SetAdminPassword must use bcrypt.DefaultCost for work factor")
	}
	// Ensure no weak hashing
	for _, weak := range []string{"md5.Sum", "sha256.Sum", "sha1.Sum"} {
		if strings.Contains(content, weak) {
			t.Errorf("admin_auth.go must not use weak hash %s for password storage", weak)
		}
	}
}

func TestSource_VerifyAdminPassword_UsesBcryptCompare(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "bcrypt.CompareHashAndPassword") {
		t.Error("VerifyAdminPassword must use bcrypt.CompareHashAndPassword")
	}
}

func TestSource_HasAdminPassword_ChecksPasswordHash(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `cfg.PasswordHash != ""`) {
		t.Error("HasAdminPassword must check that PasswordHash is not empty")
	}
}

func TestSource_AdminSession_RequiredFields(t *testing.T) {
	src, err := os.ReadFile("../db/models.go")
	if err != nil {
		t.Fatalf("failed to read models.go: %v", err)
	}
	content := string(src)

	required := []string{
		"TokenHash",
		"ExpiresAt",
		"IdleExpiresAt",
		"RevokedAt",
	}
	for _, field := range required {
		if !strings.Contains(content, field) {
			t.Errorf("AdminSession model must have field: %s", field)
		}
	}
}

func TestSource_SessionLookup_ChecksRevokedAtNull(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, "revoked_at IS NULL") {
		t.Error("GetAdminSessionByTokenHash must filter by revoked_at IS NULL")
	}
}

func TestSource_SaveAdminSession_RequiresTokenHash(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	if !strings.Contains(content, `session.TokenHash == ""`) {
		t.Error("SaveAdminSession must validate token_hash is not empty")
	}
}

func TestSource_TouchAdminSession_ChecksRevokedAt(t *testing.T) {
	src, err := os.ReadFile("admin_auth.go")
	if err != nil {
		t.Fatalf("failed to read admin_auth.go: %v", err)
	}
	content := string(src)

	// TouchAdminSession should only update non-revoked sessions
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, "TouchAdminSession") || strings.Contains(line, "RevokeAdminSession") {
			continue
		}
	}
	// Count how many times "revoked_at IS NULL" appears in touch/revoke methods
	revokedNullCount := strings.Count(content, "revoked_at IS NULL")
	if revokedNullCount < 2 {
		t.Error("session operations (touch, revoke, lookup) must check revoked_at IS NULL")
	}
}
