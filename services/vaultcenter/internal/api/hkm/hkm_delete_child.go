package hkm

import (
	"log"
	"net/http"

	chain "github.com/veilkey/veilkey-chain"
)

// handleDeleteChild removes a child node by node_id
func (h *Handler) handleDeleteChild(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if _, err := h.deps.SubmitTx(r.Context(), chain.TxDeleteChild, chain.DeleteChildPayload{NodeID: nodeID}); err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	log.Printf("Deleted child node: %s", nodeID)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": nodeID,
	})
}
