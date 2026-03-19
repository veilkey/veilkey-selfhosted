package secrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/veilkey/veilkey-go-package/crypto"
)

func decodeOpaqueCipherToken(raw string) ([]byte, []byte, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), ":", 3)
	if len(parts) != 3 || parts[0] != "VK" {
		return nil, nil, fmt.Errorf("ciphertext must use VK:{version}:{payload}")
	}
	payload, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, fmt.Errorf("ciphertext payload is not valid base64")
	}
	if len(payload) <= 12 {
		return nil, nil, fmt.Errorf("ciphertext payload is too short")
	}
	nonce := append([]byte{}, payload[:12]...)
	ciphertext := append([]byte{}, payload[12:]...)
	return ciphertext, nonce, nil
}

func (h *Handler) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Ciphertext string `json:"ciphertext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ciphertext, nonce, err := decodeOpaqueCipherToken(req.Ciphertext)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}
	dek, err := h.deps.GetLocalDEK()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load local DEK")
		return
	}
	plaintext, err := crypto.Decrypt(dek, ciphertext, nonce)
	if err != nil {
		respondError(w, http.StatusBadRequest, "decryption failed")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{
		"plaintext": string(plaintext),
	})
}
