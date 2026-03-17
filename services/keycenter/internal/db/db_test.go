package db

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create test DB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestDBCreateAndGetKey(t *testing.T) {
	d := newTestDB(t)

	encDEK := []byte("encrypted-dek-data-32byteslong!!")
	nonce := []byte("12bytenonce!")

	version, err := d.CreateKey(encDEK, nonce)
	if err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}
	if version < 1 {
		t.Fatalf("expected version >= 1, got %d", version)
	}

	// GetActiveKey should return the key we just created
	active, err := d.GetActiveKey()
	if err != nil {
		t.Fatalf("GetActiveKey failed: %v", err)
	}
	if active.Version != version {
		t.Errorf("active version = %d, want %d", active.Version, version)
	}
	if !bytes.Equal(active.EncryptedDEK, encDEK) {
		t.Errorf("EncryptedDEK mismatch")
	}
	if !bytes.Equal(active.Nonce, nonce) {
		t.Errorf("Nonce mismatch")
	}
	if active.Status != "active" {
		t.Errorf("status = %q, want %q", active.Status, "active")
	}
	if active.Algorithm != "AES-256-GCM" {
		t.Errorf("algorithm = %q, want %q", active.Algorithm, "AES-256-GCM")
	}

	// GetKeyByVersion should return the same key
	byVer, err := d.GetKeyByVersion(version)
	if err != nil {
		t.Fatalf("GetKeyByVersion failed: %v", err)
	}
	if byVer.Version != version {
		t.Errorf("GetKeyByVersion version = %d, want %d", byVer.Version, version)
	}
	if !bytes.Equal(byVer.EncryptedDEK, encDEK) {
		t.Errorf("GetKeyByVersion EncryptedDEK mismatch")
	}
}

