package api

import (
	"net/http"
)

// handleChainInfo returns genesis JSON and persistent_peers for child nodes
// joining the CometBFT chain network.
func (s *Server) handleChainInfo(w http.ResponseWriter, r *http.Request) {
	genesis, peers := s.ChainInfo()
	if genesis == nil {
		s.respondError(w, http.StatusServiceUnavailable, "chain not enabled")
		return
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"genesis":          genesis,
		"persistent_peers": peers,
	})
}
