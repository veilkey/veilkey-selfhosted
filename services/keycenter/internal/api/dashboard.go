package api

import (
	"net/http"
)

func (s *Server) handleOperatorShellEntry(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(lockedLandingHTML))
		return
	}
	if complete, session := s.installGateState(); !complete {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		renderInstallGate(s, w, session)
		return
	}
	s.handleDashboard(w, r)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if body, ok := devUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	if body, ok := embeddedUIIndex(); ok {
		_, _ = w.Write(body)
		return
	}
	http.Error(w, "admin ui build is not available", http.StatusServiceUnavailable)
}
