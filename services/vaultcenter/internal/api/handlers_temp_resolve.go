package api

import (
	"fmt"

	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
)

func (s *Server) resolveTempRef(tracked *db.TokenRef) (string, error) {
	ciphertext, nonce, err := crypto.DecodeCiphertext(tracked.Ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode temp ciphertext: %w", err)
	}

	dek, err := s.GetLocalDEK()
	if err != nil {
		return "", fmt.Errorf("get DEK: %w", err)
	}

	plaintext, err := crypto.Decrypt(dek, ciphertext, nonce)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}
