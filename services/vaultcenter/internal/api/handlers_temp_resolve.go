package api

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/veilkey/veilkey-go-package/crypto"
	"veilkey-vaultcenter/internal/db"
)

func (s *Server) resolveTempRef(tracked *db.TokenRef) (string, error) {
	parts := strings.SplitN(tracked.Ciphertext, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid temp ciphertext format")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode nonce: %w", err)
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
