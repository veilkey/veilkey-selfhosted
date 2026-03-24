package hkm

import (
	"log"
	"net/http"
)

func (h *Handler) handleAgentArchive(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := h.deps.DB().ArchiveAgent(nodeID); err != nil {
		log.Printf("agent: archive failed node=%s: %v", nodeID, err)
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}
	log.Printf("agent: archived node=%s", nodeID)
	respondJSON(w, http.StatusOK, map[string]any{
		"node_id": nodeID,
		"status":  "archived",
	})
}

func (h *Handler) handleAgentUnarchive(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("node_id")
	if nodeID == "" {
		respondError(w, http.StatusBadRequest, "node_id is required")
		return
	}
	if err := h.deps.DB().UnarchiveAgent(nodeID); err != nil {
		log.Printf("agent: unarchive failed node=%s: %v", nodeID, err)
		respondError(w, http.StatusNotFound, "agent not found")
		return
	}
	log.Printf("agent: unarchived node=%s", nodeID)
	respondJSON(w, http.StatusOK, map[string]any{
		"node_id": nodeID,
		"status":  "active",
	})
}