func TestDBGetAllKeys(t *testing.T) {
	d := newTestDB(t)

	// Create multiple keys
	for i := 0; i < 3; i++ {
		_, err := d.CreateKey([]byte("dek-data-padded-to-be-long-enoug"), []byte("12bytenonce!"))
		if err != nil {
			t.Fatalf("CreateKey %d failed: %v", i, err)
		}
	}

	keys, err := d.GetAllKeys()
	if err != nil {
		t.Fatalf("GetAllKeys failed: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	// Should be ordered DESC by version
	if keys[0].Version <= keys[1].Version || keys[1].Version <= keys[2].Version {
		t.Errorf("keys not in descending order: %d, %d, %d", keys[0].Version, keys[1].Version, keys[2].Version)
	}
}

func TestDBUpdateKeyEncryption(t *testing.T) {
	d := newTestDB(t)

	version, err := d.CreateKey([]byte("original-dek-data-32bytes-long!!"), []byte("12bytenonce!"))
	if err != nil {
		t.Fatalf("CreateKey failed: %v", err)
	}

	newDEK := []byte("updated--dek-data-32bytes-long!!")
	newNonce := []byte("newnonce12by")
	if err := d.UpdateKeyEncryption(version, newDEK, newNonce); err != nil {
		t.Fatalf("UpdateKeyEncryption failed: %v", err)
	}

	key, err := d.GetKeyByVersion(version)
	if err != nil {
		t.Fatalf("GetKeyByVersion failed: %v", err)
	}
	if !bytes.Equal(key.EncryptedDEK, newDEK) {
		t.Errorf("EncryptedDEK not updated")
	}
	if !bytes.Equal(key.Nonce, newNonce) {
		t.Errorf("Nonce not updated")
	}

	// Update non-existent version should fail
	err = d.UpdateKeyEncryption(9999, newDEK, newNonce)
	if err == nil {
		t.Error("expected error for non-existent version, got nil")
	}
}

func TestDBPing(t *testing.T) {
	d := newTestDB(t)
	if err := d.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// After close, ping should fail
	d.Close()
	if err := d.Ping(); err == nil {
		t.Error("expected Ping to fail after Close")
	}
}

func TestDBNewInvalidPath(t *testing.T) {
	// Non-existent directory should fail
	_, err := New(filepath.Join(os.TempDir(), "nonexistent-dir-xyz", "subdir", "test.db"))
	if err == nil {
		t.Error("expected error for invalid DB path, got nil")
	}
}

func TestDBAutoMigratesOperatorCatalogTables(t *testing.T) {
	d := newTestDB(t)

	for _, table := range []string{
		"vault_inventory",
		"secret_catalog",
		"bindings",
		"audit_events",
	} {
		if !d.conn.Migrator().HasTable(table) {
			t.Fatalf("expected table %s to exist after migration", table)
		}
	}
}

func TestDBBackfillsVaultInventoryFromAgents(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-a", "vault-a", "vh-a", "vault-a", "127.0.0.1", 10180, 2, 3, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.UpdateAgentDEK("node-a", "agent-a", []byte("0123456789abcdef0123456789abcdef"), []byte("123456789012")); err != nil {
		t.Fatalf("UpdateAgentDEK failed: %v", err)
	}
	if err := d.UpdateAgentManagedPaths("node-a", []string{"/srv/app", "/srv/app", "relative/path"}); err != nil {
		t.Fatalf("UpdateAgentManagedPaths failed: %v", err)
	}

	rows, err := d.ListVaultInventory()
	if err != nil {
		t.Fatalf("ListVaultInventory failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 vault inventory row, got %d", len(rows))
	}
	row := rows[0]
	if row.VaultNodeUUID != "node-a" {
		t.Fatalf("vault_node_uuid = %q", row.VaultNodeUUID)
	}
	if row.VaultHash != "vh-a" || row.VaultName != "vault-a" {
		t.Fatalf("unexpected vault identity: %+v", row)
	}
	if row.Status != "ok" {
		t.Fatalf("status = %q, want ok", row.Status)
	}
	if row.DisplayName != "vault-a" {
		t.Fatalf("display_name = %q", row.DisplayName)
	}
	if row.ManagedPathsJSON == "" {
		t.Fatal("managed_paths_json should be populated from agent")
	}
}

func TestDBBackfillsAgentCapabilitiesFromLegacyRole(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-host-only", "host-only-agent", "vh-host", "host-only-agent", "127.0.0.1", 10180, 0, 0, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.conn.Model(&Agent{}).Where("node_id = ?", "node-host-only").Updates(map[string]any{
		"agent_role":    "host-only",
		"host_enabled":  false,
		"local_enabled": true,
	}).Error; err != nil {
		t.Fatalf("seed legacy capability state: %v", err)
	}

	if err := d.BackfillAgentCapabilities(); err != nil {
		t.Fatalf("BackfillAgentCapabilities failed: %v", err)
	}

	agent, err := d.GetAgentByNodeID("node-host-only")
	if err != nil {
		t.Fatalf("GetAgentByNodeID failed: %v", err)
	}
	if !agent.HostEnabled || agent.LocalEnabled {
		t.Fatalf("capabilities = host:%v local:%v, want host-only", agent.HostEnabled, agent.LocalEnabled)
	}
	if agent.AgentRole != "host-only" {
		t.Fatalf("agent_role = %q, want host-only", agent.AgentRole)
	}
}

func TestDBSaveAndListSecretCatalog(t *testing.T) {
	d := newTestDB(t)

	entry := &SecretCatalog{
		SecretCanonicalID: "vh-a:API_KEY",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
		FieldsPresentJSON: `["URL"]`,
	}
	if err := d.SaveSecretCatalog(entry); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.SecretName != "API_KEY" || got.VaultHash != "vh-a" {
		t.Fatalf("unexpected catalog row: %+v", got)
	}

	rows, err := d.ListSecretCatalog()
	if err != nil {
		t.Fatalf("ListSecretCatalog failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 catalog row, got %d", len(rows))
	}
}

