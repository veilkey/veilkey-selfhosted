package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// RenderInstallWizard serves the embedded Vue install wizard HTML.
func RenderInstallWizard(w http.ResponseWriter) {
	body, ok := embeddedInstallIndex()
	if !ok {
		http.Error(w, "install wizard UI not available", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(body)
}

// installStatus returns current initialization and keycenter connection status.
type installStatus struct {
	Initialized        bool   `json:"initialized"`
	KeycenterURL       string `json:"keycenter_url,omitempty"`
	KeycenterSource    string `json:"keycenter_source,omitempty"`
	Connected          bool   `json:"connected"`
	KeycenterConnected bool   `json:"keycenter_connected"`
	Error              string `json:"error,omitempty"`
	KeycenterError     string `json:"keycenter_error,omitempty"`
}

// HandleInstallStatus returns setup and keycenter connection status.
func (s *Server) HandleInstallStatus(w http.ResponseWriter, r *http.Request) {
	status := installStatus{Initialized: true}

	target := s.resolveKeycenterTarget()
	status.KeycenterURL = target.URL
	status.KeycenterSource = target.Source

	// Check keycenter connectivity
	if target.URL != "" {
		healthURL := strings.TrimRight(target.URL, "/") + "/health"
		resp, err := s.httpClient.Get(healthURL)
		if err != nil {
			status.Connected = false
			status.KeycenterConnected = false
			status.Error = err.Error()
			status.KeycenterError = err.Error()
		} else {
			resp.Body.Close()
			status.Connected = resp.StatusCode == http.StatusOK
			status.KeycenterConnected = status.Connected
			if !status.Connected {
				errMsg := "keycenter returned " + resp.Status
				status.Error = errMsg
				status.KeycenterError = errMsg
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// HandlePatchKeycenterURL updates the keycenter URL in DB config.
func (s *Server) HandlePatchKeycenterURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeycenterURL string `json:"keycenter_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.KeycenterURL = strings.TrimSpace(req.KeycenterURL)
	if req.KeycenterURL == "" {
		http.Error(w, "keycenter_url is required", http.StatusBadRequest)
		return
	}
	// Basic URL validation
	if !strings.HasPrefix(req.KeycenterURL, "http://") && !strings.HasPrefix(req.KeycenterURL, "https://") {
		http.Error(w, "keycenter_url must start with http:// or https://", http.StatusBadRequest)
		return
	}

	if err := s.db.SaveConfig("VEILKEY_KEYCENTER_URL", strings.TrimRight(req.KeycenterURL, "/")); err != nil {
		log.Printf("install: failed to save keycenter URL: %v", err)
		http.Error(w, "failed to save keycenter URL", http.StatusInternalServerError)
		return
	}

	log.Printf("install: keycenter URL updated to %s", req.KeycenterURL)

	// Return updated status
	s.HandleInstallStatus(w, r)
}

// LogMiddleware wraps an http.Handler with request logging.
func LogMiddleware(next http.Handler) http.Handler {
	return logMiddleware(next)
}
