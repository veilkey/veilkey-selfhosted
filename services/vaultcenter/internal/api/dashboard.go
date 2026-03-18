package api

import (
	"net/http"

	"veilkey-vaultcenter/internal/api/install"
)

func (s *Server) handleOperatorShellEntry(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		install.RenderInstallWizard(w)
		return
	}
	if complete, _ := s.installGateState(); !complete {
		install.RenderInstallWizard(w)
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