func TestDBBackfillsSecretCatalogFromTrackedRefs(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.UpdateAgentDEK("node-a", "agent-a", []byte("dek"), []byte("nonce")); err != nil {
		t.Fatalf("UpdateAgentDEK failed: %v", err)
	}

	if err := d.SaveRef(RefParts{Family: "VK", Scope: "LOCAL", ID: "OPENAI_API_KEY"}, "ciphertext", 4, "active", "agent-a"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.SecretCanonicalID != "vh-a:VK:LOCAL:OPENAI_API_KEY" {
		t.Fatalf("secret_canonical_id = %q", got.SecretCanonicalID)
	}
	if got.SecretName != "OPENAI_API_KEY" {
		t.Fatalf("secret_name = %q", got.SecretName)
	}
	if got.Class != "key" {
		t.Fatalf("class = %q, want key", got.Class)
	}
	if got.VaultHash != "vh-a" || got.VaultRuntimeHash != "agent-a" {
		t.Fatalf("unexpected vault binding: %+v", got)
	}
}

func TestDBBackfillsConfigCatalogFromTrackedRefs(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.UpdateAgentDEK("node-a", "agent-a", []byte("dek"), []byte("nonce")); err != nil {
		t.Fatalf("UpdateAgentDEK failed: %v", err)
	}

	if err := d.SaveRef(RefParts{Family: "VE", Scope: "LOCAL", ID: "APP_URL"}, "ciphertext", 2, "active", "agent-a"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VE:LOCAL:APP_URL")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.Class != "config" {
		t.Fatalf("class = %q, want config", got.Class)
	}
}

func TestDBUpsertSecretCatalogFromTrackedRefPreservesExistingMetadata(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "10.0.0.10", 10180, 0, 0, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.UpdateAgentDEK("node-a", "agent-a", []byte("dek"), []byte("nonce")); err != nil {
		t.Fatalf("UpdateAgentDEK failed: %v", err)
	}

	revealedAt := time.Now().UTC().Add(-time.Minute)
	rotatedAt := time.Now().UTC().Add(-2 * time.Minute)
	if err := d.SaveSecretCatalog(&SecretCatalog{
		SecretCanonicalID: "vh-a:VK:LOCAL:OPENAI_API_KEY",
		SecretName:        "OPENAI_API_KEY",
		DisplayName:       "OpenAI API Key",
		Description:       "Primary upstream token",
		TagsJSON:          `["llm","prod"]`,
		Class:             "credential",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:OPENAI_API_KEY",
		FieldsPresentJSON: `["LOGIN_ID","OTP"]`,
		LastRotatedAt:     &rotatedAt,
		LastRevealedAt:    &revealedAt,
	}); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	if err := d.SaveRef(RefParts{Family: "VK", Scope: "LOCAL", ID: "OPENAI_API_KEY"}, "ciphertext", 4, "active", "agent-a"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:OPENAI_API_KEY")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.DisplayName != "OpenAI API Key" || got.Description != "Primary upstream token" {
		t.Fatalf("metadata was not preserved: %+v", got)
	}
	if got.TagsJSON != `["llm","prod"]` {
		t.Fatalf("TagsJSON = %q", got.TagsJSON)
	}
	if got.Class != "credential" {
		t.Fatalf("Class = %q, want credential", got.Class)
	}
	if got.FieldsPresentJSON != `["LOGIN_ID","OTP"]` {
		t.Fatalf("FieldsPresentJSON = %q", got.FieldsPresentJSON)
	}
	if got.LastRotatedAt == nil || got.LastRevealedAt == nil {
		t.Fatalf("rotation/reveal timestamps should survive upsert: %+v", got)
	}
}

func TestDBSaveBindingAndCountByRef(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSecretCatalog(&SecretCatalog{
		SecretCanonicalID: "vh-a:VK:LOCAL:deadbeef",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
	}); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	entry := &Binding{
		BindingID:    "bind-1",
		BindingType:  "function",
		TargetName:   "gitlab/current-user",
		VaultHash:    "vh-a",
		SecretName:   "API_KEY",
		FieldKey:     "",
		RefCanonical: "VK:LOCAL:deadbeef",
		Required:     true,
	}
	if err := d.SaveBinding(entry); err != nil {
		t.Fatalf("SaveBinding failed: %v", err)
	}

	rows, err := d.ListBindingsByTarget("function", "gitlab/current-user")
	if err != nil {
		t.Fatalf("ListBindingsByTarget failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 binding row, got %d", len(rows))
	}

	count, err := d.CountBindingsForRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("CountBindingsForRef failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("binding count = %d", count)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.BindingCount != 1 {
		t.Fatalf("catalog binding_count = %d, want 1", got.BindingCount)
	}
}

func TestDBRefreshSecretCatalogBindingCount(t *testing.T) {
	d := newTestDB(t)

	entry := &SecretCatalog{
		SecretCanonicalID: "vh-a:API_KEY",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
	}
	if err := d.SaveSecretCatalog(entry); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	for _, bindingID := range []string{"bind-1", "bind-2"} {
		if err := d.SaveBinding(&Binding{
			BindingID:    bindingID,
			BindingType:  "function",
			TargetName:   "gitlab/current-user",
			VaultHash:    "vh-a",
			SecretName:   "API_KEY",
			RefCanonical: "VK:LOCAL:deadbeef",
			Required:     true,
		}); err != nil {
			t.Fatalf("SaveBinding(%s) failed: %v", bindingID, err)
		}
	}

	if err := d.RefreshSecretCatalogBindingCount("VK:LOCAL:deadbeef"); err != nil {
		t.Fatalf("RefreshSecretCatalogBindingCount failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.BindingCount != 2 {
		t.Fatalf("binding_count = %d, want 2", got.BindingCount)
	}
}

func TestDBReplaceBindingsForTargetRefreshesCounts(t *testing.T) {
	d := newTestDB(t)

	for _, refCanonical := range []string{"VK:LOCAL:deadbeef", "VK:LOCAL:feedface"} {
		if err := d.SaveSecretCatalog(&SecretCatalog{
			SecretCanonicalID: "vh-a:" + refCanonical,
			SecretName:        refCanonical,
			DisplayName:       refCanonical,
			Class:             "key",
			Scope:             "LOCAL",
			Status:            "active",
			VaultNodeUUID:     "node-a",
			VaultRuntimeHash:  "agent-a",
			VaultHash:         "vh-a",
			RefCanonical:      refCanonical,
		}); err != nil {
			t.Fatalf("SaveSecretCatalog(%s) failed: %v", refCanonical, err)
		}
	}

	if err := d.ReplaceBindingsForTarget("function", "gitlab/current-user", []Binding{
		{
			BindingID:    "bind-1",
			BindingType:  "function",
			TargetName:   "gitlab/current-user",
			VaultHash:    "vh-a",
			SecretName:   "API_KEY",
			RefCanonical: "VK:LOCAL:deadbeef",
			Required:     true,
		},
	}); err != nil {
		t.Fatalf("ReplaceBindingsForTarget(initial) failed: %v", err)
	}

	if err := d.ReplaceBindingsForTarget("function", "gitlab/current-user", []Binding{
		{
			BindingID:    "bind-2",
			BindingType:  "function",
			TargetName:   "gitlab/current-user",
			VaultHash:    "vh-a",
			SecretName:   "API_TOKEN",
			RefCanonical: "VK:LOCAL:feedface",
			Required:     true,
		},
	}); err != nil {
		t.Fatalf("ReplaceBindingsForTarget(update) failed: %v", err)
	}

	oldRef, err := d.GetSecretCatalogByRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef(old) failed: %v", err)
	}
	if oldRef.BindingCount != 0 {
		t.Fatalf("old ref binding_count = %d, want 0", oldRef.BindingCount)
	}

	newRef, err := d.GetSecretCatalogByRef("VK:LOCAL:feedface")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef(new) failed: %v", err)
	}
	if newRef.BindingCount != 1 {
		t.Fatalf("new ref binding_count = %d, want 1", newRef.BindingCount)
	}
}

