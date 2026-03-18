package hkm

import (
	"log"
	"net/http"
)

func (h *Handler) handleAgentUnregisterByNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := h.deps.DB().DeleteAgentByNodeID(nodeID); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	log.Printf("agent: unregistered node=%s", nodeID)
	respondJSON(w, http.StatusOK, map[string]any{
		"deleted": nodeID,
		"status":  "unregistered",
	})
}
