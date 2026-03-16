package api

import (
	"net/http"
)


func (s *Server) handleOperatorShellEntry(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		renderInstallWizard(s, w)
		return
	}
	if complete, _ := s.installGateState(); !complete {
		renderInstallWizard(s, w)
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
