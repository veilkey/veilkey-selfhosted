package api

import (
	"net/http"
	"strings"
)

// handleStatus returns current key version info
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := s.hkmRuntimeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	resp["supported_features"] = localSupportedFeatures()
	resp["locked"] = s.IsLocked()
	install := s.currentInstallAccessState()
	resp["install"] = install
	resp["install_complete"] = install.Complete
	resp["install_session_exists"] = install.Exists
	resp["install_last_stage"] = install.LastStage
	s.respondJSON(w, http.StatusOK, resp)
}

// SetupAPIRoutes adds API routes to the mux
func (s *Server) SetupAPIRoutes(mux *http.ServeMux) {
	for _, path := range []string{
		"/vaults/all",
		"/vaults/list",
		"/vaults/keys",
		"/vaults/host",
		"/vaults/local",
		"/vaults/local/",
		"/functions/list",
		"/functions/bindings",
		"/functions/impact",
		"/functions/run",
		"/audit",
		"/settings/ui",
		"/settings/admin",
	} {
		mux.HandleFunc("GET "+path, s.handleOperatorShellEntry)
	}
	mux.HandleFunc("GET /vaults", s.handleOperatorShellEntry)
	mux.HandleFunc("GET /vaults/{vault}", s.handleLegacyVaultRoute)
	mux.HandleFunc("GET /vaults/local/{vault}", s.handleOperatorShellEntry)
	mux.HandleFunc("GET /audit/{vault}", s.handleOperatorShellEntry)

	mux.HandleFunc("POST /api/encrypt", s.requireTrustedIP(s.requireUnlocked(s.handleTempEncrypt)))
	mux.HandleFunc("GET /api/status", s.requireUnlocked(s.handleStatus))
	mux.HandleFunc("GET /api/configs", s.requireUnlocked(s.handleListConfigs))
	mux.HandleFunc("GET /api/configs/{key}", s.requireUnlocked(s.handleGetConfig))
	mux.HandleFunc("POST /api/configs", s.requireTrustedIP(s.requireUnlocked(s.handleSaveConfig)))
	mux.HandleFunc("PUT /api/configs/bulk", s.requireTrustedIP(s.requireUnlocked(s.handleSaveConfigsBulk)))
	mux.HandleFunc("DELETE /api/configs/{key}", s.requireTrustedIP(s.requireUnlocked(s.handleDeleteConfig)))
	mux.Handle("/assets/", s.assetHandler())
	mux.HandleFunc("GET /favicon.svg", s.handleAdminStaticFile)
	mux.HandleFunc("GET /api/ui/config", s.requireUnlocked(s.handleGetUIConfig))
	mux.HandleFunc("PATCH /api/ui/config", s.requireUnlocked(s.handlePatchUIConfig))
	mux.HandleFunc("GET /api/install/runtime-config", s.requireTrustedIP(s.requireUnlocked(s.handleGetInstallRuntimeConfig)))
	mux.HandleFunc("PATCH /api/install/runtime-config", s.requireTrustedIP(s.requireUnlocked(s.handlePatchInstallRuntimeConfig)))
	mux.HandleFunc("GET /api/install/apply", s.requireTrustedIP(s.requireUnlocked(s.handleGetInstallApply)))
	mux.HandleFunc("GET /api/install/runs", s.requireTrustedIP(s.requireUnlocked(s.handleGetInstallRuns)))
	mux.HandleFunc("POST /api/install/validate", s.requireTrustedIP(s.requireUnlocked(s.handleValidateInstallApply)))
	mux.HandleFunc("POST /api/install/apply", s.requireTrustedIP(s.requireUnlocked(s.handleRunInstallApply)))
	mux.HandleFunc("GET /api/system/update", s.requireUnlocked(s.handleGetSystemUpdate))
	mux.HandleFunc("POST /api/system/update", s.requireTrustedIP(s.requireUnlocked(s.handleRunSystemUpdate)))
	mux.HandleFunc("GET /api/otp-policy", s.handleOTPPolicy)
	mux.HandleFunc("GET /api/install/state", s.requireTrustedIP(s.handleGetInstallState))
	mux.HandleFunc("POST /api/install/session", s.requireTrustedIP(s.handleCreateInstallSession))
	mux.HandleFunc("PATCH /api/install/state", s.requireTrustedIP(s.handlePatchInstallState))
	mux.HandleFunc("POST /api/install/bootstrap/request", s.requireTrustedIP(s.handleCreateInstallBootstrapChallenge))
	mux.HandleFunc("POST /api/install/custody/request", s.requireTrustedIP(s.handleCreateInstallCustodyChallenge))
	mux.HandleFunc("POST /api/approvals/email-otp/request", s.requireTrustedIP(s.handleCreateEmailOTPChallenge))
	mux.HandleFunc("GET /api/approvals/email-otp/state", s.handleEmailOTPState)
	mux.HandleFunc("POST /api/approvals/secret-input/request", s.requireTrustedIP(s.handleCreateSecretInputChallenge))
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /preview/admin-vue", s.handleAdminVuePreview)
	mux.HandleFunc("GET /preview/admin-vue/", s.handleAdminVuePreview)
	mux.HandleFunc("GET /preview/admin-html-only", s.handleAdminHTMLOneShotPreview)
	mux.HandleFunc("GET /preview/admin-html-only/", s.handleAdminHTMLOneShotPreview)
	mux.HandleFunc("GET /preview/mockups/dark", s.handleAdminMockupDark)
	mux.HandleFunc("GET /preview/mockups/dark/", s.handleAdminMockupDark)
	mux.HandleFunc("GET /preview/mockups/amber", s.handleAdminMockupAmber)
	mux.HandleFunc("GET /preview/mockups/amber/", s.handleAdminMockupAmber)
	mux.HandleFunc("GET /preview/mockups/mono", s.handleAdminMockupMono)
	mux.HandleFunc("GET /preview/mockups/mono/", s.handleAdminMockupMono)
	mux.HandleFunc("GET /ui/approvals", s.handleApprovalHub)
	mux.HandleFunc("GET /ui/approvals/", s.handleApprovalHub)
	mux.HandleFunc("GET /ui/approvals/email-otp", s.handleEmailOTPPage)
	mux.HandleFunc("POST /ui/approvals/email-otp", s.handleSubmitEmailOTP)
	mux.HandleFunc("GET /ui/approvals/secret-input", s.handleSecretInputPage)
	mux.HandleFunc("POST /ui/approvals/secret-input", s.handleSubmitSecretInput)
	mux.HandleFunc("GET /approve/install/custody", s.handleInstallCustodyPage)
	mux.HandleFunc("GET /approve/install/bootstrap", s.handleInstallBootstrapPage)
	mux.HandleFunc("POST /approve/install/custody", s.handleSubmitInstallCustody)
	mux.HandleFunc("GET /approve/t/{token}", s.handleApprovalTokenPage)
	mux.HandleFunc("POST /approve/t/{token}", s.handleApprovalTokenSubmit)
	mux.HandleFunc("GET /ui/install/custody", s.handleInstallCustodyPage)
	mux.HandleFunc("POST /ui/install/custody", s.handleSubmitInstallCustody)
	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
}

func (s *Server) handleLegacyVaultRoute(w http.ResponseWriter, r *http.Request) {
	vault := r.PathValue("vault")
	if vault == "" {
		http.NotFound(w, r)
		return
	}
	target := "/vaults/local/" + vault
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (s *Server) handleAdminStaticFile(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/")
	if body, ok := devUIStaticFile(name); ok {
		if strings.HasSuffix(name, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		_, _ = w.Write(body)
		return
	}
	if body, ok := embeddedUIStaticFile(name); ok {
		if strings.HasSuffix(name, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		_, _ = w.Write(body)
		return
	}
	http.NotFound(w, r)
}
