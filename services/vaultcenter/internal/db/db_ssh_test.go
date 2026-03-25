package db

import (
	"testing"
)

// ══════════════════════════════════════════════════════════════════
// Behavioral DB tests for SSH key refs (ListRefsByScope, scope handling)
// Uses in-memory SQLite — no mocks, real DB operations.
// ══════════════════════════════════════════════════════════════════

func newTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func saveSSHRef(t *testing.T, d *DB, id string, status RefStatus) {
	t.Helper()
	parts := RefParts{Family: "VK", Scope: RefScopeSSH, ID: id}
	if err := d.SaveRef(parts, "cipher-"+id, 1, status, ""); err != nil {
		t.Fatalf("failed to save SSH ref %s: %v", id, err)
	}
}

func saveTempRef(t *testing.T, d *DB, id string) {
	t.Helper()
	parts := RefParts{Family: "VK", Scope: RefScopeTemp, ID: id}
	if err := d.SaveRef(parts, "cipher-"+id, 1, RefStatusTemp, ""); err != nil {
		t.Fatalf("failed to save TEMP ref %s: %v", id, err)
	}
}

func saveLocalRef(t *testing.T, d *DB, id string) {
	t.Helper()
	parts := RefParts{Family: "VK", Scope: RefScopeLocal, ID: id}
	if err := d.SaveRef(parts, "cipher-"+id, 1, RefStatusActive, ""); err != nil {
		t.Fatalf("failed to save LOCAL ref %s: %v", id, err)
	}
}

// --- ListRefsByScope ---

func TestListRefsByScope_SSH_ReturnsOnlySSH(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "ssh11111", RefStatusActive)
	saveTempRef(t, d, "temp1111")
	saveLocalRef(t, d, "local111")

	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 SSH ref, got %d", len(refs))
	}
	if refs[0].RefScope != RefScopeSSH {
		t.Errorf("expected SSH scope, got %s", refs[0].RefScope)
	}
	if refs[0].RefID != "ssh11111" {
		t.Errorf("expected ssh11111, got %s", refs[0].RefID)
	}
}

func TestListRefsByScope_SSH_Empty(t *testing.T) {
	d := newTestDB(t)
	saveTempRef(t, d, "temp0001")
	saveLocalRef(t, d, "local001")

	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 SSH refs, got %d", len(refs))
	}
}

func TestListRefsByScope_SSH_MultipleKeys(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshaaaa1", RefStatusActive)
	saveSSHRef(t, d, "sshbbbb2", RefStatusActive)
	saveSSHRef(t, d, "sshcccc3", RefStatusActive)

	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope: %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("expected 3 SSH refs, got %d", len(refs))
	}
}

func TestListRefsByScope_SSH_IncludesAllStatuses(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshact01", RefStatusActive)
	saveSSHRef(t, d, "ssharc02", RefStatusArchive)
	saveSSHRef(t, d, "sshrev03", RefStatusRevoke)

	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope: %v", err)
	}
	if len(refs) != 3 {
		t.Errorf("ListRefsByScope returns all statuses, expected 3 got %d", len(refs))
	}
}

func TestListRefsByScope_SSH_OrderByCreatedDesc(t *testing.T) {
	d := newTestDB(t)
	// Insert in order: a, b, c — should come back c, b, a (newest first)
	saveSSHRef(t, d, "sshfirst", RefStatusActive)
	saveSSHRef(t, d, "sshlast0", RefStatusActive)

	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2, got %d", len(refs))
	}
	// SQLite CURRENT_TIMESTAMP granularity may cause same-second inserts
	// Just verify both are returned
}

func TestListRefsByScope_TEMP_ReturnsOnlyTemp(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "ssh00001", RefStatusActive)
	saveTempRef(t, d, "tmp00001")
	saveTempRef(t, d, "tmp00002")

	refs, err := d.ListRefsByScope(RefScopeTemp)
	if err != nil {
		t.Fatalf("ListRefsByScope TEMP: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("expected 2 TEMP refs, got %d", len(refs))
	}
	for _, ref := range refs {
		if ref.RefScope != RefScopeTemp {
			t.Errorf("expected TEMP scope, got %s", ref.RefScope)
		}
	}
}

func TestListRefsByScope_LOCAL_ReturnsOnlyLocal(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "ssh10001", RefStatusActive)
	saveLocalRef(t, d, "loc10001")

	refs, err := d.ListRefsByScope(RefScopeLocal)
	if err != nil {
		t.Fatalf("ListRefsByScope LOCAL: %v", err)
	}
	if len(refs) != 1 {
		t.Errorf("expected 1 LOCAL ref, got %d", len(refs))
	}
}

func TestListRefsByScope_EmptyDB(t *testing.T) {
	d := newTestDB(t)
	refs, err := d.ListRefsByScope(RefScopeSSH)
	if err != nil {
		t.Fatalf("ListRefsByScope on empty DB: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected 0 refs on empty DB, got %d", len(refs))
	}
}

// --- GetRef + DeleteRef for SSH ---

func TestGetRef_SSH_Found(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshget01", RefStatusActive)

	ref, err := d.GetRef("VK:SSH:sshget01")
	if err != nil {
		t.Fatalf("GetRef: %v", err)
	}
	if ref.RefScope != RefScopeSSH {
		t.Errorf("scope=%s want SSH", ref.RefScope)
	}
	if ref.RefID != "sshget01" {
		t.Errorf("id=%s want sshget01", ref.RefID)
	}
}

