package api

import (
	"net/http"
	"net/mail"
	"strings"
)

type uiConfigPayload struct {
	Locale         string `json:"locale"`
	DefaultEmail   string `json:"default_email"`
	TargetVersion  string `json:"target_version"`
	ReleaseChannel string `json:"release_channel"`
}

func (s *Server) handleGetUIConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load ui config")
		return
	}
	s.respondJSON(w, http.StatusOK, uiConfigPayload{
		Locale:         cfg.Locale,
		DefaultEmail:   cfg.DefaultEmail,
		TargetVersion:  cfg.TargetVersion,
		ReleaseChannel: cfg.ReleaseChannel,
	})
}

func (s *Server) handlePatchUIConfig(w http.ResponseWriter, r *http.Request) {
	var req uiConfigPayload
	if err := decodeJSON(r, &req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	locale := strings.ToLower(strings.TrimSpace(req.Locale))
	if locale != "ko" && locale != "en" {
		s.respondError(w, http.StatusBadRequest, "locale must be ko or en")
		return
	}
	cfg, err := s.db.GetOrCreateUIConfig()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to load ui config")
		return
	}
	cfg.Locale = locale
	defaultEmail := strings.TrimSpace(req.DefaultEmail)
	if defaultEmail != "" {
		if _, err := mail.ParseAddress(defaultEmail); err != nil {
			s.respondError(w, http.StatusBadRequest, "default_email must be a valid email address")
			return
		}
	}
	cfg.DefaultEmail = defaultEmail
	cfg.TargetVersion = strings.TrimSpace(req.TargetVersion)
	releaseChannel := strings.ToLower(strings.TrimSpace(req.ReleaseChannel))
	if releaseChannel == "" {
		releaseChannel = "stable"
	}
	switch releaseChannel {
	case "stable", "candidate", "nightly":
	default:
		s.respondError(w, http.StatusBadRequest, "release_channel must be stable, candidate, or nightly")
		return
	}
	cfg.ReleaseChannel = releaseChannel
	if err := s.db.SaveUIConfig(cfg); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save ui config")
		return
	}
	s.respondJSON(w, http.StatusOK, uiConfigPayload{
		Locale:         cfg.Locale,
		DefaultEmail:   cfg.DefaultEmail,
		TargetVersion:  cfg.TargetVersion,
		ReleaseChannel: cfg.ReleaseChannel,
	})
}