func TestDBDeleteBindingsByTargetRefreshesCounts(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSecretCatalog(&SecretCatalog{
		SecretCanonicalID: "vh-a:VK:LOCAL:deadbeef",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
	}); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	if err := d.SaveBinding(&Binding{
		BindingID:    "bind-1",
		BindingType:  "function",
		TargetName:   "gitlab/current-user",
		VaultHash:    "vh-a",
		SecretName:   "API_KEY",
		RefCanonical: "VK:LOCAL:deadbeef",
		Required:     true,
	}); err != nil {
		t.Fatalf("SaveBinding failed: %v", err)
	}

	if err := d.DeleteBindingsByTarget("function", "gitlab/current-user"); err != nil {
		t.Fatalf("DeleteBindingsByTarget failed: %v", err)
	}

	got, err := d.GetSecretCatalogByRef("VK:LOCAL:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if got.BindingCount != 0 {
		t.Fatalf("binding_count = %d, want 0", got.BindingCount)
	}
}

func TestDBSaveBindingWritesAuditEvent(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSecretCatalog(&SecretCatalog{
		SecretCanonicalID: "vh-a:VK:LOCAL:deadbeef",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
	}); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	if err := d.SaveBinding(&Binding{
		BindingID:    "bind-1",
		BindingType:  "function",
		TargetName:   "gitlab/current-user",
		VaultHash:    "vh-a",
		SecretName:   "API_KEY",
		RefCanonical: "VK:LOCAL:deadbeef",
		Required:     true,
	}); err != nil {
		t.Fatalf("SaveBinding failed: %v", err)
	}

	rows, err := d.ListAuditEvents("binding", "bind-1")
	if err != nil {
		t.Fatalf("ListAuditEvents failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].Action != "upsert" || rows[0].Source != "operator_catalog_db" {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
}

func TestDBReplaceBindingsForTargetWritesAuditEvents(t *testing.T) {
	d := newTestDB(t)

	for _, refCanonical := range []string{"VK:LOCAL:deadbeef", "VE:LOCAL:APP_URL"} {
		if err := d.SaveSecretCatalog(&SecretCatalog{
			SecretCanonicalID: "vh-a:" + refCanonical,
			SecretName:        refCanonical,
			DisplayName:       refCanonical,
			Class:             "key",
			Scope:             "LOCAL",
			Status:            "active",
			VaultNodeUUID:     "node-a",
			VaultRuntimeHash:  "agent-a",
			VaultHash:         "vh-a",
			RefCanonical:      refCanonical,
		}); err != nil {
			t.Fatalf("SaveSecretCatalog(%s) failed: %v", refCanonical, err)
		}
	}

	if err := d.ReplaceBindingsForTarget("function", "gitlab/current-user", []Binding{
		{
			BindingID:    "bind-1",
			BindingType:  "function",
			TargetName:   "gitlab/current-user",
			VaultHash:    "vh-a",
			SecretName:   "API_KEY",
			RefCanonical: "VK:LOCAL:deadbeef",
			Required:     true,
		},
	}); err != nil {
		t.Fatalf("ReplaceBindingsForTarget(initial) failed: %v", err)
	}

	if err := d.ReplaceBindingsForTarget("function", "gitlab/current-user", []Binding{
		{
			BindingID:    "bind-2",
			BindingType:  "function",
			TargetName:   "gitlab/current-user",
			VaultHash:    "vh-a",
			SecretName:   "APP_URL",
			RefCanonical: "VE:LOCAL:APP_URL",
			Required:     true,
		},
	}); err != nil {
		t.Fatalf("ReplaceBindingsForTarget(update) failed: %v", err)
	}

	oldRows, err := d.ListAuditEvents("binding", "bind-1")
	if err != nil {
		t.Fatalf("ListAuditEvents(bind-1) failed: %v", err)
	}
	if len(oldRows) == 0 || oldRows[0].Action != "delete" {
		t.Fatalf("expected delete audit for bind-1, got %+v", oldRows)
	}

	newRows, err := d.ListAuditEvents("binding", "bind-2")
	if err != nil {
		t.Fatalf("ListAuditEvents(bind-2) failed: %v", err)
	}
	if len(newRows) == 0 || newRows[0].Action != "upsert" {
		t.Fatalf("expected upsert audit for bind-2, got %+v", newRows)
	}
}

func TestDBDeleteBindingsByTargetWritesAuditEvents(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveSecretCatalog(&SecretCatalog{
		SecretCanonicalID: "vh-a:VK:LOCAL:deadbeef",
		SecretName:        "API_KEY",
		DisplayName:       "API Key",
		Class:             "key",
		Scope:             "LOCAL",
		Status:            "active",
		VaultNodeUUID:     "node-a",
		VaultRuntimeHash:  "agent-a",
		VaultHash:         "vh-a",
		RefCanonical:      "VK:LOCAL:deadbeef",
	}); err != nil {
		t.Fatalf("SaveSecretCatalog failed: %v", err)
	}

	if err := d.SaveBinding(&Binding{
		BindingID:    "bind-1",
		BindingType:  "function",
		TargetName:   "gitlab/current-user",
		VaultHash:    "vh-a",
		SecretName:   "API_KEY",
		RefCanonical: "VK:LOCAL:deadbeef",
		Required:     true,
	}); err != nil {
		t.Fatalf("SaveBinding failed: %v", err)
	}

	if err := d.DeleteBindingsByTarget("function", "gitlab/current-user"); err != nil {
		t.Fatalf("DeleteBindingsByTarget failed: %v", err)
	}

	rows, err := d.ListAuditEvents("binding", "bind-1")
	if err != nil {
		t.Fatalf("ListAuditEvents failed: %v", err)
	}
	if len(rows) < 2 {
		t.Fatalf("expected save+delete audit rows, got %d", len(rows))
	}
	if rows[0].Action != "delete" {
		t.Fatalf("expected latest audit action delete, got %+v", rows[0])
	}
}

func TestDBSaveAndListAuditEvents(t *testing.T) {
	d := newTestDB(t)

	entry := &AuditEvent{
		EventID:             "evt-1",
		EntityType:          "secret",
		EntityID:            "vh-a:API_KEY",
		Action:              "reveal",
		ActorType:           "operator",
		ActorID:             "root",
		Reason:              "manual check",
		Source:              "cli",
		ApprovalChallengeID: "challenge-1",
		BeforeJSON:          `{}`,
		AfterJSON:           `{"status":"revealed"}`,
	}
	if err := d.SaveAuditEvent(entry); err != nil {
		t.Fatalf("SaveAuditEvent failed: %v", err)
	}

	rows, err := d.ListAuditEvents("secret", "vh-a:API_KEY")
	if err != nil {
		t.Fatalf("ListAuditEvents failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].ApprovalChallengeID != "challenge-1" {
		t.Fatalf("unexpected audit row: %+v", rows[0])
	}
}

func TestDBSaveAndGetScopedTokenRef(t *testing.T) {
	d := newTestDB(t)

	parts := RefParts{
		Family: "VK",
		Scope:  "TEMP",
		ID:     "deadbeef",
	}
	if err := d.SaveRef(parts, "ciphertext-1", 7, "temp", "agent01"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}

	ref, err := d.GetRef("VK:TEMP:deadbeef")
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if ref.RefFamily != "VK" || ref.RefScope != "TEMP" || ref.RefID != "deadbeef" {
		t.Fatalf("unexpected ref parts: %+v", ref)
	}
	if ref.RefCanonical != "VK:TEMP:deadbeef" {
		t.Fatalf("canonical = %q", ref.RefCanonical)
	}
	if ref.Status != "temp" {
		t.Fatalf("status = %q", ref.Status)
	}
}

func TestDBUpdateScopedTokenRefStatus(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveRef(RefParts{Family: "VE", Scope: "TEMP", ID: "cfg_01"}, "ciphertext-2", 1, "temp", "agent01"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}
	if err := d.UpdateRef("VE:TEMP:cfg_01", "ciphertext-3", 2, "active"); err != nil {
		t.Fatalf("UpdateRef failed: %v", err)
	}

	ref, err := d.GetRef("VE:TEMP:cfg_01")
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if ref.Ciphertext != "ciphertext-3" || ref.Version != 2 || ref.Status != "active" {
		t.Fatalf("unexpected updated ref: %+v", ref)
	}
}

func TestDBSaveRefWithNameProjectsSecretCatalogName(t *testing.T) {
	d := newTestDB(t)

	if err := d.UpsertAgent("node-a", "agent-a", "vh-a", "vault-a", "127.0.0.1", 10180, 1, 1, 1, 1); err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}
	if err := d.UpdateAgentDEK("node-a", "agent-a", []byte("0123456789abcdef0123456789abcdef"), []byte("123456789012")); err != nil {
		t.Fatalf("UpdateAgentDEK failed: %v", err)
	}

	parts := RefParts{Family: "VK", Scope: "TEMP", ID: "deadbeef"}
	if err := d.SaveRefWithName(parts, "ciphertext-1", 7, "temp", "agent-a", "GEMINI_API_KEY"); err != nil {
		t.Fatalf("SaveRefWithName failed: %v", err)
	}

	ref, err := d.GetRef("VK:TEMP:deadbeef")
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if ref.SecretName != "GEMINI_API_KEY" {
		t.Fatalf("secret_name = %q", ref.SecretName)
	}

	catalog, err := d.GetSecretCatalogByRef("VK:TEMP:deadbeef")
	if err != nil {
		t.Fatalf("GetSecretCatalogByRef failed: %v", err)
	}
	if catalog.SecretName != "GEMINI_API_KEY" {
		t.Fatalf("catalog secret_name = %q", catalog.SecretName)
	}
	if catalog.DisplayName != "GEMINI_API_KEY" {
		t.Fatalf("catalog display_name = %q", catalog.DisplayName)
	}
}

func TestDBScopedTokenRefCountAndDelete(t *testing.T) {
	d := newTestDB(t)

	if err := d.SaveRef(RefParts{Family: "VK", Scope: "TEMP", ID: "a1"}, "c1", 1, "temp", "agent01"); err != nil {
		t.Fatalf("SaveRef #1 failed: %v", err)
	}
	if err := d.SaveRef(RefParts{Family: "VE", Scope: "LOCAL", ID: "b2"}, "c2", 1, "active", "agent02"); err != nil {
		t.Fatalf("SaveRef #2 failed: %v", err)
	}

	count, err := d.CountRefs()
	if err != nil {
		t.Fatalf("CountRefs failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d", count)
	}

	if err := d.DeleteRef("VK:TEMP:a1"); err != nil {
		t.Fatalf("DeleteRef failed: %v", err)
	}
	count, err = d.CountRefs()
	if err != nil {
		t.Fatalf("CountRefs after delete failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("count after delete = %d", count)
	}
}

func TestDBSaveRefUpsertByCanonical(t *testing.T) {
	d := newTestDB(t)

	parts := RefParts{Family: "VE", Scope: "LOCAL", ID: "cfg_dedupe"}
	if err := d.SaveRef(parts, "ciphertext-1", 1, "active", "agent01"); err != nil {
		t.Fatalf("SaveRef first failed: %v", err)
	}
	if err := d.SaveRef(parts, "ciphertext-2", 2, "active", "agent02"); err != nil {
		t.Fatalf("SaveRef second failed: %v", err)
	}

	count, err := d.CountRefs()
	if err != nil {
		t.Fatalf("CountRefs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 ref after upsert, got %d", count)
	}

	ref, err := d.GetRef(parts.Canonical())
	if err != nil {
		t.Fatalf("GetRef failed: %v", err)
	}
	if ref.Ciphertext != "ciphertext-2" || ref.Version != 2 || ref.AgentHash != "agent02" {
		t.Fatalf("unexpected upserted ref: %+v", ref)
	}
}

func TestDBNewPromotesOperationalTempRefsOnStartup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	first, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	if err := first.SaveRef(RefParts{Family: "VK", Scope: "TEMP", ID: "startup01"}, "ciphertext-1", 1, "temp", "agent01"); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	second, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New failed: %v", err)
	}
	defer second.Close()

	if _, err := second.GetRef("VK:TEMP:startup01"); err == nil {
		t.Fatal("operational temp ref should be promoted away from TEMP on restart")
	}
	ref, err := second.GetRef("VK:LOCAL:startup01")
	if err != nil {
		t.Fatalf("expected LOCAL ref after restart: %v", err)
	}
	if ref.Status != "active" {
		t.Fatalf("promoted ref status = %q, want active", ref.Status)
	}
}


func TestDBNewKeepsHostOwnedTempRefsOnStartup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	first, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	if err := first.SaveRef(RefParts{Family: "VK", Scope: "TEMP", ID: "startup-host-01"}, "ciphertext-host", 1, "temp", ""); err != nil {
		t.Fatalf("SaveRef failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	second, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New failed: %v", err)
	}
	defer second.Close()

	ref, err := second.GetRef("VK:TEMP:startup-host-01")
	if err != nil {
		t.Fatalf("host-owned temp ref should remain after restart: %v", err)
	}
	if ref.Status != "temp" {
		t.Fatalf("host-owned temp ref status = %q, want temp", ref.Status)
	}
	if _, err := second.GetRef("VK:LOCAL:startup-host-01"); err == nil {
		t.Fatal("host-owned temp ref should not be promoted to LOCAL on restart")
	}
}

