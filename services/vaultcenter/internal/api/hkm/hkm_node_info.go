package hkm

import (
	"net/http"
)

// handleNodeInfo returns this node's identity and stats
func (h *Handler) handleNodeInfo(w http.ResponseWriter, r *http.Request) {
	resp, err := h.hkmRuntimeInfo()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	respondJSON(w, http.StatusOK, resp)
}
