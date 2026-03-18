package api

import (
	"log"
	"net/http"
)

// handleHeartbeat accepts URL updates from child nodes with version chain verification.
// If the child's reported DEK version doesn't match the parent's record, the child
// is considered out-of-sync (missed a rotation) and gets disconnected.
func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultNodeUUID string `json:"vault_node_uuid"`
		NodeID        string `json:"node_id"`
		URL           string `json:"url"`
		Version       int    `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	nodeID := req.VaultNodeUUID
	if nodeID == "" {
		nodeID = req.NodeID
	}
	if nodeID == "" || req.URL == "" {
		s.respondError(w, http.StatusBadRequest, "vault_node_uuid (or node_id) and url are required")
		return
	}

	child, err := s.db.GetChild(nodeID)
	if err != nil {
		s.respondError(w, http.StatusNotFound, "child "+nodeID+" not found")
		return
	}

	// Version chain verification
	if req.Version > 0 && req.Version != child.Version {
		log.Printf("heartbeat: VERSION MISMATCH child %s (%s) — reported v%d, expected v%d. Disconnecting.",
			nodeID, child.Label, req.Version, child.Version)
		if err := s.db.DeleteChild(nodeID); err != nil {
			log.Printf("heartbeat: failed to delete child %s: %v", nodeID, err)
		}
		s.respondJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":            "version_mismatch",
			"message":          "DEK version chain broken. Re-register with parent.",
			"expected_version": child.Version,
			"reported_version": req.Version,
		})
		return
	}

	if err := s.db.UpdateChildURL(nodeID, req.URL); err != nil {
		s.respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": child.Version,
	})
}
