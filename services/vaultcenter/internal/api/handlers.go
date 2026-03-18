package api

import (
	"net/http"
	"strings"

	"veilkey-vaultcenter/internal/api/admin"
)

// handleStatus returns current key version info
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
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
	mux.HandleFunc("POST /api/lookup/exact", s.requireTrustedIP(s.requireReadyForOps(s.handleExactLookup)))
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