func TestDBNewDropsTempRefThatWouldConflictWithExistingLocalRef(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	first, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	if err := first.SaveRef(RefParts{Family: "VK", Scope: "LOCAL", ID: "startup03"}, "ciphertext-local", 2, "active", "agent-local"); err != nil {
		t.Fatalf("SaveRef local failed: %v", err)
	}
	if err := first.SaveRef(RefParts{Family: "VK", Scope: "TEMP", ID: "startup03"}, "ciphertext-temp", 1, "temp", "agent-temp"); err != nil {
		t.Fatalf("SaveRef temp failed: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	second, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New failed: %v", err)
	}
	defer second.Close()

	ref, err := second.GetRef("VK:LOCAL:startup03")
	if err != nil {
		t.Fatalf("expected LOCAL ref after restart: %v", err)
	}
	if ref.Ciphertext != "ciphertext-local" || ref.Status != "active" {
		t.Fatalf("unexpected surviving LOCAL ref: %+v", ref)
	}
	if _, err := second.GetRef("VK:TEMP:startup03"); err == nil {
		t.Fatal("conflicting TEMP ref should be removed during startup promotion")
	}
	count, err := second.CountRefs()
	if err != nil {
		t.Fatalf("CountRefs failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 ref after conflict cleanup, got %d", count)
	}
}

