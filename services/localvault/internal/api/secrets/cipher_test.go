package secrets

import (
	"bytes"
	"testing"

	"github.com/veilkey/veilkey-go-package/crypto"
)

// ══════════════════════════════════════════════════════════════════
// Domain-level cipher tests for LocalVault secrets
// The secrets package stores ciphertext encrypted with AES-256-GCM
// via the agent's DEK. These tests verify the fundamental encryption
// invariants that all secret storage depends on.
// ══════════════════════════════════════════════════════════════════

// --- Secret encrypt/decrypt roundtrip ---

// Guarantees: A secret value encrypted with a DEK can be recovered with the same DEK.
// This is the core secret storage contract: when VaultCenter pushes an encrypted
// secret to LocalVault, it must be decryptable for bulk-apply file rendering.
func TestSecretEncryptDecrypt_Roundtrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"simple password", []byte("hunter2")},
		{"JSON credentials", []byte(`{"username":"admin","password":"s3cret"}`)},
		{"empty value", []byte("")},
		{"binary token", []byte{0xde, 0xad, 0xbe, 0xef, 0x00, 0x01}},
		{"multiline PEM key", []byte("-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----\n")},
	}

	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, nonce, err := crypto.Encrypt(dek, tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			decrypted, err := crypto.Decrypt(dek, ciphertext, nonce)
			if err != nil {
				t.Fatalf("Decrypt failed: %v", err)
			}

			if !bytes.Equal(decrypted, tt.plaintext) {
				t.Errorf("roundtrip failed: got %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

// --- Different KEKs produce different ciphertext ---

// Guarantees: The same plaintext encrypted with different keys produces different ciphertext.
// This ensures key isolation between vaults: secrets from one vault cannot be
// confused with or decrypted by another vault's key.
func TestDifferentKEKs_ProduceDifferentCiphertext(t *testing.T) {
	key1, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey (key1) failed: %v", err)
	}
	key2, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey (key2) failed: %v", err)
	}

	plaintext := []byte("same-secret-for-both-keys")

	ct1, _, err := crypto.Encrypt(key1, plaintext)
	if err != nil {
		t.Fatalf("Encrypt with key1 failed: %v", err)
	}
	ct2, _, err := crypto.Encrypt(key2, plaintext)
	if err != nil {
		t.Fatalf("Encrypt with key2 failed: %v", err)
	}

	if bytes.Equal(ct1, ct2) {
		t.Error("different keys must produce different ciphertext")
	}
}

// --- Tampered ciphertext fails decrypt ---

// Guarantees: Tampered ciphertext is detected and rejected.
// AES-GCM provides authenticated encryption: any bit flip in the ciphertext
// causes decryption to fail. This prevents an attacker who has DB access
// from silently modifying secret values.
func TestTamperedCiphertext_FailsDecrypt(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(ct []byte, nonce []byte) ([]byte, []byte)
	}{
		{
			name: "flipped ciphertext byte",
			tamper: func(ct, nonce []byte) ([]byte, []byte) {
				tampered := make([]byte, len(ct))
				copy(tampered, ct)
				tampered[0] ^= 0xff
				return tampered, nonce
			},
		},
		{
			name: "flipped nonce byte",
			tamper: func(ct, nonce []byte) ([]byte, []byte) {
				tampered := make([]byte, len(nonce))
				copy(tampered, nonce)
				tampered[0] ^= 0xff
				return ct, tampered
			},
		},
		{
			name: "truncated ciphertext",
			tamper: func(ct, nonce []byte) ([]byte, []byte) {
				if len(ct) < 2 {
					return ct, nonce
				}
				return ct[:len(ct)-1], nonce
			},
		},
		{
			name: "appended byte to ciphertext",
			tamper: func(ct, nonce []byte) ([]byte, []byte) {
				return append(ct, 0x42), nonce
			},
		},
	}

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plaintext := []byte("integrity-protected-secret")
			ct, nonce, err := crypto.Encrypt(key, plaintext)
			if err != nil {
				t.Fatalf("Encrypt failed: %v", err)
			}

			tamperedCT, tamperedNonce := tt.tamper(ct, nonce)
			_, err = crypto.Decrypt(key, tamperedCT, tamperedNonce)
			if err == nil {
				t.Error("Decrypt with tampered data must return error")
			}
		})
	}
}

// --- Cross-key decryption fails ---