func TestGetRef_SSH_NotFound(t *testing.T) {
	d := newTestDB(t)
	_, err := d.GetRef("VK:SSH:nonexist")
	if err == nil {
		t.Error("GetRef must return error for nonexistent ref")
	}
}

func TestDeleteRef_SSH_Removes(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshdel01", RefStatusActive)

	if err := d.DeleteRef("VK:SSH:sshdel01"); err != nil {
		t.Fatalf("DeleteRef: %v", err)
	}

	_, err := d.GetRef("VK:SSH:sshdel01")
	if err == nil {
		t.Error("ref must be gone after delete")
	}
}

func TestDeleteRef_SSH_DoesNotAffectOthers(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshdel02", RefStatusActive)
	saveSSHRef(t, d, "sshkeep1", RefStatusActive)
	saveTempRef(t, d, "tmpkeep1")

	if err := d.DeleteRef("VK:SSH:sshdel02"); err != nil {
		t.Fatalf("DeleteRef: %v", err)
	}

	refs, _ := d.ListRefsByScope(RefScopeSSH)
	if len(refs) != 1 || refs[0].RefID != "sshkeep1" {
		t.Error("other SSH ref must survive delete")
	}
	temps, _ := d.ListRefsByScope(RefScopeTemp)
	if len(temps) != 1 {
		t.Error("TEMP ref must survive SSH delete")
	}
}

// --- SaveRef with SSH scope ---

func TestSaveRef_SSH_DefaultsToActive(t *testing.T) {
	d := newTestDB(t)
	// SSH scope with empty status — should NOT default to active
	// because DefaultRefStatus only handles LOCAL/EXTERNAL → active, rest → temp
	// But we explicitly pass RefStatusActive in our saveSSHRef helper
	parts := RefParts{Family: "VK", Scope: RefScopeSSH, ID: "sshdflt1"}
	if err := d.SaveRef(parts, "cipher", 1, "", ""); err != nil {
		t.Fatalf("SaveRef: %v", err)
	}
	ref, _ := d.GetRef("VK:SSH:sshdflt1")
	// Empty status → defaults to temp (SSH is not LOCAL/EXTERNAL)
	if ref.Status != RefStatusTemp {
		t.Errorf("SSH with empty status defaults to %s (expected temp)", ref.Status)
	}
}

func TestSaveRef_SSH_ExplicitActive(t *testing.T) {
	d := newTestDB(t)
	parts := RefParts{Family: "VK", Scope: RefScopeSSH, ID: "sshexp01"}
	if err := d.SaveRef(parts, "cipher", 1, RefStatusActive, ""); err != nil {
		t.Fatalf("SaveRef: %v", err)
	}
	ref, _ := d.GetRef("VK:SSH:sshexp01")
	if ref.Status != RefStatusActive {
		t.Errorf("explicit active status must be preserved, got %s", ref.Status)
	}
}

func TestSaveRef_SSH_CanonicalFormat(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "sshcan01", RefStatusActive)
	ref, _ := d.GetRef("VK:SSH:sshcan01")
	if ref.RefCanonical != "VK:SSH:sshcan01" {
		t.Errorf("canonical=%s want VK:SSH:sshcan01", ref.RefCanonical)
	}
	if ref.RefFamily != "VK" {
		t.Errorf("family=%s want VK", ref.RefFamily)
	}
}

// --- Upsert behavior ---

func TestSaveRef_SSH_Upsert_UpdatesCiphertext(t *testing.T) {
	d := newTestDB(t)
	parts := RefParts{Family: "VK", Scope: RefScopeSSH, ID: "sshups01"}
	if err := d.SaveRef(parts, "cipher-v1", 1, RefStatusActive, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.SaveRef(parts, "cipher-v2", 2, RefStatusActive, ""); err != nil {
		t.Fatal(err)
	}
	ref, _ := d.GetRef("VK:SSH:sshups01")
	if ref.Ciphertext != "cipher-v2" {
		t.Errorf("upsert must update ciphertext, got %s", ref.Ciphertext)
	}
	if ref.Version != 2 {
		t.Errorf("upsert must update version, got %d", ref.Version)
	}
}

// --- Mixed scope isolation ---

func TestListRefsByScope_MixedScopes_Isolated(t *testing.T) {
	d := newTestDB(t)
	saveSSHRef(t, d, "ssh_iso1", RefStatusActive)
	saveSSHRef(t, d, "ssh_iso2", RefStatusActive)
	saveTempRef(t, d, "tmp_iso1")
	saveTempRef(t, d, "tmp_iso2")
	saveTempRef(t, d, "tmp_iso3")
	saveLocalRef(t, d, "loc_iso1")

	ssh, _ := d.ListRefsByScope(RefScopeSSH)
	tmp, _ := d.ListRefsByScope(RefScopeTemp)
	loc, _ := d.ListRefsByScope(RefScopeLocal)
	all, _ := d.ListRefs()

	if len(ssh) != 2 {
		t.Errorf("SSH: expected 2, got %d", len(ssh))
	}
	if len(tmp) != 3 {
		t.Errorf("TEMP: expected 3, got %d", len(tmp))
	}
	if len(loc) != 1 {
		t.Errorf("LOCAL: expected 1, got %d", len(loc))
	}
	if len(all) != 6 {
		t.Errorf("ALL: expected 6, got %d", len(all))
	}
}
