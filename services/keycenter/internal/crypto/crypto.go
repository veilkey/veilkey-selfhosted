package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// GenerateKey generates a random 32-byte key for AES-256
func GenerateKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// GenerateNonce generates a random 12-byte nonce for AES-GCM
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

// Encrypt encrypts plaintext using AES-256-GCM
func Encrypt(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}

	nonce, err = GenerateNonce()
	if err != nil {
		return nil, nil, err
	}

	ciphertext = gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM
func Decrypt(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// GenerateShortHash generates short hash from encrypted value.
// Storage remains CK:xxxxxxxx internally; APIs present scoped refs.
func GenerateShortHash(encryptedValue []byte) string {
	hash := sha256.Sum256(encryptedValue)
	return fmt.Sprintf("CK:%s", hex.EncodeToString(hash[:])[:8])
}

// GenerateUUID generates a random UUID v4
func GenerateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// EncryptDEK encrypts DEK with KEK (envelope encryption)
func EncryptDEK(kek, dek []byte) ([]byte, []byte, error) {
	return Encrypt(kek, dek)
}

// DecryptDEK decrypts DEK with KEK
func DecryptDEK(kek, encryptedDEK, nonce []byte) ([]byte, error) {
	return Decrypt(kek, encryptedDEK, nonce)
}
