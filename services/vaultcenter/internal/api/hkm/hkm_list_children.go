package hkm

import (
	"net/http"
)

// handleListChildren returns all registered children
func (h *Handler) handleListChildren(w http.ResponseWriter, r *http.Request) {
	children, err := h.deps.DB().ListChildren()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to list children")
		return
	}

	type childResp struct {
		NodeID        string `json:"node_id"`
		VaultNodeUUID string `json:"vault_node_uuid"`
		Label         string `json:"label"`
		URL           string `json:"url,omitempty"`
		Version       int    `json:"version"`
		LastSeen      string `json:"last_seen,omitempty"`
	}
	var result []childResp
	for i := range children {
		c := &children[i]
		cr := childResp{
			NodeID:        c.NodeID,
			VaultNodeUUID: c.NodeID,
			Label:         c.Label,
			URL:           c.URL,
			Version:       c.Version,
		}
		if c.LastSeen != nil {
			cr.LastSeen = c.LastSeen.Format("2006-01-02T15:04:05Z")
		}
		result = append(result, cr)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"children": result,
		"count":    len(result),
	})
}
