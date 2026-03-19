package api

import "net/http"

func supportedFeatures() []string {
	return []string{
		"status",
		"node_info",
		"secrets",
		"configs",
		"resolve",
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status := "ok"
	if s.IsLocked() {
		status = "locked"
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": status})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		s.respondError(w, http.StatusServiceUnavailable, "server is locked")
		return
	}
	if err := s.db.Ping(); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	info, err := s.db.GetNodeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	secretCount, _ := s.db.CountSecrets()
	configCount, _ := s.db.CountConfigs()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"mode":               "vault",
		"node_id":            info.NodeID,
		"vault_node_uuid":    info.NodeID,
		"vault_hash":         s.identity.VaultHash,
		"vault_name":         s.identity.VaultName,
		"vault_id":           formatVaultID(s.identity.VaultName, s.identity.VaultHash),
		"version":            info.Version,
		"secrets_count":      secretCount,
		"configs_count":      configCount,
		"locked":             s.IsLocked(),
		"supported_features": supportedFeatures(),
	})
}

func formatVaultID(name, hash string) string {
	if hash == "" {
		return name
	}
	if name == "" {
		return hash
	}
	return name + ":" + hash
}
