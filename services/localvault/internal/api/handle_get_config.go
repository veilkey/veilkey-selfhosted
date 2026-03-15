package api

import "net/http"

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		s.respondError(w, http.StatusBadRequest, "key is required")
		return
	}
	if !isValidResourceName(key) {
		s.respondError(w, http.StatusBadRequest, "key must match [A-Z_][A-Z0-9_]*")
		return
	}

	config, err := s.db.GetConfig(key)
	if err != nil {
		s.respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if config.Status == "block" {
		s.respondError(w, http.StatusLocked, "ref is blocked: "+ParsedRef{Family: RefFamilyVE, Scope: RefScope(config.Scope), ID: config.Key}.CanonicalString())
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"key":    config.Key,
		"value":  config.Value,
		"ref":    ParsedRef{Family: RefFamilyVE, Scope: RefScope(config.Scope), ID: config.Key}.CanonicalString(),
		"scope":  config.Scope,
		"status": config.Status,
	})
}
