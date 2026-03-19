package api

import (
	"net/http"

	"veilkey-vaultcenter/internal/api/admin"
)

func (s *Server) handleOperatorShellEntry(w http.ResponseWriter, r *http.Request) {
	s.handleDashboard(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if body, ok := admin.DevUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	if body, ok := admin.EmbeddedUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	http.Error(w, "admin ui build is not available", http.StatusServiceUnavailable)
}
