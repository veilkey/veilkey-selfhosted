package api

import (
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/api/admin"
)

// handleStatus returns current key version info
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.IsLocked() {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"locked": true})
		return
	}
	if s.hkmHandler == nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	resp, err := s.hkmHandler.HkmRuntimeInfo()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "node info not available")
		return
	}
	resp["supported_features"] = localSupportedFeatures()
	resp["locked"] = false
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
		"/keycenter",
	} {
		mux.HandleFunc("GET "+path, s.handleOperatorShellEntry)
	}
	mux.HandleFunc("GET /vaults", s.handleOperatorShellEntry)
	mux.HandleFunc("GET /vaults/{vault}", s.handleLegacyVaultRoute)
	mux.HandleFunc("GET /vaults/local/{vault}", s.handleOperatorShellEntry)
	mux.HandleFunc("GET /audit/{vault}", s.handleOperatorShellEntry)
	mux.HandleFunc("GET /keycenter/{ref}", s.handleOperatorShellEntry)

	mux.HandleFunc("POST /api/encrypt", s.requireTrustedIP(s.requireUnlocked(s.handleTempEncrypt)))
	mux.HandleFunc("POST /api/lookup/exact", s.requireTrustedIP(s.requireReadyForOps(s.handleExactLookup)))
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/keycenter/temp-refs", s.requireUnlocked(s.requireAdminAuth(s.handleKeycenterTempRefs)))
	mux.HandleFunc("POST /api/keycenter/temp-refs", s.requireUnlocked(s.requireAdminAuth(s.handleKeycenterCreateTempRef)))
	mux.HandleFunc("GET /api/keycenter/temp-refs/{ref}/value", s.requireUnlocked(s.requireAdminAuth(s.handleKeycenterRevealRef)))
	mux.HandleFunc("POST /api/keycenter/promote", s.requireUnlocked(s.requireAdminAuth(s.handleKeycenterPromoteToVault)))
	mux.HandleFunc("POST /api/admin/login", s.handleAdminLogin)
	mux.HandleFunc("POST /api/admin/logout", s.handleAdminLogout)
	mux.HandleFunc("GET /api/admin/check", s.handleAdminCheck)
	// Registration token management
	mux.HandleFunc("POST /api/admin/registration-tokens", s.requireUnlocked(s.requireAdminAuth(s.handleCreateRegistrationToken)))
	mux.HandleFunc("GET /api/admin/registration-tokens", s.requireUnlocked(s.requireAdminAuth(s.handleListRegistrationTokens)))
	mux.HandleFunc("DELETE /api/admin/registration-tokens/{token_id}", s.requireUnlocked(s.requireAdminAuth(s.handleRevokeRegistrationToken)))
	mux.HandleFunc("GET /api/registration-tokens/{token_id}/validate", s.requireUnlocked(s.handleValidateRegistrationToken))
	mux.HandleFunc("GET /api/configs", s.requireUnlocked(s.handleListConfigs))
	mux.HandleFunc("GET /api/configs/{key}", s.requireUnlocked(s.handleGetConfig))
	mux.HandleFunc("POST /api/configs", s.requireTrustedIP(s.requireUnlocked(s.handleSaveConfig)))
	mux.HandleFunc("PUT /api/configs/bulk", s.requireTrustedIP(s.requireUnlocked(s.handleSaveConfigsBulk)))
	mux.HandleFunc("DELETE /api/configs/{key}", s.requireTrustedIP(s.requireUnlocked(s.handleDeleteConfig)))
	mux.Handle("/assets/", s.assetHandler())
	mux.HandleFunc("GET /favicon.svg", s.handleAdminStaticFile)
	mux.HandleFunc("GET /api/ui/config", s.requireUnlocked(s.handleGetUIConfig))
	mux.HandleFunc("PATCH /api/ui/config", s.requireUnlocked(s.handlePatchUIConfig))
	mux.HandleFunc("GET /api/system/update", s.requireUnlocked(s.handleGetSystemUpdate))
	mux.HandleFunc("POST /api/system/update", s.requireTrustedIP(s.requireUnlocked(s.handleRunSystemUpdate)))
	mux.HandleFunc("GET /api/otp-policy", s.handleOTPPolicy)
	mux.HandleFunc("GET /ui", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /ui/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /preview/admin-vue", s.adminHandler.HandleAdminVuePreview)
	mux.HandleFunc("GET /preview/admin-vue/", s.adminHandler.HandleAdminVuePreview)
	mux.HandleFunc("GET /preview/admin-html-only", s.adminHandler.HandleAdminHTMLOneShotPreview)
	mux.HandleFunc("GET /preview/admin-html-only/", s.adminHandler.HandleAdminHTMLOneShotPreview)
	mux.HandleFunc("GET /preview/mockups/dark", s.adminHandler.HandleAdminMockupDark)
	mux.HandleFunc("GET /preview/mockups/dark/", s.adminHandler.HandleAdminMockupDark)
	mux.HandleFunc("GET /preview/mockups/amber", s.adminHandler.HandleAdminMockupAmber)
	mux.HandleFunc("GET /preview/mockups/amber/", s.adminHandler.HandleAdminMockupAmber)
	mux.HandleFunc("GET /preview/mockups/mono", s.adminHandler.HandleAdminMockupMono)
	mux.HandleFunc("GET /preview/mockups/mono/", s.adminHandler.HandleAdminMockupMono)
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
	if body, ok := admin.DevUIStaticFile(name); ok {
		if strings.HasSuffix(name, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		_, _ = w.Write(body)
		return
	}
	if body, ok := admin.EmbeddedUIStaticFile(name); ok {
		if strings.HasSuffix(name, ".svg") {
			w.Header().Set("Content-Type", "image/svg+xml")
		}
		_, _ = w.Write(body)
		return
	}
	http.NotFound(w, r)
}
