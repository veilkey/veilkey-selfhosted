package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/veilkey/veilkey-go-package/httputil"
	"strings"

	"veilkey-localvault/internal/db"
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

// installStatus returns current initialization and vaultcenter connection status.
type installStatus struct {
	Initialized          bool   `json:"initialized"`
	VaultcenterURL       string `json:"vaultcenter_url,omitempty"`
	VaultcenterSource    string `json:"vaultcenter_source,omitempty"`
	Connected            bool   `json:"connected"`
	VaultcenterConnected bool   `json:"vaultcenter_connected"`
	Error                string `json:"error,omitempty"`
	VaultcenterError     string `json:"vaultcenter_error,omitempty"`
}

// HandleInstallStatus returns setup and vaultcenter connection status.
func (s *Server) HandleInstallStatus(w http.ResponseWriter, r *http.Request) {
	status := installStatus{Initialized: s.db.HasNodeInfo()}

	target := s.resolveVaultcenterTarget()
	status.VaultcenterURL = target.URL
	status.VaultcenterSource = target.Source

	// Check vaultcenter connectivity
	if target.URL != "" {
		healthURL := joinPath(target.URL, "/health")
		resp, err := s.httpClient.Get(healthURL)
		if err != nil {
			status.Connected = false
			status.VaultcenterConnected = false
			status.Error = err.Error()
			status.VaultcenterError = err.Error()
		} else {
			resp.Body.Close()
			status.Connected = resp.StatusCode == http.StatusOK
			status.VaultcenterConnected = status.Connected
			if !status.Connected {
				errMsg := "vaultcenter returned " + resp.Status
				status.Error = errMsg
				status.VaultcenterError = errMsg
			}
		}
	}

	w.Header().Set("Content-Type", httputil.ContentTypeJSON)
	json.NewEncoder(w).Encode(status)
}

// HandlePatchVaultcenterURL updates the vaultcenter URL in DB config.
func (s *Server) HandlePatchVaultcenterURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VaultcenterURL string `json:"vaultcenter_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.VaultcenterURL = strings.TrimSpace(req.VaultcenterURL)
	if req.VaultcenterURL == "" {
		http.Error(w, "vaultcenter_url is required", http.StatusBadRequest)
		return
	}
	// Basic URL validation
	if !strings.HasPrefix(req.VaultcenterURL, "http://") && !strings.HasPrefix(req.VaultcenterURL, "https://") {
		http.Error(w, "vaultcenter_url must start with http:// or https://", http.StatusBadRequest)
		return
	}

	if err := s.db.SaveConfig(db.ConfigKeyVaultcenterURL, strings.TrimRight(req.VaultcenterURL, "/")); err != nil {
		log.Printf("install: failed to save vaultcenter URL: %v", err)
		http.Error(w, "failed to save vaultcenter URL", http.StatusInternalServerError)
		return
	}

	log.Printf("install: vaultcenter URL updated to %s", req.VaultcenterURL)

	// Return updated status
	s.HandleInstallStatus(w, r)
}

// LogMiddleware wraps an http.Handler with request logging.
func LogMiddleware(next http.Handler) http.Handler {
	return logMiddleware(next)
}
