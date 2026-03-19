package secrets

import (
	"encoding/json"
	"net/http"
)

func (h *Handler) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Plaintext string `json:"plaintext"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Plaintext == "" {
		respondError(w, http.StatusBadRequest, "plaintext is required")
		return
	}
	respondError(w, http.StatusForbidden, vaultcenterOnlyDecryptMessage)
}
