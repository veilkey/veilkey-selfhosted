package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func (s *Server) handleSaveConfigsBulk(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Configs map[string]string `json:"configs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Configs) == 0 {
		s.respondError(w, http.StatusBadRequest, "configs map is required")
		return
	}
	if len(req.Configs) > maxBulkItems {
		s.respondError(w, http.StatusBadRequest, "too many configs (max 200)")
		return
	}

	for k := range req.Configs {
		if !isValidResourceName(k) {
			s.respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid key: %s (must match [A-Z_][A-Z0-9_]*)", k))
			return
		}
	}

	if err := s.db.SaveConfigs(req.Configs); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save configs: "+err.Error())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"saved": len(req.Configs),
	})
}