// Guarantees: Ciphertext from one DEK cannot be decrypted with a different DEK.
// This ensures vault isolation: even if an attacker obtains the ciphertext
// from one vault's database, they cannot decrypt it with another vault's key.
func TestCrossKeyDecrypt_Fails(t *testing.T) {
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()

	ct, nonce, err := crypto.Encrypt(key1, []byte("vault-A-secret"))
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = crypto.Decrypt(key2, ct, nonce)
	if err == nil {
		t.Error("decrypting with wrong key must fail")
	}
}

// --- Edge case: encrypt empty data ---

// Guarantees: Empty plaintext can be encrypted and decrypted.
// Some AES-GCM implementations may mishandle zero-length inputs.
func TestEncrypt_EmptyData(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	ct, nonce, err := crypto.Encrypt(dek, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty data failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(dek, ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt empty data failed: %v", err)
	}

	if len(decrypted) != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", len(decrypted))
	}
}

// --- Edge case: encrypt 1MB data ---

// Guarantees: Large payloads (1MB) encrypt and decrypt correctly.
// This catches buffer overflow or memory issues with large inputs.
func TestEncrypt_1MB_Data(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	large := bytes.Repeat([]byte("A"), 1024*1024)
	ct, nonce, err := crypto.Encrypt(dek, large)
	if err != nil {
		t.Fatalf("Encrypt 1MB data failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(dek, ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt 1MB data failed: %v", err)
	}

	if !bytes.Equal(decrypted, large) {
		t.Error("1MB roundtrip failed: decrypted data does not match")
	}
}

// --- Edge case: encrypt all-zero bytes ---

// Guarantees: Data consisting entirely of zero bytes encrypts correctly.
// Some implementations have edge cases with all-zero inputs.
func TestEncrypt_AllZeroBytes(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	zeros := make([]byte, 256)
	ct, nonce, err := crypto.Encrypt(dek, zeros)
	if err != nil {
		t.Fatalf("Encrypt all-zero data failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(dek, ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt all-zero data failed: %v", err)
	}

	if !bytes.Equal(decrypted, zeros) {
		t.Error("all-zero roundtrip failed")
	}
}

// --- Edge case: encrypt all-0xFF bytes ---

// Guarantees: Data consisting entirely of 0xFF bytes encrypts correctly.
func TestEncrypt_AllFFBytes(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	ffs := bytes.Repeat([]byte{0xFF}, 256)
	ct, nonce, err := crypto.Encrypt(dek, ffs)
	if err != nil {
		t.Fatalf("Encrypt all-0xFF data failed: %v", err)
	}

	decrypted, err := crypto.Decrypt(dek, ct, nonce)
	if err != nil {
		t.Fatalf("Decrypt all-0xFF data failed: %v", err)
	}

	if !bytes.Equal(decrypted, ffs) {
		t.Error("all-0xFF roundtrip failed")
	}
}

// --- Edge case: decrypt truncated ciphertext ---

// Guarantees: Truncated ciphertext is rejected. AES-GCM requires the
// authentication tag at the end; truncating removes it.
func TestDecrypt_TruncatedCiphertext(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	plaintext := []byte("this will be truncated")
	ct, nonce, err := crypto.Encrypt(dek, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Truncate at multiple positions
	truncations := []int{1, len(ct) / 2, len(ct) - 1}
	for _, truncLen := range truncations {
		if truncLen >= len(ct) || truncLen <= 0 {
			continue
		}
		truncated := ct[:truncLen]
		_, err := crypto.Decrypt(dek, truncated, nonce)
		if err == nil {
			t.Errorf("Decrypt with ciphertext truncated to %d bytes must fail", truncLen)
		}
	}
}

// --- Edge case: decrypt extended ciphertext (extra bytes appended) ---

// Guarantees: Appending extra bytes to ciphertext is detected and rejected.
// This prevents length extension or padding oracle attacks.
func TestDecrypt_ExtendedCiphertext(t *testing.T) {
	dek, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	plaintext := []byte("this will be extended")
	ct, nonce, err := crypto.Encrypt(dek, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Append various extra bytes
	extras := [][]byte{
		{0x00},
		{0xFF},
		{0x00, 0x00, 0x00, 0x00},
		bytes.Repeat([]byte{0x42}, 16),
	}
	for _, extra := range extras {
		extended := append([]byte{}, ct...)
		extended = append(extended, extra...)
		_, err := crypto.Decrypt(dek, extended, nonce)
		if err == nil {
			t.Errorf("Decrypt with %d extra bytes appended must fail", len(extra))
		}
	}
}
