package hkm

import (
	"net/http"
)

// handleListRegistry returns key distribution map
func (h *Handler) handleListRegistry(w http.ResponseWriter, r *http.Request) {
	entries, err := h.deps.DB().ListRegistry()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list registry")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
		"count":   len(entries),
	})
}