func TestDBNewBackfillsRefPartsFromCanonicalStorage(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	first, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	if err := first.conn.Exec(`
INSERT INTO token_refs (ref_canonical, ref_family, ref_scope, ref_id, agent_hash, ciphertext, version, status, created_at)
VALUES (?, '', '', '', ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, "VK:TEMP:backfill01", "agent-backfill", "ciphertext-1", 1, "temp").Error; err != nil {
		t.Fatalf("seed legacy token_ref: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	second, err := New(dbPath)
	if err != nil {
		t.Fatalf("second New failed: %v", err)
	}
	defer second.Close()

	ref, err := second.GetRef("VK:LOCAL:backfill01")
	if err != nil {
		t.Fatalf("expected promoted/backfilled ref after restart: %v", err)
	}
	if ref.RefFamily != "VK" || ref.RefScope != "LOCAL" || ref.RefID != "backfill01" {
		t.Fatalf("backfilled parts = %+v", ref)
	}
}

func TestDBNewRejectsCanonicalComponentMismatch(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	first, err := New(dbPath)
	if err != nil {
		t.Fatalf("first New failed: %v", err)
	}
	if err := first.conn.Exec(`
INSERT INTO token_refs (ref_canonical, ref_family, ref_scope, ref_id, agent_hash, ciphertext, version, status, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
`, "VK:LOCAL:mismatch01", "VE", "LOCAL", "mismatch01", "agent-mismatch", "ciphertext-1", 1, "active").Error; err != nil {
		t.Fatalf("seed mismatched token_ref: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}

	if _, err := New(dbPath); err == nil {
		t.Fatal("expected New to fail on canonical/component mismatch")
	}
}
