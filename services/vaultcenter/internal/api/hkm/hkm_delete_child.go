package hkm

import (
	"log"
	"net/http"
)

// handleDeleteChild removes a child node by node_id
func (h *Handler) handleDeleteChild(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := h.deps.DB().DeleteChild(nodeID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("Deleted child node: %s", nodeID)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": nodeID,
	})
}
