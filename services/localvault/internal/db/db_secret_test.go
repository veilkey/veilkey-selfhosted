package db

import (
	"testing"
)

func TestSaveAndGetSecret(t *testing.T) {
	d := newTestDB(t)

	secret := &Secret{
		ID:         "id-1",
		Name:       "DB_PASSWORD",
		Ref:        "VK:LOCAL:abc123",
		Ciphertext: []byte("encrypted"),
		Nonce:      []byte("nonce"),
		Version:    1,
	}

	if err := d.SaveSecret(secret); err != nil {
		t.Fatalf("SaveSecret: %v", err)
	}

	// Defaults should be applied.
	got, err := d.GetSecretByName("DB_PASSWORD")
	if err != nil {
		t.Fatalf("GetSecretByName: %v", err)
	}
	if got.Status != RefStatusActive {
		t.Errorf("status = %q, want %q", got.Status, RefStatusActive)
	}
	if got.Scope != RefScopeLocal {
		t.Errorf("scope = %q, want %q", got.Scope, RefScopeLocal)
	}
}

func TestGetSecretByRef(t *testing.T) {
	d := newTestDB(t)

	secret := &Secret{
		ID:         "id-2",
		Name:       "API_KEY",
		Ref:        "VK:LOCAL:ref-2",
		Ciphertext: []byte("ct"),
		Nonce:      []byte("n"),
		Version:    1,
	}
	d.SaveSecret(secret)

	got, err := d.GetSecretByRef("VK:LOCAL:ref-2")
	if err != nil {
		t.Fatalf("GetSecretByRef: %v", err)
	}
	if got.Name != "API_KEY" {
		t.Errorf("name = %q, want API_KEY", got.Name)
	}
}

func TestGetSecret_NotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetSecretByName("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}

	_, err = d.GetSecretByRef("VK:LOCAL:nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}

func TestListSecrets(t *testing.T) {
	d := newTestDB(t)

	for _, name := range []string{"ZEBRA", "ALPHA", "MIDDLE"} {
		d.SaveSecret(&Secret{
			ID:         "id-" + name,
			Name:       name,
			Ref:        "VK:LOCAL:" + name,
			Ciphertext: []byte("ct"),
			Nonce:      []byte("n"),
			Version:    1,
		})
	}

	secrets, err := d.ListSecrets()
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if len(secrets) != 3 {
		t.Fatalf("len = %d, want 3", len(secrets))
	}
	// Should be ordered by name.
	if secrets[0].Name != "ALPHA" {
		t.Errorf("first = %q, want ALPHA", secrets[0].Name)
	}
}

func TestDeleteSecret(t *testing.T) {
	d := newTestDB(t)

	d.SaveSecret(&Secret{
		ID:         "id-del",
		Name:       "TO_DELETE",
		Ref:        "VK:LOCAL:del",
		Ciphertext: []byte("ct"),
		Nonce:      []byte("n"),
		Version:    1,
	})

	if err := d.DeleteSecret("TO_DELETE"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	_, err := d.GetSecretByName("TO_DELETE")
	if err == nil {
		t.Fatal("secret should be deleted")
	}
}

func TestDeleteSecret_NotFound(t *testing.T) {
	d := newTestDB(t)
	if err := d.DeleteSecret("NONEXISTENT"); err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestCountSecrets(t *testing.T) {
	d := newTestDB(t)

	count, err := d.CountSecrets()
	if err != nil {
		t.Fatalf("CountSecrets: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}

	d.SaveSecret(&Secret{
		ID: "id-c1", Name: "S1", Ref: "VK:LOCAL:c1",
		Ciphertext: []byte("ct"), Nonce: []byte("n"), Version: 1,
	})
	d.SaveSecret(&Secret{
		ID: "id-c2", Name: "S2", Ref: "VK:LOCAL:c2",
		Ciphertext: []byte("ct"), Nonce: []byte("n"), Version: 1,
	})

	count, err = d.CountSecrets()
	if err != nil {
		t.Fatalf("CountSecrets: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestUpdateSecretStatus(t *testing.T) {
	d := newTestDB(t)

	d.SaveSecret(&Secret{
		ID: "id-s", Name: "STATUS_TEST", Ref: "VK:LOCAL:st",
		Ciphertext: []byte("ct"), Nonce: []byte("n"), Version: 1,
	})

	if err := d.UpdateSecretStatus("VK:LOCAL:st", RefStatusBlock); err != nil {
		t.Fatalf("UpdateSecretStatus: %v", err)
	}

	got, _ := d.GetSecretByRef("VK:LOCAL:st")
	if got.Status != RefStatusBlock {
		t.Errorf("status = %q, want %q", got.Status, RefStatusBlock)
	}
}

func TestUpdateSecretStatus_NotFound(t *testing.T) {
	d := newTestDB(t)
	if err := d.UpdateSecretStatus("VK:LOCAL:nope", RefStatusBlock); err == nil {
		t.Fatal("expected error for nonexistent ref")
	}
}

func TestReencryptAllSecrets(t *testing.T) {
	d := newTestDB(t)

	for i, name := range []string{"S1", "S2"} {
		d.SaveSecret(&Secret{
			ID: name, Name: name, Ref: "VK:LOCAL:" + name,
			Ciphertext: []byte("old-ct"), Nonce: []byte("old-n"), Version: 1,
		})
		_ = i
	}

	decryptFn := func(ct, nonce []byte) ([]byte, error) {
		return []byte("plaintext-" + string(ct)), nil
	}
	encryptFn := func(pt []byte) ([]byte, []byte, error) {
		return []byte("new-ct"), []byte("new-n"), nil
	}

	count, err := d.ReencryptAllSecrets(decryptFn, encryptFn, 2)
	if err != nil {
		t.Fatalf("ReencryptAllSecrets: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}

	got, _ := d.GetSecretByName("S1")
	if string(got.Ciphertext) != "new-ct" {
		t.Errorf("ciphertext = %q, want new-ct", got.Ciphertext)
	}
	if got.Version != 2 {
		t.Errorf("version = %d, want 2", got.Version)
	}
	if got.LastRotatedAt == nil {
		t.Error("LastRotatedAt should be set")
	}
}

func TestReencryptMixedSecrets_SkipsCurrent(t *testing.T) {
	d := newTestDB(t)

	// Secret already at version 2.
	d.SaveSecret(&Secret{
		ID: "current", Name: "CURRENT", Ref: "VK:LOCAL:cur",
		Ciphertext: []byte("v2-ct"), Nonce: []byte("v2-n"), Version: 2,
	})
	// Secret at old version.
	d.SaveSecret(&Secret{
		ID: "old", Name: "OLD", Ref: "VK:LOCAL:old",
		Ciphertext: []byte("v1-ct"), Nonce: []byte("v1-n"), Version: 1,
	})

	decryptOld := func(ct, nonce []byte) ([]byte, error) {
		return []byte("plain"), nil
	}
	decryptCurrent := func(ct, nonce []byte) ([]byte, error) {
		return []byte("plain"), nil
	}
	encrypt := func(pt []byte) ([]byte, []byte, error) {
		return []byte("new-ct"), []byte("new-n"), nil
	}

	updated, skipped, err := d.ReencryptMixedSecrets(decryptOld, decryptCurrent, encrypt, 2)
	if err != nil {
		t.Fatalf("ReencryptMixedSecrets: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated = %d, want 1", updated)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}
