package db

import (
	"bytes"
	"testing"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// ══════════════════════════════════════════════════════════════════
// SSHKey CRUD tests
// ══════════════════════════════════════════════════════════════════

func TestSSHKey_EncryptDecrypt_Roundtrip(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	privateKey := []byte("-----BEGIN OPENSSH PRIVATE KEY-----\ntest-private-key-data\n-----END OPENSSH PRIVATE KEY-----")

	ciphertext, nonce, err := crypto.Encrypt(key, privateKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(key, ciphertext, nonce)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, privateKey) {
		t.Errorf("SSH private key roundtrip failed: got %q, want %q", decrypted, privateKey)
	}
}

func TestSSHKey_EncryptDecrypt_WrongKey_Fails(t *testing.T) {
	key1, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	key2, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	privateKey := []byte("ssh-private-key-content")
	ciphertext, nonce, err := crypto.Encrypt(key1, privateKey)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = crypto.Decrypt(key2, ciphertext, nonce)
	if err == nil {
		t.Error("Decrypt with wrong key must return error")
	}
}

func TestSSHKey_ModelFields(t *testing.T) {
	key := SSHKey{
		Ref:         "VK:SSH:abc12345",
		Ownership:   "own",
		Label:       "my-server-key",
		KeyType:     "ed25519",
		Fingerprint: "sha256-fingerprint-here",
		PublicKey:   "ssh-ed25519 AAAAC3...",
		HostsJSON:   "[]",
		MetadataJSON: "{}",
	}

	if key.TableName() != "ssh_keys" {
		t.Errorf("TableName: got %q, want %q", key.TableName(), "ssh_keys")
	}
	if key.Ref != "VK:SSH:abc12345" {
		t.Errorf("Ref: got %q", key.Ref)
	}
	if key.Ownership != "own" {
		t.Errorf("Ownership: got %q", key.Ownership)
	}
	if key.KeyType != "ed25519" {
		t.Errorf("KeyType: got %q", key.KeyType)
	}
}

func TestSSHKey_ExternalOwnership_NoPrivateKey(t *testing.T) {
	key := SSHKey{
		Ref:         "VK:SSH:ext12345",
		Ownership:   "external",
		Label:       "external-key",
		KeyType:     "rsa",
		Fingerprint: "sha256-ext-fingerprint",
		PublicKey:   "ssh-rsa AAAAB3...",
		HostsJSON:   "[]",
		MetadataJSON: "{}",
	}

	if len(key.PrivateKeyEnc) != 0 {
		t.Error("external key must not have encrypted private key")
	}
	if len(key.PrivateKeyNonce) != 0 {
		t.Error("external key must not have private key nonce")
	}
}
