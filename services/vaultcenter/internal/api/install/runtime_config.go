package install

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"veilkey-vaultcenter/internal/db"
	"veilkey-vaultcenter/internal/httputil"
)

type installRuntimeConfigPayload struct {
	PublicBaseURL  string `json:"public_base_url"`
	RuntimeWarning string `json:"runtime_warning,omitempty"`
	InstallProfile string `json:"install_profile"`
	InstallRoot    string `json:"install_root"`
	InstallScript  string `json:"install_script"`
	InstallWorkdir string `json:"install_workdir"`
	VaultcenterURL string `json:"vaultcenter_url"`
	LocalvaultURL  string `json:"localvault_url"`
	TLSCertPath    string `json:"tls_cert_path"`
	TLSKeyPath     string `json:"tls_key_path"`
	TLSCAPath      string `json:"tls_ca_path"`
}

type installRuntimeConfigPatchRequest struct {
	PublicBaseURL  *string `json:"public_base_url"`
	InstallProfile *string `json:"install_profile"`
	InstallRoot    *string `json:"install_root"`
	InstallScript  *string `json:"install_script"`
	InstallWorkdir *string `json:"install_workdir"`
	VaultcenterURL *string `json:"vaultcenter_url"`
	LocalvaultURL  *string `json:"localvault_url"`
	TLSCertPath    *string `json:"tls_cert_path"`
	TLSKeyPath     *string `json:"tls_key_path"`
	TLSCAPath      *string `json:"tls_ca_path"`
}

func installRuntimeConfigFromUI(cfg *db.UIConfig) installRuntimeConfigPayload {
	return installRuntimeConfigPayload{
		PublicBaseURL:  cfg.PublicBaseURL,
		InstallProfile: cfg.InstallProfile,
		InstallRoot:    cfg.InstallRoot,
		InstallScript:  cfg.InstallScript,
		InstallWorkdir: cfg.InstallWorkdir,
		VaultcenterURL: cfg.VaultcenterURL,
		LocalvaultURL:  cfg.LocalvaultURL,
		TLSCertPath:    cfg.TLSCertPath,
		TLSKeyPath:     cfg.TLSKeyPath,
		TLSCAPath:      cfg.TLSCAPath,
	}
}

func validateOptionalURL(raw string) bool {
	if raw == "" {
		return true
	}
	parsed, err := url.Parse(raw)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func normalizeOptionalPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return filepath.Clean(raw)
}

func (h *Handler) handleGetInstallRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := h.db.GetOrCreateUIConfig()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}
	respond(w, http.StatusOK, installRuntimeConfigFromUI(cfg))
}

func (h *Handler) handlePatchInstallRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	var req installRuntimeConfigPatchRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		respondErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, err := h.db.GetOrCreateUIConfig()
	if err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to load install runtime config")
		return
	}

	if req.InstallProfile != nil {
		cfg.InstallProfile = strings.TrimSpace(*req.InstallProfile)
	}
	if req.PublicBaseURL != nil {
		cfg.PublicBaseURL = strings.TrimSpace(*req.PublicBaseURL)
	}
	if req.InstallRoot != nil {
		cfg.InstallRoot = normalizeOptionalPath(*req.InstallRoot)
	}
	if req.InstallScript != nil {
		cfg.InstallScript = normalizeOptionalPath(*req.InstallScript)
	}
	if req.InstallWorkdir != nil {
		cfg.InstallWorkdir = normalizeOptionalPath(*req.InstallWorkdir)
	}
	if req.VaultcenterURL != nil {
		cfg.VaultcenterURL = strings.TrimSpace(*req.VaultcenterURL)
	}
	if req.LocalvaultURL != nil {
		cfg.LocalvaultURL = strings.TrimSpace(*req.LocalvaultURL)
	}
	if req.TLSCertPath != nil {
		cfg.TLSCertPath = normalizeOptionalPath(*req.TLSCertPath)
	}
	if req.TLSKeyPath != nil {
		cfg.TLSKeyPath = normalizeOptionalPath(*req.TLSKeyPath)
	}
	if req.TLSCAPath != nil {
		cfg.TLSCAPath = normalizeOptionalPath(*req.TLSCAPath)
	}

	if !validateOptionalURL(cfg.PublicBaseURL) {
		respondErr(w, http.StatusBadRequest, "public_base_url must be an absolute URL")
		return
	}
	if !validateOptionalURL(cfg.VaultcenterURL) {
		respondErr(w, http.StatusBadRequest, "vaultcenter_url must be an absolute URL")
		return
	}
	if !validateOptionalURL(cfg.LocalvaultURL) {
		respondErr(w, http.StatusBadRequest, "localvault_url must be an absolute URL")
		return
	}
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath == "" {
		respondErr(w, http.StatusBadRequest, "tls_key_path is required when tls_cert_path is set")
		return
	}
	if cfg.TLSKeyPath != "" && cfg.TLSCertPath == "" {
		respondErr(w, http.StatusBadRequest, "tls_cert_path is required when tls_key_path is set")
		return
	}

	if err := h.db.SaveUIConfig(cfg); err != nil {
		respondErr(w, http.StatusInternalServerError, "failed to save install runtime config")
		return
	}
	respond(w, http.StatusOK, installRuntimeConfigFromUI(cfg))
}
