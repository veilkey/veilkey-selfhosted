package hkm

import (
	"log"
	"net/http"

	chain "github.com/veilkey/veilkey-chain"
)

func (h *Handler) handleAgentUnregisterByNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if _, err := h.deps.SubmitTx(r.Context(), chain.TxDeleteAgent, chain.DeleteAgentPayload{NodeID: nodeID}); err != nil {
		respondError(w, http.StatusNotFound, "not found")
		return
	}
	log.Printf("agent: unregistered node=%s", nodeID)
	respondJSON(w, http.StatusOK, map[string]any{
		"deleted": nodeID,
		"status":  "unregistered",
	})
}
